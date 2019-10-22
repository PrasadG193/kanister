// +build integration
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

package testing

import (
	"context"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	. "gopkg.in/check.v1"
	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	crv1alpha1 "github.com/kanisterio/kanister/pkg/apis/cr/v1alpha1"
	crclient "github.com/kanisterio/kanister/pkg/client/clientset/versioned/typed/cr/v1alpha1"
	"github.com/kanisterio/kanister/pkg/controller"
	_ "github.com/kanisterio/kanister/pkg/function"
	"github.com/kanisterio/kanister/pkg/kanctl"
	"github.com/kanisterio/kanister/pkg/kube"
	"github.com/kanisterio/kanister/pkg/poll"
	"github.com/kanisterio/kanister/pkg/resource"
	"github.com/kanisterio/kanister/pkg/testing/app"
	"github.com/kanisterio/kanister/pkg/testing/db"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type test struct {
	database    db.Instance
	application app.Kanister
	namespace   string
}

type IntegrationSuite struct {
	cli          kubernetes.Interface
	crCli        crclient.CrV1alpha1Interface
	tests        map[string]test
	cancel       context.CancelFunc
	controllerNs string
}

var _ = Suite(&IntegrationSuite{
	tests: make(map[string]test),
})

func (s *IntegrationSuite) SetUpSuite(c *C) {
	ctx := context.Background()
	ctx, s.cancel = context.WithCancel(ctx)

	s.controllerNs = "e2e-test"

	// Instantiate Client SDKs
	cfg, err := kube.LoadConfig()
	c.Assert(err, IsNil)
	s.cli, err = kubernetes.NewForConfig(cfg)
	c.Assert(err, IsNil)
	s.crCli, err = crclient.NewForConfig(cfg)
	c.Assert(err, IsNil)

	postgresDB, err := db.NewPostgresDB(s.cli)
	c.Assert(err, IsNil)

	// Add new tests in the map
	s.tests = map[string]test{
		"rds-postgres": test{
			database:    db.Instance{Type: db.ManagedDB, Database: postgresDB},
			application: app.NewPostgresApp(s.cli, s.crCli, "e2e-rds-postgres"),
			namespace:   "e2e-rds-postgres",
		},
	}

	// Create a new test namespace
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: s.controllerNs,
		},
	}
	_, err = s.cli.CoreV1().Namespaces().Create(ns)
	c.Assert(err, IsNil)

	for name, t := range s.tests {
		// Create a new test namespace
		log.Infof("Creating DB for %s", name)
		c.Assert(err, IsNil)

		// install db
		err := t.database.Install(ctx, t.namespace)
		c.Assert(err, IsNil)

		// create configmap and secret for in case of Managed DB
		if t.database.Type == db.ManagedDB {
			err := t.database.CreateConfig(ctx, t.namespace)
			c.Assert(err, IsNil)
		}
	}

	// Start the controller
	err = resource.CreateCustomResources(ctx, cfg)
	c.Assert(err, IsNil)
	ctlr := controller.New(cfg)
	err = ctlr.StartWatch(ctx, s.controllerNs)
	c.Assert(err, IsNil)
}

func (s *IntegrationSuite) TestRun(c *C) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Exec methods
	for name, t := range s.tests {
		log.Infof("Connection info for %s", name)

		// Install Application
		err := t.application.Install(ctx, s.controllerNs)
		c.Assert(err, IsNil)

		// Check connection
		data := t.database.Data()
		err = data.Ping(ctx)
		c.Assert(err, IsNil)

		err = data.Reset(ctx)
		c.Assert(err, IsNil)

		// Add few entries
		err = data.Insert(ctx, 3)
		c.Assert(err, IsNil)

		count, err := data.Count(ctx)
		c.Assert(err, IsNil)
		c.Assert(count, Equals, 3)

		// Take backup
		backup := s.createActionset(ctx, t.application.ActionSet(), t.namespace, "backup", c)

		// Reset DB
		err = data.Reset(ctx)
		c.Assert(err, IsNil)

		// Restore backup
		c.Assert(len(backup), Not(Equals), 0)
		pas, err := s.crCli.ActionSets(s.controllerNs).Get(backup, metav1.GetOptions{})
		c.Assert(err, IsNil)
		s.createActionset(ctx, pas, t.namespace, "restore", c)

		// Verify data
		err = data.Ping(ctx)
		c.Assert(err, IsNil)

		count, err = data.Count(ctx)
		c.Assert(err, IsNil)
		c.Assert(count, Equals, 3)

		// Delete snapshots
		s.createActionset(ctx, pas, t.namespace, "delete", c)
		err = t.application.Remove(ctx)
		c.Assert(c, IsNil)
	}
}

func (s *IntegrationSuite) createActionset(ctx context.Context, as *crv1alpha1.ActionSet, appNs, action string, c *C) string {
	var err error
	switch action {
	case "backup":
		as, err = s.crCli.ActionSets(s.controllerNs).Create(as)
		c.Assert(err, IsNil)
	case "restore", "delete":
		as, err = restoreActionSetSpecs(as, action)
		c.Assert(err, IsNil)
		as, err = s.crCli.ActionSets(s.controllerNs).Create(as)
		c.Assert(err, IsNil)
	default:
		c.Errorf("Invalid action %s while creating ActionSet", action)
	}

	// Wait for the ActionSet to complete.
	err = poll.Wait(ctx, func(ctx context.Context) (bool, error) {
		as, err = s.crCli.ActionSets(s.controllerNs).Get(as.GetName(), metav1.GetOptions{})
		switch {
		case err != nil, as.Status == nil:
			return false, err
		case as.Status.State == crv1alpha1.StateFailed:
			return true, errors.Errorf("Actionset failed: %#v", as.Status)
		case as.Status.State == crv1alpha1.StateComplete:
			return true, nil
		}
		return false, nil
	})
	c.Assert(err, IsNil)
	return as.GetName()
}

func restoreActionSetSpecs(from *crv1alpha1.ActionSet, action string) (*crv1alpha1.ActionSet, error) {
	params := kanctl.PerformParams{
		ActionName: action,
		ParentName: from.GetName(),
	}
	return kanctl.ChildActionSet(from, &params)
}

func (s *IntegrationSuite) TearDownSuite(c *C) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for name, t := range s.tests {
		log.Infof("Deleting DB for %s", name)
		t.database.Remove(ctx, t.namespace)
		if len(t.namespace) != 0 {
			s.cli.CoreV1().Namespaces().Delete(t.namespace, nil)
		}
	}

	// Delete e2e-test ns
	s.cli.CoreV1().Namespaces().Delete(s.controllerNs, nil)
	if s.cancel != nil {
		s.cancel()
	}
}
