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

	crv1alpha1 "github.com/kanisterio/kanister/pkg/apis/cr/v1alpha1"
	crclient "github.com/kanisterio/kanister/pkg/client/clientset/versioned/typed/cr/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type PostgresApp struct {
	cli       kubernetes.Interface
	crCli     crclient.CrV1alpha1Interface
	namespace string
}

// NewPostgresApp
func NewPostgresApp(cli kubernetes.Interface, crCli crclient.CrV1alpha1Interface, ns string) Kanister {
	return &PostgresApp{
		cli:       cli,
		crCli:     crCli,
		namespace: ns,
	}
}

func (app PostgresApp) Install(ctx context.Context, controllerNs string) error {
	// Get secret creds
	dbsecret, err := app.cli.CoreV1().Secrets(app.namespace).Get("dbsecret", metav1.GetOptions{})
	if err != nil {
		return err
	}

	// Create profile
	p := &crv1alpha1.Profile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "profile",
			Namespace: app.namespace,
		},
		Location: crv1alpha1.Location{
			Type:   crv1alpha1.LocationTypeS3Compliant,
			Region: string(dbsecret.Data["aws_region"]),
		},
		Credential: crv1alpha1.Credential{
			Type: crv1alpha1.CredentialTypeKeyPair,
			KeyPair: &crv1alpha1.KeyPair{
				Secret: crv1alpha1.ObjectReference{
					Name:      "dbsecret",
					Namespace: app.namespace,
				},
				IDField:     "access_key_id",
				SecretField: "secret_access_key",
			},
		},
	}
	_, err = app.crCli.Profiles(app.namespace).Create(p)
	if err != nil {
		return err
	}

	// Create blueprint
	bp := &crv1alpha1.Blueprint{
		ObjectMeta: metav1.ObjectMeta{
			Name: "blueprint",
		},
		Actions: map[string]*crv1alpha1.BlueprintAction{
			"backup": &crv1alpha1.BlueprintAction{
				Kind: "Namespace",
				OutputArtifacts: map[string]crv1alpha1.Artifact{
					"snapshot": crv1alpha1.Artifact{
						KeyValue: map[string]string{
							"id":   "{{ .Namespace.Name }}-{{ toDate \"2006-01-02T15:04:05.999999999Z07:00\" .Time  | date \"2006-01-02T15-04-05\" }}",
							"sgid": "{{ .Phases.backupSnapshot.Output.securityGroupID }}",
						},
					},
				},
				ConfigMapNames: []string{"dbconfig"},
				Phases: []crv1alpha1.BlueprintPhase{
					crv1alpha1.BlueprintPhase{
						Func: "KubeTask",
						Name: "backupSnapshot",
						Args: map[string]interface{}{
							"namespace": app.namespace,
							"image":     "kanisterio/postgres-kanister-tools:0.21.0",
							"command": []string{
								"bash",
								"-o",
								"errexit",
								"-o",
								"pipefail",
								"-o",
								"nounset",
								"-o",
								"xtrace",
								"-c",
								"set +o xtrace\n" +
									"export AWS_SECRET_ACCESS_KEY=\"{{ .Profile.Credential.KeyPair.Secret }}\"\n" +
									"export AWS_ACCESS_KEY_ID=\"{{ .Profile.Credential.KeyPair.ID }}\"\n" +
									"set -o xtrace\n" +
									"aws rds create-db-snapshot --db-instance-identifier=\"{{ index .ConfigMaps.dbconfig.Data \"postgres.instanceid\" }}\" --db-snapshot-identifier=\"{{ .Namespace.Name }}-{{ toDate \"2006-01-02T15:04:05.999999999Z07:00\" .Time  | date \"2006-01-02T15-04-05\" }}\" --region \"{{ .Profile.Location.Region }}\"\n" +
									"aws rds wait db-snapshot-completed --region \"{{ .Profile.Location.Region }}\" --db-snapshot-identifier=\"{{ .Namespace.Name }}-{{ toDate \"2006-01-02T15:04:05.999999999Z07:00\" .Time  | date \"2006-01-02T15-04-05\" }}\" \n" +
									"\n" +
									"vpcsgid=$(aws rds describe-db-instances --db-instance-identifier=\"{{ index .ConfigMaps.dbconfig.Data \"postgres.instanceid\" }}\" --region \"{{ .Profile.Location.Region }}\" --query 'DBInstances[].VpcSecurityGroups[].VpcSecurityGroupId' --output text)\n" +
									"kando output securityGroupID $vpcsgid\n",
							},
						},
					},
				},
			},

			"restore": &crv1alpha1.BlueprintAction{
				Kind:               "Namespace",
				InputArtifactNames: []string{"snapshot"},
				Phases: []crv1alpha1.BlueprintPhase{
					crv1alpha1.BlueprintPhase{
						Func: "KubeTask",
						Name: "restoreSnapshot",
						Args: map[string]interface{}{
							"namespace": app.namespace,
							"image":     "kanisterio/postgres-kanister-tools:0.21.0",
							"command": []string{
								"bash",
								"-o",
								"errexit",
								"-o",
								"nounset",
								"-o",
								"xtrace",
								"-c",
								"set +o xtrace\n" +
									"export AWS_SECRET_ACCESS_KEY=\"{{ .Profile.Credential.KeyPair.Secret }}\"\n" +
									"export AWS_ACCESS_KEY_ID=\"{{ .Profile.Credential.KeyPair.ID }}\"\n" +
									"set -o xtrace\n" +
									"\n" +
									"# Delete old db instance\n" +
									"aws rds delete-db-instance --db-instance-identifier=\"{{ index .ConfigMaps.dbconfig.Data \"postgres.instanceid\" }}\" --skip-final-snapshot --region \"{{ .Profile.Location.Region }}\"\n" +
									"\n" +
									"aws rds wait db-instance-deleted --region \"{{ .Profile.Location.Region }}\" --db-instance-identifier=\"{{ index .ConfigMaps.dbconfig.Data \"postgres.instanceid\" }}\"\n" +
									"\n" +
									"# Restore instance from snapshot\n" +
									"aws rds restore-db-instance-from-db-snapshot --db-instance-identifier=\"{{ index .ConfigMaps.dbconfig.Data \"postgres.instanceid\" }}\" --db-snapshot-identifier=\"{{ .ArtifactsIn.snapshot.KeyValue.id }}\" --vpc-security-group-ids \"{{ .ArtifactsIn.snapshot.KeyValue.sgid }}\" --region \"{{ .Profile.Location.Region }}\"\n" +
									"aws rds wait db-instance-available --region \"{{ .Profile.Location.Region }}\" --db-instance-identifier=\"{{ index .ConfigMaps.dbconfig.Data \"postgres.instanceid\" }}\"\n",
							},
						},
					},
				},
			},

			"delete": &crv1alpha1.BlueprintAction{
				Kind:               "Namespace",
				InputArtifactNames: []string{"snapshot"},
				Phases: []crv1alpha1.BlueprintPhase{
					crv1alpha1.BlueprintPhase{
						Func: "KubeTask",
						Name: "deleteSnapshot",
						Args: map[string]interface{}{
							"namespace": app.namespace,
							"image":     "kanisterio/postgres-kanister-tools:0.21.0",
							"command": []string{
								"bash",
								"-o",
								"errexit",
								"-o",
								"nounset",
								"-o",
								"xtrace",
								"-c",
								"set +o xtrace\n" +
									"export AWS_SECRET_ACCESS_KEY=\"{{ .Profile.Credential.KeyPair.Secret }}\"\n" +
									"export AWS_ACCESS_KEY_ID=\"{{ .Profile.Credential.KeyPair.ID }}\"\n" +
									"set -o xtrace\n" +
									"aws rds delete-db-snapshot --db-snapshot-identifier=\"{{ .ArtifactsIn.snapshot.KeyValue.id }}\" --region \"{{ .Profile.Location.Region }}\"\n",
							},
						},
					},
				},
			},
		},
	}
	bp, err = app.crCli.Blueprints(controllerNs).Create(bp)
	return nil
}

func (app PostgresApp) Remove(ctx context.Context, ns string) error {
	return nil
}

func (app PostgresApp) ActionSet() *crv1alpha1.ActionSet {
	return &crv1alpha1.ActionSet{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-actionset-",
		},
		Spec: &crv1alpha1.ActionSetSpec{
			Actions: []crv1alpha1.ActionSpec{
				crv1alpha1.ActionSpec{
					Name: "backup",
					Object: crv1alpha1.ObjectReference{
						Kind:      "Namespace",
						Name:      app.namespace,
						Namespace: app.namespace,
					},
					Blueprint: "blueprint",
					Profile: &crv1alpha1.ObjectReference{
						Name:      "profile",
						Namespace: app.namespace,
					},
					ConfigMaps: map[string]crv1alpha1.ObjectReference{
						"dbconfig": crv1alpha1.ObjectReference{
							Name:      "dbconfig",
							Namespace: app.namespace,
						},
					},
					Secrets: map[string]crv1alpha1.ObjectReference{
						"dbsecret": crv1alpha1.ObjectReference{
							Name:      "dbsecret",
							Namespace: app.namespace,
						},
					},
				},
			},
		},
	}
}
