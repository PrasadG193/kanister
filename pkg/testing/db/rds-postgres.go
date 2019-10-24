// Copyright 2019 The Kanister Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	log "github.com/sirupsen/logrus"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	awsconfig "github.com/kanisterio/kanister/pkg/config/aws"
	"github.com/kanisterio/kanister/pkg/testing/utils"

	// Initialize pq driver
	_ "github.com/lib/pq"
)

type PostgresDB struct {
	cli             kubernetes.Interface
	id              string
	host            string
	dbname          string
	username        string
	password        string
	accessID        string
	secretKey       string
	region          string
	sessionToken    string
	securityGroupID string

	PostgresData Data
}

type PostgresData struct {
	cli       kubernetes.Interface
	namespace string
	sqlDB     *sql.DB
}

func NewPostgresDB(cli kubernetes.Interface) (Database, error) {
	return &PostgresDB{
		cli:      cli,
		id:       "test-postgresql-instance",
		dbname:   "postgres",
		username: "master",
		password: "secret99",
	}, nil
}

func (pdb *PostgresDB) GetConfig(ctx context.Context) error {
	var ok bool

	pdb.region, ok = os.LookupEnv(awsconfig.Region)
	if !ok {
		return fmt.Errorf("Env var %s is not set", awsconfig.Region)
	}

	// If sessionToken is set, accessID and secretKey not required
	pdb.sessionToken, ok = os.LookupEnv(awsconfig.SessionToken)
	if ok {
		return nil
	}

	pdb.accessID, ok = os.LookupEnv(awsconfig.AccessKeyID)
	if !ok {
		return fmt.Errorf("Env var %s is not set", awsconfig.AccessKeyID)
	}
	pdb.secretKey, ok = os.LookupEnv(awsconfig.SecretAccessKey)
	if !ok {
		return fmt.Errorf("Env var %s is not set", awsconfig.SecretAccessKey)
	}
	return nil
}

func (pdb *PostgresDB) Install(ctx context.Context, nsName string) error {
	var err error
	// Create Namespace
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
		},
	}
	_, err = pdb.cli.CoreV1().Namespaces().Create(ns)
	if err != nil {
		return err
	}

	// Create ec2 client
	ec2, err := utils.NewEC2Client(ctx, pdb.accessID, pdb.secretKey, pdb.region, pdb.sessionToken)
	if err != nil {
		return err
	}

	// Create security group
	log.Info("PostgresDB: creating security group")
	sg, err := ec2.CreateSecurityGroup(ctx, "pgtest-sg", "pgtest-security-group")
	if err != nil {
		return err
	}
	pdb.securityGroupID = *sg.GroupId

	// Add ingress rule
	log.Info("PostgresDB: adding ingress rule to security group")
	_, err = ec2.AuthorizeSecurityGroupIngress(ctx, "pgtest-sg", "0.0.0.0/0", "tcp", 5432)
	if err != nil {
		return err
	}

	// Create rds client
	rds, err := utils.NewRDSClient(ctx, pdb.accessID, pdb.secretKey, pdb.region, pdb.sessionToken)
	if err != nil {
		return err
	}

	// Create RDS instance
	log.Info("PostgresDB: creating rds instance")
	_, err = rds.CreateDBInstance(ctx, 20, "db.t2.micro", pdb.id, "postgres", pdb.username, pdb.password, pdb.securityGroupID)
	if err != nil {
		return err
	}

	// Wait for DB to be ready
	log.Info("PostgresDB: Waiting for rds to be ready")
	err = rds.WaitUntilDBInstanceAvailable(ctx, pdb.id)
	if err != nil {
		return err
	}

	// Find host of the instance
	dbInstance, err := rds.DescribeDBInstances(ctx, pdb.id)
	if err != nil {
		return err
	}
	pdb.host = *dbInstance.DBInstances[0].Endpoint.Address

	// Init Data
	pdb.PostgresData = &PostgresData{
		cli:       pdb.cli,
		namespace: nsName,
	}

	return nil
}

