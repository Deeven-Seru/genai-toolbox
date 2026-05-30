// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses///LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package autodiscover_test

import (
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools/autodiscover"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestGenerateCRUDTools(t *testing.T) {
	table := sources.TableSchema{
		TableName: "users",
		Columns: []sources.ColumnSchema{
			{ColumnName: "id", DataType: "serial", IsNullable: false, IsPrimaryKey: true},
			{ColumnName: "name", DataType: "text", IsNullable: false, IsPrimaryKey: false},
			{ColumnName: "email", DataType: "varchar", IsNullable: true, IsPrimaryKey: false},
			{ColumnName: "active", DataType: "boolean", IsNullable: false, IsPrimaryKey: false},
		},
	}

	generated, err := autodiscover.GenerateCRUDTools("test-db", "postgres", table)
	if err != nil {
		t.Fatalf("failed to generate CRUD tools: %v", err)
	}

	if len(generated) != 5 {
		t.Fatalf("expected 5 generated tools, got %d", len(generated))
	}

	expectedNames := map[string]bool{
		"list_users":         true,
		"get_users_by_id":    true,
		"insert_users":       true,
		"update_users_by_id": true,
		"delete_users_by_id": true,
	}

	for _, tool := range generated {
		name := tool.GetName()
		if !expectedNames[name] {
			t.Errorf("unexpected tool name generated: %s", name)
		}

		params := tool.GetParameters()

		switch name {
		case "list_users":
			if len(params) != 2 {
				t.Errorf("list_users should have 2 parameters, got %d", len(params))
			}
			limit := findParam(params, "limit")
			if limit == nil || limit.GetType() != parameters.TypeInt || limit.GetRequired() {
				t.Errorf("invalid limit parameter definition")
			}

		case "get_users_by_id":
			if len(params) != 1 {
				t.Errorf("get_users_by_id should have 1 parameter, got %d", len(params))
			}
			id := findParam(params, "id")
			if id == nil || id.GetType() != parameters.TypeInt || !id.GetRequired() {
				t.Errorf("invalid id parameter definition in get_users_by_id")
			}

		case "insert_users":
			// Columns: name, email, active (serial id is skipped)
			if len(params) != 3 {
				t.Errorf("insert_users should have 3 parameters, got %d", len(params))
			}
			nameParam := findParam(params, "name")
			if nameParam == nil || nameParam.GetType() != parameters.TypeString || !nameParam.GetRequired() {
				t.Errorf("invalid name parameter definition in insert_users")
			}
			emailParam := findParam(params, "email")
			if emailParam == nil || emailParam.GetType() != parameters.TypeString || emailParam.GetRequired() {
				t.Errorf("invalid email parameter definition in insert_users")
			}
			activeParam := findParam(params, "active")
			if activeParam == nil || activeParam.GetType() != parameters.TypeBool || !activeParam.GetRequired() {
				t.Errorf("invalid active parameter definition in insert_users")
			}

		case "update_users_by_id":
			// Columns to update: name, email, active, plus PK id
			if len(params) != 4 {
				t.Errorf("update_users_by_id should have 4 parameters, got %d", len(params))
			}
			idParam := findParam(params, "id")
			if idParam == nil || idParam.GetType() != parameters.TypeInt || !idParam.GetRequired() {
				t.Errorf("invalid id parameter definition in update_users_by_id")
			}
			nameParam := findParam(params, "name")
			if nameParam == nil || nameParam.GetType() != parameters.TypeString || nameParam.GetRequired() {
				t.Errorf("name parameter in update_users_by_id should be optional for partial updates")
			}

		case "delete_users_by_id":
			if len(params) != 1 {
				t.Errorf("delete_users_by_id should have 1 parameter, got %d", len(params))
			}
			id := findParam(params, "id")
			if id == nil || id.GetType() != parameters.TypeInt || !id.GetRequired() {
				t.Errorf("invalid id parameter definition in delete_users_by_id")
			}
		}
	}
}

func findParam(params parameters.Parameters, name string) parameters.Parameter {
	for _, p := range params {
		if p.GetName() == name {
			return p
		}
	}
	return nil
}
