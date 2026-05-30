// Copyright 2024 Google LLC
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

package postgres

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/sources/sqlcommenter"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/orderedmap"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/trace"
)

const SourceType string = "postgres"

// validate interface
var _ sources.SourceConfig = Config{}

func init() {
	if !sources.Register(SourceType, newConfig) {
		panic(fmt.Sprintf("source type %q already registered", SourceType))
	}
}

func newConfig(ctx context.Context, name string, decoder *yaml.Decoder) (sources.SourceConfig, error) {
	actual := Config{Name: name}
	if err := decoder.DecodeContext(ctx, &actual); err != nil {
		return nil, err
	}
	return actual, nil
}

type AutoDiscoverConfig struct {
	Enabled       bool     `yaml:"enabled"`
	IncludeTables []string `yaml:"includeTables"`
	ExcludeTables []string `yaml:"excludeTables"`
}

type Config struct {
	Name          string             `yaml:"name" validate:"required"`
	Type          string             `yaml:"type" validate:"required"`
	Host          string             `yaml:"host" validate:"required"`
	Port          string             `yaml:"port" validate:"required"`
	User          string             `yaml:"user" validate:"required"`
	Password      string             `yaml:"password" validate:"required"`
	Database      string             `yaml:"database" validate:"required"`
	QueryParams   map[string]string  `yaml:"queryParams"`
	QueryExecMode string             `yaml:"queryExecMode" validate:"omitempty,oneof=cache_statement cache_describe describe_exec exec simple_protocol"`
	AutoDiscover  AutoDiscoverConfig `yaml:"autoDiscover"`
}

func (r Config) SourceConfigType() string {
	return SourceType
}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer) (sources.Source, error) {
	pool, err := initPostgresConnectionPool(ctx, tracer, r.Name, r.Host, r.Port, r.User, r.Password, r.Database, r.QueryParams, r.QueryExecMode)
	if err != nil {
		return nil, fmt.Errorf("unable to create pool: %w", err)
	}

	err = pool.Ping(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to connect successfully: %w", err)
	}

	s := &Source{
		Config: r,
		Pool:   pool,
	}
	return s, nil
}

var _ sources.Source = &Source{}
var _ sources.IntrospectableSource = &Source{}

type Source struct {
	Config
	Pool *pgxpool.Pool
}

func (s *Source) SourceType() string {
	return SourceType
}

// IsAutoDiscoverEnabled returns true if auto-discovery is enabled for this source.
func (s *Source) IsAutoDiscoverEnabled() bool {
	return s.AutoDiscover.Enabled
}

// DiscoverTables queries information_schema to return tables and columns schema metadata.
func (s *Source) DiscoverTables(ctx context.Context) ([]sources.TableSchema, error) {
	// Query all base tables in public schema
	tableQuery := `
		SELECT table_name 
		FROM information_schema.tables 
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
	`
	rows, err := s.Pool.Query(ctx, tableQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tables: %w", err)
	}
	defer rows.Close()

	var allTables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, err
		}
		if s.shouldIncludeTable(tableName) {
			allTables = append(allTables, tableName)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(allTables) == 0 {
		return nil, nil
	}

	// Query column information and PK constraints for all found tables
	columnQuery := `
		SELECT 
			c.table_name,
			c.column_name, 
			c.data_type, 
			c.is_nullable = 'YES' AS is_nullable,
			(pk.column_name IS NOT NULL) AS is_primary_key,
			COALESCE(c.column_default, '') AS column_default
		FROM information_schema.columns c
		LEFT JOIN (
			SELECT kcu.table_name, kcu.column_name
			FROM information_schema.table_constraints tc
			JOIN information_schema.key_column_usage kcu
				ON tc.constraint_name = kcu.constraint_name
				AND tc.table_schema = kcu.table_schema
			WHERE tc.constraint_type = 'PRIMARY KEY'
				AND tc.table_schema = 'public'
		) pk ON c.table_name = pk.table_name AND c.column_name = pk.column_name
		WHERE c.table_schema = 'public' AND c.table_name = ANY($1)
		ORDER BY c.table_name, c.ordinal_position;
	`
	colRows, err := s.Pool.Query(ctx, columnQuery, allTables)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch columns: %w", err)
	}
	defer colRows.Close()

	tableMap := make(map[string][]sources.ColumnSchema)
	for colRows.Next() {
		var tableName, columnName, dataType, columnDefault string
		var isNullable, isPrimaryKey bool
		if err := colRows.Scan(&tableName, &columnName, &dataType, &isNullable, &isPrimaryKey, &columnDefault); err != nil {
			return nil, err
		}
		tableMap[tableName] = append(tableMap[tableName], sources.ColumnSchema{
			ColumnName:    columnName,
			DataType:      dataType,
			IsNullable:    isNullable,
			IsPrimaryKey:  isPrimaryKey,
			ColumnDefault: columnDefault,
		})
	}
	if err := colRows.Err(); err != nil {
		return nil, err
	}

	var schemas []sources.TableSchema
	for _, tableName := range allTables {
		if cols, ok := tableMap[tableName]; ok {
			schemas = append(schemas, sources.TableSchema{
				TableName: tableName,
				Columns:   cols,
			})
		}
	}

	return schemas, nil
}

