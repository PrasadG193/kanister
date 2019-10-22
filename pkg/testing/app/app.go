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
)

type Kanister interface {
	// Install creates Kanister CRs profiles and blueprint
	Install(context.Context, string) error
	// Remove deletes Kanister CRs like profile and blueprint
	Remove(context.Context) error
	// Actionset returns actionset specs to Backup the data
	ActionSet() *crv1alpha1.ActionSet
}
