// Copyright 2026 Google LLC
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

package autodiscover

import (
	"fmt"
	"strings"

	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/postgres/postgressql"
	"github.com/googleapis/mcp-toolbox/internal/tools/sqlite/sqlitesql"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

func mapDataTypeToParameter(colName, dataType string, required bool) parameters.Parameter {
	desc := fmt.Sprintf("Field %q of type %s", colName, dataType)

	switch strings.ToLower(dataType) {
	case "integer", "bigint", "smallint", "int", "serial", "bigserial":
		return parameters.NewIntParameterWithRequired(colName, desc, required)
	case "numeric", "double precision", "real", "decimal":
		return parameters.NewFloatParameterWithRequired(colName, desc, required)
	case "boolean", "bool":
		return parameters.NewBooleanParameterWithRequired(colName, desc, required)
	default:
		return parameters.NewStringParameterWithRequired(colName, desc, required)
	}
}

// GenerateCRUDTools dynamically generates 5 core operations (list, get, insert, update, delete) for a table.
func GenerateCRUDTools(sourceName string, sourceType string, table sources.TableSchema) ([]tools.Tool, error) {
	var generated []tools.Tool

	var primaryKeys []sources.ColumnSchema
	for _, col := range table.Columns {
		if col.IsPrimaryKey {
			primaryKeys = append(primaryKeys, col)
		}
	}

	// 1. list_<table>
	listTool, err := generateListTool(sourceName, sourceType, table)
	if err == nil {
		generated = append(generated, listTool)
	}

	// If there's exactly one primary key, we can generate the other PK-based operations
	if len(primaryKeys) == 1 {
		pk := primaryKeys[0]

		// 2. get_<table>_by_id
		getTool, err := generateGetTool(sourceName, sourceType, table, pk)
		if err == nil {
			generated = append(generated, getTool)
		}

		// 3. insert_<table>
		insertTool, err := generateInsertTool(sourceName, sourceType, table)
		if err == nil {
			generated = append(generated, insertTool)
		}

		// 4. update_<table>_by_id
		updateTool, err := generateUpdateTool(sourceName, sourceType, table, pk)
		if err == nil {
			generated = append(generated, updateTool)
		}

		// 5. delete_<table>_by_id
		deleteTool, err := generateDeleteTool(sourceName, sourceType, table, pk)
		if err == nil {
			generated = append(generated, deleteTool)
		}
	}

	return generated, nil
}

func initTool(name, sourceType, sourceName, desc, stmt string, params []parameters.Parameter, isReadOnly bool) (tools.Tool, error) {
	var annotations *tools.ToolAnnotations
	if isReadOnly {
		annotations = tools.NewReadOnlyAnnotations()
	} else {
		annotations = tools.NewDestructiveAnnotations()
	}

	if sourceType == "sqlite" {
		cfg := sqlitesql.Config{
			Name:        name,
			Type:        "sqlite-sql",
			Source:      sourceName,
			Description: desc,
			Statement:   stmt,
			Parameters:  params,
			Annotations: annotations,
		}
		return cfg.Initialize(nil)
	}

	// Default to postgres
	cfg := postgressql.Config{
		Name:        name,
		Type:        "postgres-sql",
		Source:      sourceName,
		Description: desc,
		Statement:   stmt,
		Parameters:  params,
		Annotations: annotations,
	}
	return cfg.Initialize(nil)
}

func quoteIdentifier(name string) string {
	return fmt.Sprintf("\"%s\"", strings.ReplaceAll(name, "\"", "\"\""))
}

func generateListTool(sourceName, sourceType string, table sources.TableSchema) (tools.Tool, error) {
	falseVal := false
	limitParam := parameters.NewIntParameterWithDefault("limit", 100, "Maximum number of records to return")
	limitParam.Required = &falseVal
	offsetParam := parameters.NewIntParameterWithDefault("offset", 0, "Number of records to skip")
	offsetParam.Required = &falseVal

	quotedTable := quoteIdentifier(table.TableName)
	return initTool(
		fmt.Sprintf("list_%s", table.TableName),
		sourceType,
		sourceName,
		fmt.Sprintf("Retrieve a list of records from %q", table.TableName),
		fmt.Sprintf("SELECT * FROM %s LIMIT $1 OFFSET $2", quotedTable),
		[]parameters.Parameter{
			limitParam,
			offsetParam,
		},
		true,
	)
}

func generateGetTool(sourceName, sourceType string, table sources.TableSchema, pk sources.ColumnSchema) (tools.Tool, error) {
	quotedTable := quoteIdentifier(table.TableName)
	quotedPK := quoteIdentifier(pk.ColumnName)
	return initTool(
		fmt.Sprintf("get_%s_by_id", table.TableName),
		sourceType,
		sourceName,
		fmt.Sprintf("Retrieve a single record from %q by its primary key", table.TableName),
		fmt.Sprintf("SELECT * FROM %s WHERE %s = $1", quotedTable, quotedPK),
		[]parameters.Parameter{
			mapDataTypeToParameter(pk.ColumnName, pk.DataType, true),
		},
		true,
	)
}

func generateInsertTool(sourceName, sourceType string, table sources.TableSchema) (tools.Tool, error) {
	var cols []string
	var placeholders []string
	var params []parameters.Parameter

	paramIndex := 1
	for _, col := range table.Columns {
		// Skip serial/autoincrement primary keys from insert parameters (database handles it)
		isAutoKey := strings.Contains(strings.ToLower(col.ColumnDefault), "nextval") ||
			strings.Contains(strings.ToLower(col.DataType), "serial") ||
			strings.Contains(strings.ToLower(col.DataType), "autoincrement") ||
			(sourceType == "sqlite" && col.IsPrimaryKey && strings.ToLower(col.DataType) == "integer")
		if col.IsPrimaryKey && isAutoKey {
			continue
		}

		cols = append(cols, quoteIdentifier(col.ColumnName))
		placeholders = append(placeholders, fmt.Sprintf("$%d", paramIndex))
		// Columns with default values should not be marked as required
		isRequired := !col.IsNullable && col.ColumnDefault == ""
		params = append(params, mapDataTypeToParameter(col.ColumnName, col.DataType, isRequired))
		paramIndex++
	}

	if len(cols) == 0 {
		return nil, fmt.Errorf("no columns to insert for table %q", table.TableName)
	}

	quotedTable := quoteIdentifier(table.TableName)
	statement := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING *",
		quotedTable,
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", "),
	)

	return initTool(
		fmt.Sprintf("insert_%s", table.TableName),
		sourceType,
		sourceName,
		fmt.Sprintf("Insert a new record into %q", table.TableName),
		statement,
		params,
		false,
	)
}

