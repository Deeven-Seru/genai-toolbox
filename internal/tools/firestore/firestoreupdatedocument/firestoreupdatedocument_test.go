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

package firestoreupdatedocument

import (
	"context"
	"strings"
	"testing"

	firestoreapi "cloud.google.com/go/firestore"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	firestoreds "github.com/googleapis/mcp-toolbox/internal/sources/firestore"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	fsUtil "github.com/googleapis/mcp-toolbox/internal/tools/firestore/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestNewConfig(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    server.ToolConfigs
		wantErr bool
	}{
		{
			name: "valid config",
			yaml: `
			kind: tool
			name: test-update-document
			type: firestore-update-document
			source: test-firestore
			description: Update a document in Firestore
			authRequired:
			  - google-oauth
			`,
			want: server.ToolConfigs{
				"test-update-document": Config{
					Name:         "test-update-document",
					Type:         "firestore-update-document",
					Source:       "test-firestore",
					Description:  "Update a document in Firestore",
					AuthRequired: []string{"google-oauth"},
				},
			},
			wantErr: false,
		},
		{
			name: "minimal config",
			yaml: `
			kind: tool
			name: test-update-document
			type: firestore-update-document
			source: test-firestore
			description: Update a document
			`,
			want: server.ToolConfigs{
				"test-update-document": Config{
					Name:         "test-update-document",
					Type:         "firestore-update-document",
					Source:       "test-firestore",
					Description:  "Update a document",
					AuthRequired: []string{},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid yaml",
			yaml: `
			kind: tool
			name: test-update-document
			type: [invalid
			`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, got, _, _, err := server.UnmarshalResourceConfig(context.Background(), testutils.FormatYaml(tt.yaml))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("config mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestConfig_ToolConfigType(t *testing.T) {
	cfg := Config{}
	got := cfg.ToolConfigType()
	want := "firestore-update-document"
	if got != want {
		t.Fatalf("ToolConfigType() = %v, want %v", got, want)
	}
}

func TestConfig_Initialize(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		sources map[string]sources.Source
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid initialization",
			config: Config{
				Name:        "test-update-document",
				Type:        "firestore-update-document",
				Source:      "test-firestore",
				Description: "Update a document",
			},
			sources: map[string]sources.Source{
				"test-firestore": &firestoreds.Source{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool, err := tt.config.Initialize(tt.sources)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Fatalf("error message %q does not contain %q", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tool == nil {
				t.Fatalf("expected tool to be non-nil")
			}

			// Verify tool properties
			actualTool := tool.(Tool)
			if actualTool.Name != tt.config.Name {
				t.Fatalf("tool.Name = %v, want %v", actualTool.Name, tt.config.Name)
			}
			if actualTool.Type != "firestore-update-document" {
				t.Fatalf("tool.Type = %v, want %v", actualTool.Type, "firestore-update-document")
			}
			if diff := cmp.Diff(tt.config.AuthRequired, actualTool.AuthRequired); diff != "" {
				t.Fatalf("AuthRequired mismatch (-want +got):\n%s", diff)
			}
			if actualTool.Parameters == nil {
				t.Fatalf("expected Parameters to be non-nil")
			}
			if len(actualTool.Parameters) != 4 {
				t.Fatalf("len(Parameters) = %v, want 4", len(actualTool.Parameters))
			}
		})
	}
}

func TestTool_ParseParams(t *testing.T) {
	tool := Tool{
		Parameters: parameters.Parameters{
			parameters.NewStringParameter("documentPath", "Document path"),
			parameters.NewMapParameter("documentData", "Document data", ""),
			parameters.NewArrayParameterWithRequired("updateMask", "Update mask", false, parameters.NewStringParameter("field", "Field")),
			parameters.NewBooleanParameterWithDefault("returnData", false, "Return data"),
		},
	}

	tests := []struct {
		name    string
		data    map[string]any
		claims  map[string]map[string]any
		wantErr bool
	}{
		{
			name: "valid params with all fields",
			data: map[string]any{
				"documentPath": "users/user1",
				"documentData": map[string]any{
					"name": map[string]any{"stringValue": "John"},
				},
				"updateMask": []any{"name"},
				"returnData": true,
			},
			wantErr: false,
		},
		{
			name: "valid params without optional fields",
			data: map[string]any{
				"documentPath": "users/user1",
				"documentData": map[string]any{
					"name": map[string]any{"stringValue": "John"},
				},
			},
			wantErr: false,
		},
		{
			name: "missing required documentPath",
			data: map[string]any{
				"documentData": map[string]any{
					"name": map[string]any{"stringValue": "John"},
				},
			},
			wantErr: true,
		},
		{
			name: "missing required documentData",
			data: map[string]any{
				"documentPath": "users/user1",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, err := parameters.ParseParams(tool.GetParameters(), tt.data, tt.claims)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if params == nil {
				t.Fatalf("expected params to be non-nil")
			}
		})
	}
}

func TestTool_Manifest(t *testing.T) {
	tool := Tool{
		manifest: tools.Manifest{
			Description: "Test description",
			Parameters: []parameters.ParameterManifest{
				{
					Name:        "documentPath",
					Type:        "string",
					Description: "Document path",
					Required:    true,
				},
			},
			AuthRequired: []string{"google-oauth"},
		},
	}

	manifest := tool.Manifest()
	if manifest.Description != "Test description" {
		t.Fatalf("manifest.Description = %v, want %v", manifest.Description, "Test description")
	}
	if len(manifest.Parameters) != 1 {
		t.Fatalf("len(manifest.Parameters) = %v, want 1", len(manifest.Parameters))
	}
	if diff := cmp.Diff([]string{"google-oauth"}, manifest.AuthRequired); diff != "" {
		t.Fatalf("AuthRequired mismatch (-want +got):\n%s", diff)
	}
}

func TestTool_Authorized(t *testing.T) {
	tests := []struct {
		name                 string
		authRequired         []string
		verifiedAuthServices []string
		want                 bool
	}{
		{
			name:                 "no auth required",
			authRequired:         nil,
			verifiedAuthServices: nil,
			want:                 true,
		},
		{
			name:                 "auth required and provided",
			authRequired:         []string{"google-oauth"},
			verifiedAuthServices: []string{"google-oauth"},
			want:                 true,
		},
		{
			name:                 "auth required but not provided",
			authRequired:         []string{"google-oauth"},
			verifiedAuthServices: []string{"api-key"},
			want:                 false,
		},
		{
			name:                 "multiple auth required, one provided",
			authRequired:         []string{"google-oauth", "api-key"},
			verifiedAuthServices: []string{"google-oauth"},
			want:                 true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := Tool{
				Config: Config{
					AuthRequired: tt.authRequired,
				},
			}
			got := tool.Authorized(tt.verifiedAuthServices)
			if got != tt.want {
				t.Fatalf("Authorized() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetFieldValue(t *testing.T) {
	tests := []struct {
		name   string
		data   map[string]interface{}
		path   string
		want   interface{}
		exists bool
	}{
		{
			name: "simple field",
			data: map[string]interface{}{
				"name": "John",
			},
			path:   "name",
			want:   "John",
			exists: true,
		},
		{
			name: "nested field",
			data: map[string]interface{}{
				"user": map[string]interface{}{
					"name": "John",
				},
			},
			path:   "user.name",
			want:   "John",
			exists: true,
		},
		{
			name: "deeply nested field",
			data: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"level3": "value",
					},
				},
			},
			path:   "level1.level2.level3",
			want:   "value",
			exists: true,
		},
		{
			name: "non-existent field",
			data: map[string]interface{}{
				"name": "John",
			},
			path:   "age",
			want:   nil,
			exists: false,
		},
		{
			name: "non-existent nested field",
			data: map[string]interface{}{
				"user": map[string]interface{}{
					"name": "John",
				},
			},
			path:   "user.age",
			want:   nil,
			exists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, exists := getFieldValue(tt.data, tt.path)
			if exists != tt.exists {
				t.Fatalf("exists = %v, want %v", exists, tt.exists)
			}
			if tt.exists {
				if diff := cmp.Diff(tt.want, got); diff != "" {
					t.Fatalf("value mismatch (-want +got):\n%s", diff)
				}
			}
		})
	}
}

type mockFirestoreSource struct {
	sources.Source
	lastDocumentPath string
	lastUpdates      []firestoreapi.Update
	lastDocumentData any
	lastReturnData   bool
}

func (m *mockFirestoreSource) SourceType() string {
	return "firestore"
}

func (m *mockFirestoreSource) ToConfig() sources.SourceConfig {
	return nil
}

func (m *mockFirestoreSource) FirestoreClient() *firestoreapi.Client {
	return nil
}

func (m *mockFirestoreSource) UpdateDocument(_ context.Context, documentPath string, updates []firestoreapi.Update, documentData any, returnData bool) (map[string]any, error) {
	m.lastDocumentPath = documentPath
	m.lastUpdates = updates
	m.lastDocumentData = documentData
	m.lastReturnData = returnData
	return map[string]any{"status": "ok"}, nil
}

type mockSourceProvider struct {
	sourceName string
	source     sources.Source
}

func (m mockSourceProvider) GetSource(name string) (sources.Source, bool) {
	if name == m.sourceName {
		return m.source, true
	}
	return nil, false
}

func TestToolInvoke_AppliesVectorFieldsWithoutMask(t *testing.T) {
	cfg := Config{
		Name:        "update-docs",
		Type:        resourceType,
		Source:      "firestore-source",
		Description: "Update doc",
		VectorFields: []fsUtil.VectorFieldConfig{
			{
				Name:        "content_to_embed",
				Description: "text",
				FieldPath:   "embedding",
				EmbeddedBy:  "gemini",
			},
		},
	}

	toolIface, err := cfg.Initialize(nil)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	tool := toolIface.(Tool)

	firestoreSource := &mockFirestoreSource{}
	provider := mockSourceProvider{sourceName: cfg.Source, source: firestoreSource}

	params := parameters.ParamValues{
		{Name: documentPathKey, Value: "users/user1"},
		{Name: documentDataKey, Value: map[string]any{
			"mapValue": map[string]any{
				"fields": map[string]any{
					"title": map[string]any{"stringValue": "hello"},
				},
			},
		}},
		{Name: returnDocumentDataKey, Value: false},
		{Name: cfg.VectorFields[0].Name, Value: []float32{0.25, -0.25}},
	}

	if _, err := tool.Invoke(context.Background(), provider, params, ""); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}

	dataMap, ok := firestoreSource.lastDocumentData.(map[string]any)
	if !ok {
		t.Fatalf("expected document data map, got %T", firestoreSource.lastDocumentData)
	}
	vector, ok := dataMap["embedding"].([]float64)
	if !ok {
		t.Fatalf("expected embedding field to be present")
	}
	if diff := cmp.Diff([]float64{0.25, -0.25}, vector); diff != "" {
		t.Fatalf("embedding mismatch (-want +got):\n%s", diff)
	}
}

func TestToolInvoke_AppliesVectorFieldsWithUpdateMask(t *testing.T) {
	cfg := Config{
		Name:        "update-docs",
		Type:        resourceType,
		Source:      "firestore-source",
		Description: "Update doc",
		VectorFields: []fsUtil.VectorFieldConfig{
			{
				Name:        "content_to_embed",
				Description: "text",
				FieldPath:   "embedding",
				EmbeddedBy:  "gemini",
			},
		},
	}

	toolIface, err := cfg.Initialize(nil)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	tool := toolIface.(Tool)

	firestoreSource := &mockFirestoreSource{}
	provider := mockSourceProvider{sourceName: cfg.Source, source: firestoreSource}

	params := parameters.ParamValues{
		{Name: documentPathKey, Value: "users/user1"},
		{Name: documentDataKey, Value: map[string]any{
			"mapValue": map[string]any{
				"fields": map[string]any{
					"title": map[string]any{"stringValue": "hello"},
				},
			},
		}},
		{Name: updateMaskKey, Value: []any{"title"}},
		{Name: returnDocumentDataKey, Value: false},
		{Name: cfg.VectorFields[0].Name, Value: []float64{0.5, 0.75}},
	}

	if _, err := tool.Invoke(context.Background(), provider, params, ""); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}

	if firestoreSource.lastDocumentData != nil {
		t.Fatalf("expected document data to be nil for masked update")
	}
	if len(firestoreSource.lastUpdates) != 2 {
		t.Fatalf("expected two updates (scalar + vector), got %d", len(firestoreSource.lastUpdates))
	}

	foundVector := false
	for _, update := range firestoreSource.lastUpdates {
		if update.Path == "embedding" {
			vector, ok := update.Value.([]float64)
			if !ok {
				t.Fatalf("vector update has unexpected type %T", update.Value)
			}
			if diff := cmp.Diff([]float64{0.5, 0.75}, vector); diff != "" {
				t.Fatalf("vector update mismatch (-want +got):\n%s", diff)
			}
			foundVector = true
		}
	}
	if !foundVector {
		t.Fatalf("expected embedding field to be added to update mask")
	}
}
