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

package bigquerysql

import (
	"testing"

	bigqueryapi "cloud.google.com/go/bigquery"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func TestBuildQueryParameters(t *testing.T) {
	required := false
	paramsMetadata := parameters.Parameters{
		&parameters.StringParameter{
			CommonParameter: parameters.CommonParameter{
				Name:     "opt_string",
				Type:     parameters.TypeString,
				Required: &required,
			},
		},
		&parameters.IntParameter{
			CommonParameter: parameters.CommonParameter{
				Name:     "opt_int",
				Type:     parameters.TypeInt,
				Required: &required,
			},
		},
		&parameters.FloatParameter{
			CommonParameter: parameters.CommonParameter{
				Name:     "opt_float",
				Type:     parameters.TypeFloat,
				Required: &required,
			},
		},
		&parameters.BooleanParameter{
			CommonParameter: parameters.CommonParameter{
				Name:     "opt_bool",
				Type:     parameters.TypeBool,
				Required: &required,
			},
		},
		&parameters.ArrayParameter{
			CommonParameter: parameters.CommonParameter{
				Name:     "opt_array",
				Type:     parameters.TypeArray,
				Required: &required,
			},
			Items: parameters.NewStringParameter("item", ""),
		},
	}

	paramsMap := map[string]any{
		// All are omitted
	}
	statement := "SELECT @opt_string, @opt_int, @opt_float, @opt_bool, @opt_array"

	gotHigh, gotLow, err := buildQueryParameters(paramsMetadata, paramsMap, statement)
	if err != nil {
		t.Fatalf("buildQueryParameters failed: %v", err)
	}

	wantHigh := []bigqueryapi.QueryParameter{
		{Name: "opt_string", Value: bigqueryapi.NullString{Valid: false}},
		{Name: "opt_int", Value: bigqueryapi.NullInt64{Valid: false}},
		{Name: "opt_float", Value: bigqueryapi.NullFloat64{Valid: false}},
		{Name: "opt_bool", Value: bigqueryapi.NullBool{Valid: false}},
		{Name: "opt_array", Value: []string(nil)},
	}

	if diff := cmp.Diff(wantHigh, gotHigh); diff != "" {
		t.Errorf("High-level parameters mismatch (-want +got):\n%s", diff)
	}

	// For low-level, we check the NullFields slice
	for i, p := range gotLow {
		foundNull := false
		for _, field := range p.ParameterValue.NullFields {
			if field == "Value" {
				foundNull = true
				break
			}
		}
		if !foundNull {
			t.Errorf("Low-level parameter %d (%s) NullFields does not contain 'Value', want true", i, p.Name)
		}
	}

	// Verify one non-null case
	paramsMapFull := map[string]any{
		"opt_string": "hello",
	}
	gotHighFull, gotLowFull, _ := buildQueryParameters(paramsMetadata, paramsMapFull, statement)
	
	if gotHighFull[0].Value != "hello" {
		t.Errorf("Expected string value 'hello', got %v", gotHighFull[0].Value)
	}
	if len(gotLowFull[0].ParameterValue.NullFields) > 0 {
		t.Error("Expected low-level NullFields to be empty for non-null value")
	}
	if gotLowFull[0].ParameterValue.Value != "hello" {
		t.Errorf("Expected low-level string value 'hello', got %s", gotLowFull[0].ParameterValue.Value)
	}
}

func TestBuildQueryParameters_Types(t *testing.T) {
	// Mixed cases
	required := false
	paramsMetadata := parameters.Parameters{
		&parameters.StringParameter{CommonParameter: parameters.CommonParameter{Name: "s", Type: "string", Required: &required}},
		&parameters.IntParameter{CommonParameter: parameters.CommonParameter{Name: "i", Type: "integer", Required: &required}},
	}
	paramsMap := map[string]any{
		"s": "val",
		// i is omitted
	}
	statement := "SELECT @s, @i"

	gotHigh, gotLow, _ := buildQueryParameters(paramsMetadata, paramsMap, statement)

	expectedHigh := []bigqueryapi.QueryParameter{
		{Name: "s", Value: "val"},
		{Name: "i", Value: bigqueryapi.NullInt64{Valid: false}},
	}

	if diff := cmp.Diff(expectedHigh, gotHigh, cmp.AllowUnexported(bigqueryapi.NullInt64{})); diff != "" {
		t.Errorf("High-level parameters mismatch (-want +got):\n%s", diff)
	}

	if len(gotLow[0].ParameterValue.NullFields) > 0 {
		t.Error("Expected low-level NullFields to be empty for 's'")
	}
	foundNull := false
	for _, field := range gotLow[1].ParameterValue.NullFields {
		if field == "Value" {
			foundNull = true
			break
		}
	}
	if !foundNull {
		t.Error("Expected low-level NullFields to contain 'Value' for 'i'")
	}
}
