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

package app

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"k8s.io/client-go/kubernetes"

	"github.com/pkg/errors"

	crv1alpha1 "github.com/kanisterio/kanister/pkg/apis/cr/v1alpha1"
	awsconfig "github.com/kanisterio/kanister/pkg/config/aws"
	"github.com/kanisterio/kanister/pkg/field"
	"github.com/kanisterio/kanister/pkg/helm"
	"github.com/kanisterio/kanister/pkg/kube"
	"github.com/kanisterio/kanister/pkg/log"
)

type chartInfo struct {
	release  string
	chart    string
	repoUrl  string
	repoName string
	values   map[string]string
}

type PostgresDB struct {
	cli           kubernetes.Interface
	chart         chartInfo
	password      string
	namespace     string
	podName       string
	containerName string
}

type PostgresBP struct {
	name         string
	appNamespace string
}

func NewPostgresDB() App {
	return &PostgresDB{
		password: "test@54321",
		chart: chartInfo{
			release:  "my-postgres",
			repoName: "stable",
			repoUrl:  "https://kubernetes-charts.storage.googleapis.com",
			chart:    "postgresql",
			values: map[string]string{
				"image.repository":                      "kanisterio/postgresql",
				"image.tag":                             "0.22.0",
				"postgresqlPassword":                    "test@54321",
				"postgresqlExtendedConf.archiveCommand": "'envdir /bitnami/postgresql/data/env wal-e wal-push %p'",
				"postgresqlExtendedConf.archiveMode":    "true",
				"postgresqlExtendedConf.archiveTimeout": "60",
				"postgresqlExtendedConf.walLevel":       "archive",
			},
		},
	}
}

func (pdb *PostgresDB) getStatefulSetName() string {
	return fmt.Sprintf("%s-postgresql", pdb.chart.release)
}

func (pdb *PostgresDB) Init(ctx context.Context) error {
	// Instantiate Client SDKs
	cfg, err := kube.LoadConfig()
	if err != nil {
		return nil
	}
	pdb.cli, err = kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}

	if _, ok := os.LookupEnv(awsconfig.Region); !ok {
		return fmt.Errorf("Env var %s is not set", awsconfig.Region)
	}
	// If sessionToken is set, accessID and secretKey not required
	if _, ok := os.LookupEnv(awsconfig.SessionToken); ok {
		return nil
	}
	if _, ok := os.LookupEnv(awsconfig.AccessKeyID); !ok {
		return fmt.Errorf("Env var %s is not set", awsconfig.AccessKeyID)
	}
	if _, ok := os.LookupEnv(awsconfig.SecretAccessKey); !ok {
		return fmt.Errorf("Env var %s is not set", awsconfig.SecretAccessKey)
	}
	return nil
}

func (pdb *PostgresDB) Install(ctx context.Context, ns string) error {
	log.Info().Print("Installing helm chart.", field.M{"app": "postgresql", "release": pdb.chart.release, "namespace": ns})
	pdb.namespace = ns

	// Create helm client
	cli := helm.NewCliClient()

	// Add helm repo and fetch charts
	if err := cli.AddRepo(ctx, pdb.chart.repoName, pdb.chart.repoUrl); err != nil {
		return err
	}
	// Install helm chart
	if err := cli.Install(ctx, fmt.Sprintf("%s/%s", pdb.chart.repoName, pdb.chart.chart), pdb.chart.release, pdb.namespace, pdb.chart.values); err != nil {
		return err
	}
	return nil
}

func (pdb *PostgresDB) IsReady(ctx context.Context) (bool, error) {
	// Add timeout to context
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	if err := kube.WaitOnStatefulSetReady(ctx, pdb.cli, pdb.namespace, pdb.getStatefulSetName()); err != nil {
		return false, err
	}
	return true, nil
}

func (pdb *PostgresDB) Object() crv1alpha1.ObjectReference {
	return crv1alpha1.ObjectReference{
		Kind:      "statefulset",
		Name:      pdb.getStatefulSetName(),
		Namespace: pdb.namespace,
	}
}

func (pdb PostgresDB) ConfigMaps() map[string]crv1alpha1.ObjectReference {
	return nil
}

func (pdb PostgresDB) Secrets() map[string]crv1alpha1.ObjectReference {
	return map[string]crv1alpha1.ObjectReference{
		"postgresql": crv1alpha1.ObjectReference{
			Kind:      "secret",
			Name:      pdb.getStatefulSetName(),
			Namespace: pdb.namespace,
		},
	}
}

