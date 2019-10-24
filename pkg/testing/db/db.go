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

import "context"

type DBType string

const (
	ManagedDB    DBType = "managed"
	SelfHostedDB DBType = "selfHosted"
)

// Instance contains Database instance
type Instance struct {
	Database
	Type DBType
}

// Database interface for managed or self hosted DBs
type Database interface {
	GetConfig(context.Context) error
	Install(context.Context, string) error
	CreateConfig(context.Context, string) error
	Remove(context.Context, string) error
	Data() Data
}

// Data interface to do read/write operations on Database
type Data interface {
	// Ping test connection with db
	Ping(context.Context) error
	// Insert n number of records
	Insert(context.Context, int) error
	// Count number of records
	Count(context.Context) (int, error)
	// Reset DB
	Reset(context.Context) error
}
