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

// NOTE:
// Prerequisites:
// - To use blueprint method - "blueprint.ReadFromFile()",
//   one needs to create symlink to the kanister/pkg/blueprints dir where main pkg exists.
// - In case of test files, create symlink in the pkg where test files are placed
// - Symlink should be created with the name of "blueprints" and relative path should be given
//   e.g if you have to use this pkg in tests of pkg/testing pkg, the command should look like -
//   "ln -sf ../../pkg/blueprint/blueprints blueprints"

package blueprint

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"k8s.io/apimachinery/pkg/util/yaml"

	crv1alpha1 "github.com/kanisterio/kanister/pkg/apis/cr/v1alpha1"
)

const (
	blueprintsRepo = "./blueprints"
)

// ReadFromFile parsed and returns Blueprint specs placed at blueprints/{app}-blueprint.yaml
func ReadFromFile(app string) (*crv1alpha1.Blueprint, error) {
	path, err := filepath.Abs(fmt.Sprintf("%s/%s-blueprint.yaml", blueprintsRepo, app))
	if err != nil {
		return nil, err
	}
	bpRaw, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var bp crv1alpha1.Blueprint
	dec := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(bpRaw), 1000)
	if err := dec.Decode(&bp); err != nil {
		return nil, err
	}
	return &bp, err
}