// Ping makes and tests DB connection
func (pdb *PostgresDB) Ping(ctx context.Context) error {
	// Get pod and container name
	pod, container, err := getPodContainerFromStatefulSet(ctx, pdb.cli, pdb.namespace, pdb.getStatefulSetName())
	if err != nil {
		return err
	}
	cmd := "pg_isready -U 'postgres' -h 127.0.0.1 -p 5432"
	_, stderr, err := kube.Exec(pdb.cli, pdb.namespace, pod, container, []string{"sh", "-c", cmd}, nil)
	if err != nil {
		return errors.Wrapf(err, "Failed to ping postgresql DB. %s", stderr)
	}
	log.Info().Print("Connected to database.", field.M{"app": "postgresql"})
	return nil
}

func (pdb PostgresDB) Insert(ctx context.Context, n int) error {
	// Get pod and container name
	pod, container, err := getPodContainerFromStatefulSet(ctx, pdb.cli, pdb.namespace, pdb.getStatefulSetName())
	if err != nil {
		return err
	}
	for i := 0; i < n; i++ {
		cmd := fmt.Sprintf("PGPASSWORD=${POSTGRES_PASSWORD} psql -d test -c \"INSERT INTO COMPANY (NAME,AGE,CREATED_AT) VALUES ('foo', 32, now());\"")
		_, stderr, err := kube.Exec(pdb.cli, pdb.namespace, pod, container, []string{"sh", "-c", cmd}, nil)
		if err != nil {
			return errors.Wrapf(err, "Failed to create db in postgresql. %s", stderr)
		}
		log.Info().Print("Inserted a row in test db.", field.M{"app": "postgresql"})
	}
	return nil
}

func (pdb PostgresDB) Count(ctx context.Context) (int, error) {
	// Get pod and container name
	pod, container, err := getPodContainerFromStatefulSet(ctx, pdb.cli, pdb.namespace, pdb.getStatefulSetName())
	if err != nil {
		return 0, err
	}
	cmd := "PGPASSWORD=${POSTGRES_PASSWORD} psql -d test -c 'SELECT COUNT(*) FROM company;'"
	stdout, stderr, err := kube.Exec(pdb.cli, pdb.namespace, pod, container, []string{"sh", "-c", cmd}, nil)
	if err != nil {
		return 0, errors.Wrapf(err, "Failed to count db entries in postgresql. %s ", stderr)
	}

	out := strings.Fields(stdout)
	if len(out) < 4 {
		return 0, fmt.Errorf("Unknown response for count query")
	}
	count, err := strconv.Atoi(out[2])
	if err != nil {
		return 0, errors.Wrapf(err, "Failed to count db entries in postgresql. %s ", stderr)
	}
	log.Info().Print("Counting rows in test db.", field.M{"app": "postgresql", "count": count})
	return count, nil
}

func (pdb PostgresDB) Reset(ctx context.Context) error {
	// Get pod and container name
	pod, container, err := getPodContainerFromStatefulSet(ctx, pdb.cli, pdb.namespace, pdb.getStatefulSetName())
	if err != nil {
		return err
	}

	// Delete database if exists
	cmd := "PGPASSWORD=${POSTGRES_PASSWORD} psql -c 'DROP DATABASE IF EXISTS test;'"
	_, stderr, err := kube.Exec(pdb.cli, pdb.namespace, pod, container, []string{"sh", "-c", cmd}, nil)
	if err != nil {
		return errors.Wrapf(err, "Failed to drop db from postgresql. %s ", stderr)
	}

	// Create database
	cmd = "PGPASSWORD=${POSTGRES_PASSWORD} psql -c 'CREATE DATABASE test;'"
	_, stderr, err = kube.Exec(pdb.cli, pdb.namespace, pod, container, []string{"sh", "-c", cmd}, nil)
	if err != nil {
		return errors.Wrapf(err, "Failed to create db in postgresql. %s ", stderr)
	}

	// Create table
	cmd = "PGPASSWORD=${POSTGRES_PASSWORD} psql -d test -c 'CREATE TABLE COMPANY(ID SERIAL PRIMARY KEY NOT NULL, NAME TEXT NOT NULL, AGE INT NOT NULL, CREATED_AT TIMESTAMP);'"
	_, stderr, err = kube.Exec(pdb.cli, pdb.namespace, pod, container, []string{"sh", "-c", cmd}, nil)
	if err != nil {
		return errors.Wrapf(err, "Failed to create table in postgresql. %s ", stderr)
	}
	log.Info().Print("Database reset successful!", field.M{"app": "postgresql"})
	return nil
}

func (pdb PostgresDB) Uninstall(ctx context.Context) error {
	log.Info().Print("Uninstalling helm chart.", field.M{"app": "postgresql", "release": pdb.chart.release, "namespace": pdb.namespace})
	// Create helm client
	cli := helm.NewCliClient()

	// Install helm chart
	if err := cli.Uninstall(ctx, pdb.chart.release, pdb.namespace); err != nil {
		return err
	}

	// Add helm repo and fetch charts
	if err := cli.RemoveRepo(ctx, pdb.chart.repoName); err != nil {
		return err
	}
	return nil
}