func generateUpdateTool(sourceName, sourceType string, table sources.TableSchema, pk sources.ColumnSchema) (tools.Tool, error) {
	var setClauses []string
	var params []parameters.Parameter

	paramIndex := 1
	for _, col := range table.Columns {
		if col.IsPrimaryKey {
			continue
		}

		quotedCol := quoteIdentifier(col.ColumnName)
		// Use COALESCE to keep the existing value if parameter is passed as NULL (partial updates)
		setClauses = append(setClauses, fmt.Sprintf("%s = COALESCE($%d, %s)", quotedCol, paramIndex, quotedCol))
		params = append(params, mapDataTypeToParameter(col.ColumnName, col.DataType, false))
		paramIndex++
	}

	if len(setClauses) == 0 {
		return nil, fmt.Errorf("no columns to update for table %q", table.TableName)
	}

	setClausesStr := strings.Join(setClauses, ", ")
	quotedTable := quoteIdentifier(table.TableName)
	quotedPK := quoteIdentifier(pk.ColumnName)
	statement := fmt.Sprintf("UPDATE %s SET %s WHERE %s = $%d RETURNING *",
		quotedTable,
		setClausesStr,
		quotedPK,
		paramIndex,
	)

	params = append(params, mapDataTypeToParameter(pk.ColumnName, pk.DataType, true))

	return initTool(
		fmt.Sprintf("update_%s_by_id", table.TableName),
		sourceType,
		sourceName,
		fmt.Sprintf("Update an existing record in %q by its primary key", table.TableName),
		statement,
		params,
		false,
	)
}

func generateDeleteTool(sourceName, sourceType string, table sources.TableSchema, pk sources.ColumnSchema) (tools.Tool, error) {
	quotedTable := quoteIdentifier(table.TableName)
	quotedPK := quoteIdentifier(pk.ColumnName)
	return initTool(
		fmt.Sprintf("delete_%s_by_id", table.TableName),
		sourceType,
		sourceName,
		fmt.Sprintf("Delete a record from %q by its primary key", table.TableName),
		fmt.Sprintf("DELETE FROM %s WHERE %s = $1 RETURNING *", quotedTable, quotedPK),
		[]parameters.Parameter{
			mapDataTypeToParameter(pk.ColumnName, pk.DataType, true),
		},
		false,
	)
}
