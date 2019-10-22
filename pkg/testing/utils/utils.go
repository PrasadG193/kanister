package utils

import (
	"context"
	"github.com/pkg/errors"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/rds"

	awsconfig "github.com/kanisterio/kanister/pkg/config/aws"
)

const (
	maxRetries = 10
)

// EC2 is kasten's wrapper around ec2.EC2 structs
type EC2 struct {
	*ec2.EC2
	DryRun bool
	Role   string
}

// RDS is kasten's wrapper around ec2.RDS structs
type RDS struct {
	*rds.RDS
	Role string
}

func newAwsConfig(accessID, secretKey, region string) (*aws.Config, *session.Session, string, error) {
	config := make(map[string]string)
	config[awsconfig.ConfigRegion] = region
	config[awsconfig.AccessKeyID] = accessID
	config[awsconfig.SecretAccessKey] = secretKey

	awsConfig, region, role, err := awsconfig.GetConfig(config)
	if err != nil {
		return nil, nil, "", err
	}

	s, err := session.NewSession(awsConfig)
	if err != nil {
		return nil, nil, "", errors.Wrap(err, "Failed to create session for EFS")
	}
	creds := awsConfig.Credentials
	if role != "" {
		creds = stscreds.NewCredentials(s, role)
	}
	return awsConfig.WithMaxRetries(maxRetries).WithRegion(region).WithCredentials(creds), s, role, nil
}

// NewEC2Client returns ec2 client struct.
func NewEC2Client(ctx context.Context, accessID, secretKey, region string) (*EC2, error) {
	conf, s, role, err := newAwsConfig(accessID, secretKey, region)
	if err != nil {
		return nil, err
	}
	return &EC2{EC2: ec2.New(s, conf), Role: role}, nil
}

func (e EC2) DescribeSecurityGroup(ctx context.Context, groupName string) (*ec2.DescribeSecurityGroupsOutput, error) {
	sgi := &ec2.DescribeSecurityGroupsInput{
		DryRun:     &e.DryRun,
		GroupNames: []*string{&groupName},
	}
	return e.DescribeSecurityGroupsWithContext(ctx, sgi)
}

func (e EC2) CreateSecurityGroup(ctx context.Context, groupName, description string) (*ec2.CreateSecurityGroupOutput, error) {
	sgi := &ec2.CreateSecurityGroupInput{
		DryRun:      &e.DryRun,
		Description: &description,
		GroupName:   &groupName,
	}
	return e.CreateSecurityGroupWithContext(ctx, sgi)
}

func (e EC2) AuthorizeSecurityGroupIngress(ctx context.Context, groupName, cidr, protocol string, port int64) (*ec2.AuthorizeSecurityGroupIngressOutput, error) {
	sgi := &ec2.AuthorizeSecurityGroupIngressInput{
		DryRun:     &e.DryRun,
		GroupName:  &groupName,
		CidrIp:     &cidr,
		IpProtocol: &protocol,
		ToPort:     &port,
		FromPort:   &port,
	}
	return e.AuthorizeSecurityGroupIngressWithContext(ctx, sgi)
}

func (e EC2) DeleteSecurityGroup(ctx context.Context, groupName string) (*ec2.DeleteSecurityGroupOutput, error) {
	sgi := &ec2.DeleteSecurityGroupInput{
		DryRun:    &e.DryRun,
		GroupName: &groupName,
	}
	return e.DeleteSecurityGroupWithContext(ctx, sgi)
}

// NewRDSClient returns ec2 client struct.
func NewRDSClient(ctx context.Context, accessID, secretKey, region string) (*RDS, error) {
	conf, s, role, err := newAwsConfig(accessID, secretKey, region)
	if err != nil {
		return nil, err
	}
	return &RDS{RDS: rds.New(s, conf), Role: role}, nil
}

// CreateDBInstanceWithContext
func (r RDS) CreateDBInstance(ctx context.Context, storage int64, instanceClass, instanceID, engine, username, password, sgid string) (*rds.CreateDBInstanceOutput, error) {
	dbi := &rds.CreateDBInstanceInput{
		AllocatedStorage:     &storage,
		DBInstanceIdentifier: &instanceID,
		VpcSecurityGroupIds:  []*string{&sgid},
		DBInstanceClass:      &instanceClass,
		Engine:               &engine,
		MasterUsername:       &username,
		MasterUserPassword:   &password,
	}
	return r.CreateDBInstanceWithContext(ctx, dbi)
}

func (r RDS) WaitUntilDBInstanceAvailable(ctx context.Context, instanceID string) error {
	dba := &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: &instanceID,
	}
	return r.WaitUntilDBInstanceAvailableWithContext(ctx, dba)
}

func (r RDS) WaitUntilDBInstanceDeleted(ctx context.Context, instanceID string) error {
	dba := &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: &instanceID,
	}
	return r.WaitUntilDBInstanceDeletedWithContext(ctx, dba)
}

func (r RDS) DescribeDBInstances(ctx context.Context, instanceID string) (*rds.DescribeDBInstancesOutput, error) {
	dbi := &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: &instanceID,
	}
	return r.DescribeDBInstancesWithContext(ctx, dbi)
}

func (r RDS) DeleteDBInstance(ctx context.Context, instanceID string) (*rds.DeleteDBInstanceOutput, error) {
	skipSnapshot := true
	dbi := &rds.DeleteDBInstanceInput{
		DBInstanceIdentifier: &instanceID,
		SkipFinalSnapshot:    &skipSnapshot,
	}
	return r.DeleteDBInstanceWithContext(ctx, dbi)
}