func (s *Source) shouldIncludeTable(tableName string) bool {
	include := s.AutoDiscover.IncludeTables
	exclude := s.AutoDiscover.ExcludeTables

	if len(include) > 0 {
		found := false
		for _, inc := range include {
			if strings.EqualFold(inc, tableName) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(exclude) > 0 {
		for _, exc := range exclude {
			if strings.EqualFold(exc, tableName) {
				return false
			}
		}
	}

	return true
}

func (s *Source) ToConfig() sources.SourceConfig {
	return s.Config
}

func (s *Source) PostgresPool() *pgxpool.Pool {
	return s.Pool
}

func (s *Source) RunSQL(ctx context.Context, statement string, params []any) (any, error) {
	statement = sqlcommenter.AppendComment(ctx, statement, SourceType)
	results, err := s.PostgresPool().Query(ctx, statement, params...)
	if err != nil {
		return nil, fmt.Errorf("unable to execute query: %w", err)
	}
	defer results.Close()

	fields := results.FieldDescriptions()
	out := []any{}
	for results.Next() {
		values, err := results.Values()
		if err != nil {
			return nil, fmt.Errorf("unable to parse row: %w", err)
		}
		row := orderedmap.Row{}
		for i, f := range fields {
			row.Add(f.Name, values[i])
		}
		out = append(out, row)
	}
	// this will catch actual query execution errors
	if err := results.Err(); err != nil {
		return nil, fmt.Errorf("unable to execute query: %w", err)
	}
	return out, nil
}

func initPostgresConnectionPool(ctx context.Context, tracer trace.Tracer, name, host, port, user, pass, dbname string, queryParams map[string]string, queryExecMode string) (*pgxpool.Pool, error) {
	//nolint:all // Reassigned ctx
	ctx, span := sources.InitConnectionSpan(ctx, tracer, SourceType, name)
	defer span.End()
	userAgent, err := util.UserAgentFromContext(ctx)
	if err != nil {
		userAgent = "genai-toolbox"
	}
	if queryParams == nil {
		// Initialize the map before using it
		queryParams = make(map[string]string)
	}
	if _, ok := queryParams["application_name"]; !ok {
		queryParams["application_name"] = userAgent
	}

	config, err := pgxpool.ParseConfig(BuildPostgresURL(host, port, user, pass, dbname, queryParams))
	if err != nil {
		return nil, fmt.Errorf("unable to parse connection uri: %w", err)
	}

	execMode, err := ParseQueryExecMode(queryExecMode)
	if err != nil {
		return nil, err
	}
	config.ConnConfig.DefaultQueryExecMode = execMode

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection pool: %w", err)
	}

	return pool, nil
}

// BuildPostgresURL assembles a postgres connection URL from its components.
// It uses net.JoinHostPort so IPv6 host literals are wrapped in brackets as
// required by RFC 3986 (e.g. "[::1]:5432"); IPv4 addresses and hostnames are
// left unchanged. Query parameters are encoded with url.Values so special
// characters are escaped correctly and the output is deterministic.
func BuildPostgresURL(host, port, user, pass, dbname string, queryParams map[string]string) string {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, pass),
		Host:   net.JoinHostPort(host, port),
		Path:   dbname,
	}
	if len(queryParams) > 0 {
		q := url.Values{}
		for k, v := range queryParams {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
	}
	return u.String()
}

func ParseQueryExecMode(queryExecMode string) (pgx.QueryExecMode, error) {
	switch queryExecMode {
	case "", "cache_statement":
		return pgx.QueryExecModeCacheStatement, nil
	case "cache_describe":
		return pgx.QueryExecModeCacheDescribe, nil
	case "describe_exec":
		return pgx.QueryExecModeDescribeExec, nil
	case "exec":
		return pgx.QueryExecModeExec, nil
	case "simple_protocol":
		return pgx.QueryExecModeSimpleProtocol, nil
	default:
		return 0, fmt.Errorf("invalid queryExecMode %q: must be one of %q, %q, %q, %q, or %q", queryExecMode, "cache_statement", "cache_describe", "describe_exec", "exec", "simple_protocol")
	}
}