func (pdb PostgresDB) CreateConfig(ctx context.Context, ns string) error {
	// Create configmap
	dbconfig := &v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "dbconfig",
		},
		Data: map[string]string{
			"postgres.instanceid": pdb.id,
			"postgres.host":       pdb.host,
			"postgres.database":   pdb.dbname,
			"postgres.user":       pdb.username,
		},
	}
	_, err := pdb.cli.CoreV1().ConfigMaps(ns).Create(dbconfig)
	if err != nil {
		return err
	}

	// Create secret
	dbsecret := &v1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "dbsecret",
		},
		StringData: map[string]string{
			"password":          pdb.password,
			"access_key_id":     pdb.accessID,
			"secret_access_key": pdb.secretKey,
			"aws_region":        pdb.region,
		},
	}
	_, err = pdb.cli.CoreV1().Secrets(ns).Create(dbsecret)
	if err != nil {
		return err
	}
	return nil
}

func (pdb PostgresDB) Remove(ctx context.Context, nsName string) error {
	// Create rds client
	rds, err := utils.NewRDSClient(ctx, pdb.accessID, pdb.secretKey, pdb.region, pdb.sessionToken)
	if err != nil {
		log.Errorf("Failed to create rds client: %s. You may need to delete RDS resources manually", err.Error())
		return err
	}

	// Delete rds instance
	log.Info("PostgresDB: deleting rds instance")
	_, err = rds.DeleteDBInstance(ctx, pdb.id)
	if err == nil {
		// Waiting for rds to be deleted
		log.Info("PostgresDB: Waiting for rds to be deleted")
		err = rds.WaitUntilDBInstanceDeleted(ctx, pdb.id)
		if err != nil {
			log.Errorf("Failed to wait for rds instance %s till delete succeeds: %s", pdb.id, err.Error())
		}
	} else {
		log.Errorf("Failed to delete rds instance %s: %s. You may need to delete it manually", pdb.id, err.Error())
	}

	// Create ec2 client
	ec2, err := utils.NewEC2Client(ctx, pdb.accessID, pdb.secretKey, pdb.region, pdb.sessionToken)
	if err != nil {
		log.Errorf("Failed to ec2 rds client: %s. You may need to delete EC2 resources manually", err.Error())
		return err
	}

	// Delete security group
	log.Info("PostgresDB: deleting security group")
	_, err = ec2.DeleteSecurityGroup(ctx, "pgtest-sg")
	if err != nil {
		log.Errorf("Failed to delete security group pgtest-sg: %s. You may need to delete it manually", err.Error())
	}
	return nil
}

func (pdb PostgresDB) Data() Data {
	return pdb.PostgresData
}

// PostgresData methods
// Ping makes and tests DB connection
func (c *PostgresData) Ping(ctx context.Context) error {
	// Get connection info from configmap
	dbconfig, err := c.cli.CoreV1().ConfigMaps(c.namespace).Get("dbconfig", metav1.GetOptions{})
	if err != nil {
		return err
	}

	// Get secret creds
	dbsecret, err := c.cli.CoreV1().Secrets(c.namespace).Get("dbsecret", metav1.GetOptions{})
	if err != nil {
		return err
	}

	var connectionString string = fmt.Sprintf("host=%s user=%s password=%s dbname=%s sslmode=disable", dbconfig.Data["postgres.host"], dbconfig.Data["postgres.user"], dbsecret.Data["password"], dbconfig.Data["postgres.database"])

	// Initialize connection object.
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		return err
	}

	err = db.Ping()
	if err != nil {
		return err
	}

	c.sqlDB = db
	log.Info("Successfully created connection to database")
	return nil
}

func (c *PostgresData) Insert(ctx context.Context, n int) error {
	for i := 0; i < n; i++ {
		now := time.Now().Format(time.RFC3339Nano)
		stmt := "INSERT INTO inventory (name) VALUES ($1);"
		_, err := c.sqlDB.Exec(stmt, now)
		if err != nil {
			return err
		}
		log.Info("Inserted a row")
	}
	return nil
}

func (c *PostgresData) Count(ctx context.Context) (int, error) {
	stmt := "SELECT COUNT(*) FROM inventory;"
	row := c.sqlDB.QueryRow(stmt)
	var count int
	err := row.Scan(&count)
	if err != nil {
		return 0, err
	}
	log.Infof("Found %d rows\n", count)
	return count, nil
}

func (c *PostgresData) Reset(ctx context.Context) error {
	_, err := c.sqlDB.Exec("DROP TABLE IF EXISTS inventory;")
	if err != nil {
		return err
	}
	log.Info("Finished dropping table (if existed)")

	// Create table.
	_, err = c.sqlDB.Exec("CREATE TABLE inventory (id serial PRIMARY KEY, name VARCHAR(50));")
	if err != nil {
		return err
	}
	log.Info("Finished creating table")
	return nil
}
