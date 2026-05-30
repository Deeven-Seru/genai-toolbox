// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sqlite_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/sqlite"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/autodiscover"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestParseFromYamlSQLite(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		want server.SourceConfigs
	}{
		{
			desc: "basic example",
			in: `
            kind: source
            name: my-sqlite-db
            type: sqlite
            database: /path/to/database.db
            `,
			want: map[string]sources.SourceConfig{
				"my-sqlite-db": sqlite.Config{
					Name:     "my-sqlite-db",
					Type:     sqlite.SourceType,
					Database: "/path/to/database.db",
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			got, _, _, _, _, _, err := server.UnmarshalResourceConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if !cmp.Equal(tc.want, got) {
				t.Fatalf("incorrect parse: want %v, got %v", tc.want, got)
			}
		})
	}
}

func TestFailParseFromYaml(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		err  string
	}{
		{
			desc: "extra field",
			in: `
            kind: source
            name: my-sqlite-db
            type: sqlite
            database: /path/to/database.db
            foo: bar
            `,
			err: "error unmarshaling source: unable to parse source \"my-sqlite-db\" as \"sqlite\": [2:1] unknown field \"foo\"\n   1 | database: /path/to/database.db\n>  2 | foo: bar\n       ^\n   3 | name: my-sqlite-db\n   4 | type: sqlite",
		},
		{
			desc: "missing required field",
			in: `
            kind: source
            name: my-sqlite-db
            type: sqlite
            `,
			err: "error unmarshaling source: unable to parse source \"my-sqlite-db\" as \"sqlite\": Key: 'Config.Database' Error:Field validation for 'Database' failed on the 'required' tag",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, _, _, _, err := server.UnmarshalResourceConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err == nil {
				t.Fatalf("expect parsing to fail")
			}
			errStr := err.Error()
			if errStr != tc.err {
				t.Fatalf("unexpected error: got %q, want %q", errStr, tc.err)
			}
		})
	}
}

func TestSQLiteIntrospectionAndToolInvocation(t *testing.T) {
	// 1. Create a temporary SQLite database on disk
	tmpFile, err := os.CreateTemp("", "test_autodiscover_*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// 2. Initialize the SQLite connection
	cfg := sqlite.Config{
		Name:     "test-sqlite-db",
		Type:     sqlite.SourceType,
		Database: tmpFile.Name(),
		AutoDiscover: sqlite.AutoDiscoverConfig{
			Enabled: true,
		},
	}

	ctx := context.Background()
	src, err := cfg.Initialize(ctx, noop.NewTracerProvider().Tracer("noop"))
	if err != nil {
		t.Fatalf("failed to initialize SQLite source: %v", err)
	}
	sqliteSource := src.(*sqlite.Source)
	defer sqliteSource.Db.Close()

	// 3. Create a test table
	_, err = sqliteSource.Db.ExecContext(ctx, `
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			email TEXT,
			active BOOLEAN NOT NULL
		);
	`)
	if err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}

	// 4. Introspect schema
	introspectable := src.(sources.IntrospectableSource)
	tables, err := introspectable.DiscoverTables(ctx)
	if err != nil {
		t.Fatalf("failed to discover tables: %v", err)
	}

	if len(tables) != 1 {
		t.Fatalf("expected 1 table discovered, got %d", len(tables))
	}
	table := tables[0]
	if table.TableName != "users" {
		t.Errorf("expected table name 'users', got %q", table.TableName)
	}

	// 5. Generate dynamic CRUD tools
	generatedTools, err := autodiscover.GenerateCRUDTools("test-sqlite-db", "sqlite", table)
	if err != nil {
		t.Fatalf("failed to generate dynamic CRUD tools: %v", err)
	}

	if len(generatedTools) != 5 {
		t.Fatalf("expected 5 dynamic CRUD tools generated, got %d", len(generatedTools))
	}

	// 6. Register tools in a mock resource provider
	mockProvider := &mockSourceProvider{src: sqliteSource}

	// 7. Invoke the insert tool to actually write to the database!
	var insertTool tools.Tool
	for _, gt := range generatedTools {
		if gt.GetName() == "insert_users" {
			insertTool = gt
			break
		}
	}
	if insertTool == nil {
		t.Fatalf("insert_users tool not generated")
	}

	insertParams, err := parameters.ParseParams(
		insertTool.GetParameters(),
		map[string]any{
			"name":   "Mike Ross",
			"email":  "mike@pearson-specter.com",
			"active": true,
		},
		nil,
	)
	if err != nil {
		t.Fatalf("failed to parse parameters for insert: %v", err)
	}

	resp, invokeErr := insertTool.Invoke(ctx, mockProvider, insertParams, "")
	if invokeErr != nil {
		t.Fatalf("failed to invoke insert tool: %v", invokeErr)
	}

	// Verify that the record was actually inserted and returned!
	rows, ok := resp.([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("expected response to be a slice with 1 row, got %v", resp)
	}

	// 8. Invoke the list tool to read it back!
	var listTool tools.Tool
	for _, gt := range generatedTools {
		if gt.GetName() == "list_users" {
			listTool = gt
			break
		}
	}
	if listTool == nil {
		t.Fatalf("list_users tool not generated")
	}

	listParams, err := parameters.ParseParams(
		listTool.GetParameters(),
		map[string]any{
			"limit":  10,
			"offset": 0,
		},
		nil,
	)
	if err != nil {
		t.Fatalf("failed to parse parameters for list: %v", err)
	}

	listResp, invokeErr := listTool.Invoke(ctx, mockProvider, listParams, "")
	if invokeErr != nil {
		t.Fatalf("failed to invoke list tool: %v", invokeErr)
	}

	listRows, ok := listResp.([]any)
	if !ok || len(listRows) != 1 {
		t.Fatalf("expected list response to return 1 row, got %v", listResp)
	}
}

type mockSourceProvider struct {
	src sources.Source
}

func (m *mockSourceProvider) GetSource(name string) (sources.Source, bool) {
	return m.src, true
}
