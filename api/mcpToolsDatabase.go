package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// migrationApplier is implemented by branchers that can record migrations
// (currently provisioner.DBBrancher).
type migrationApplier interface {
	ApplyMigration(ctx context.Context, projectID, dbName, version, name, sql string) error
}

type mcpDatabaseIn struct {
	ProjectId string `json:"project_id,omitempty" jsonschema:"the project ref (omit when the server is scoped to a project)"`
	Branch    string `json:"branch,omitempty" jsonschema:"optional branch name to target instead of the main database"`
}

func (a *Api) registerDatabaseTools(server *mcp.Server, scopedRef string) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_tables",
		Description: "Lists all tables in the project's database, with schema, row estimate and RLS status.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in mcpDatabaseIn) (*mcp.CallToolResult, any, error) {
		project, err := a.mcpProject(ctx, req, in.ProjectId, scopedRef)
		if err != nil {
			return nil, nil, err
		}
		dbName, err := a.mcpResolveDB(ctx, project.ProjectRef, in.Branch)
		if err != nil {
			return nil, nil, err
		}
		out, err := a.mcpExecSQL(ctx, project.ProjectRef, dbName, `
			SELECT n.nspname AS schema, c.relname AS name,
			       c.reltuples::bigint AS estimated_rows,
			       c.relrowsecurity AS rls_enabled
			FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE c.relkind = 'r'
			  AND n.nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
			ORDER BY n.nspname, c.relname`)
		if err != nil {
			return nil, nil, err
		}
		return textResult(out), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_extensions",
		Description: "Lists all installed extensions in the project's database.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in mcpDatabaseIn) (*mcp.CallToolResult, any, error) {
		project, err := a.mcpProject(ctx, req, in.ProjectId, scopedRef)
		if err != nil {
			return nil, nil, err
		}
		dbName, err := a.mcpResolveDB(ctx, project.ProjectRef, in.Branch)
		if err != nil {
			return nil, nil, err
		}
		out, err := a.mcpExecSQL(ctx, project.ProjectRef, dbName, `
			SELECT e.extname AS name, e.extversion AS version, n.nspname AS schema
			FROM pg_extension e
			JOIN pg_namespace n ON n.oid = e.extnamespace
			ORDER BY e.extname`)
		if err != nil {
			return nil, nil, err
		}
		return textResult(out), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_migrations",
		Description: "Lists all migrations applied to the project's database.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in mcpDatabaseIn) (*mcp.CallToolResult, any, error) {
		project, err := a.mcpProject(ctx, req, in.ProjectId, scopedRef)
		if err != nil {
			return nil, nil, err
		}
		dbName, err := a.mcpResolveDB(ctx, project.ProjectRef, in.Branch)
		if err != nil {
			return nil, nil, err
		}
		out, err := a.mcpExecSQL(ctx, project.ProjectRef, dbName, `
			SELECT version, coalesce(name, '') AS name
			FROM supabase_migrations.schema_migrations
			ORDER BY version`)
		if err != nil {
			// The migrations table may not exist yet on fresh projects
			if strings.Contains(err.Error(), "does not exist") {
				return textResult("version,name\n"), nil, nil
			}
			return nil, nil, err
		}
		return textResult(out), nil, nil
	})

	type applyMigrationIn struct {
		ProjectId string `json:"project_id,omitempty" jsonschema:"the project ref (omit when the server is scoped to a project)"`
		Branch    string `json:"branch,omitempty" jsonschema:"optional branch name to target instead of the main database"`
		Name      string `json:"name" jsonschema:"the name of the migration in snake_case"`
		Query     string `json:"query" jsonschema:"the SQL query to apply"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "apply_migration",
		Description: "Applies a migration to the database and records it in the migration history. Use for DDL operations.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in applyMigrationIn) (*mcp.CallToolResult, any, error) {
		project, err := a.mcpProjectWithRole(ctx, req, in.ProjectId, scopedRef, "owner", "admin", "member")
		if err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(in.Query) == "" {
			return nil, nil, errors.New("query is required")
		}
		dbName, err := a.mcpResolveDB(ctx, project.ProjectRef, in.Branch)
		if err != nil {
			return nil, nil, err
		}

		applier, ok := a.brancher.(migrationApplier)
		if !ok || a.brancher == nil {
			return nil, nil, errors.New("migrations are not available (provisioner disabled)")
		}

		version := time.Now().UTC().Format("20060102150405")
		if err := applier.ApplyMigration(ctx, project.ProjectRef, dbName, version, in.Name, in.Query); err != nil {
			return nil, nil, err
		}
		return textResult(fmt.Sprintf("Migration %s (%s) applied to %s", version, in.Name, dbName)), nil, nil
	})

	type executeSQLIn struct {
		ProjectId string `json:"project_id,omitempty" jsonschema:"the project ref (omit when the server is scoped to a project)"`
		Branch    string `json:"branch,omitempty" jsonschema:"optional branch name to target instead of the main database"`
		Query     string `json:"query" jsonschema:"the SQL query to execute"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "execute_sql",
		Description: "Executes raw SQL in the project's database. Use apply_migration for DDL; results are returned as CSV.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in executeSQLIn) (*mcp.CallToolResult, any, error) {
		project, err := a.mcpProjectWithRole(ctx, req, in.ProjectId, scopedRef, "owner", "admin", "member")
		if err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(in.Query) == "" {
			return nil, nil, errors.New("query is required")
		}
		dbName, err := a.mcpResolveDB(ctx, project.ProjectRef, in.Branch)
		if err != nil {
			return nil, nil, err
		}
		out, err := a.mcpExecSQL(ctx, project.ProjectRef, dbName, in.Query)
		if err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(out) == "" {
			out = "(no rows returned)"
		}
		return textResult(out), nil, nil
	})
}

// advisorCheck is one lint the get_advisors tool runs against a project DB.
type advisorCheck struct {
	Name        string
	Level       string // ERROR | WARN | INFO
	Category    string // SECURITY | PERFORMANCE
	Description string
	Remediation string
	SQL         string // returns one row per finding, first column is the object name
}

var advisorChecks = []advisorCheck{
	{
		Name:        "rls_disabled_in_public",
		Level:       "ERROR",
		Category:    "SECURITY",
		Description: "Table is exposed via the API but Row Level Security is not enabled",
		Remediation: "ALTER TABLE <table> ENABLE ROW LEVEL SECURITY; then add policies",
		SQL: `SELECT n.nspname || '.' || c.relname
		      FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		      WHERE c.relkind = 'r' AND n.nspname = 'public' AND NOT c.relrowsecurity`,
	},
	{
		Name:        "rls_enabled_no_policy",
		Level:       "WARN",
		Category:    "SECURITY",
		Description: "Row Level Security is enabled but the table has no policies (all access blocked for non-service roles)",
		Remediation: "CREATE POLICY statements to grant intended access",
		SQL: `SELECT n.nspname || '.' || c.relname
		      FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		      WHERE c.relkind = 'r' AND n.nspname = 'public' AND c.relrowsecurity
		        AND NOT EXISTS (SELECT 1 FROM pg_policy p WHERE p.polrelid = c.oid)`,
	},
	{
		Name:        "unindexed_foreign_keys",
		Level:       "INFO",
		Category:    "PERFORMANCE",
		Description: "Foreign key has no covering index, which slows joins and cascades",
		Remediation: "CREATE INDEX on the referencing column(s)",
		SQL: `SELECT conrelid::regclass || ' (' || con.conname || ')'
		      FROM pg_constraint con
		      WHERE con.contype = 'f'
		        AND NOT EXISTS (
		          SELECT 1 FROM pg_index i
		          WHERE i.indrelid = con.conrelid
		            AND (i.indkey::int2[])[0:array_length(con.conkey,1)-1] @> con.conkey::int2[]
		        )`,
	},
	{
		Name:        "extensions_in_public_schema",
		Level:       "WARN",
		Category:    "SECURITY",
		Description: "Extension is installed in the public schema, exposing its functions via the API",
		Remediation: "Reinstall the extension into a dedicated schema (e.g. extensions)",
		SQL: `SELECT e.extname
		      FROM pg_extension e JOIN pg_namespace n ON n.oid = e.extnamespace
		      WHERE n.nspname = 'public' AND e.extname NOT IN ('plpgsql')`,
	},
}

func (a *Api) registerDebuggingTools(server *mcp.Server, scopedRef string) {
	type getLogsIn struct {
		ProjectId string `json:"project_id,omitempty" jsonschema:"the project ref (omit when the server is scoped to a project)"`
		Service   string `json:"service" jsonschema:"the service to fetch logs for: db, auth, rest, realtime, storage, kong, edge-functions, studio, meta, analytics"`
		Tail      int    `json:"tail,omitempty" jsonschema:"number of log lines to return (default 100)"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_logs",
		Description: "Fetches logs for a project service.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in getLogsIn) (*mcp.CallToolResult, any, error) {
		project, err := a.mcpProject(ctx, req, in.ProjectId, scopedRef)
		if err != nil {
			return nil, nil, err
		}
		if a.provisioner == nil {
			return nil, nil, errors.New("provisioner is not available")
		}
		tail := in.Tail
		if tail <= 0 {
			tail = 100
		}
		lines, err := a.provisioner.GetLogs(ctx, project.ProjectRef, in.Service, tail)
		if err != nil {
			return nil, nil, err
		}
		return textResult(strings.Join(lines, "\n")), nil, nil
	})

	type getAdvisorsIn struct {
		ProjectId string `json:"project_id,omitempty" jsonschema:"the project ref (omit when the server is scoped to a project)"`
		Type      string `json:"type,omitempty" jsonschema:"filter by advisor type: security or performance (default: both)"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_advisors",
		Description: "Runs security and performance lint checks against the project's database and reports findings.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in getAdvisorsIn) (*mcp.CallToolResult, any, error) {
		project, err := a.mcpProject(ctx, req, in.ProjectId, scopedRef)
		if err != nil {
			return nil, nil, err
		}

		type finding struct {
			Name        string   `json:"name"`
			Level       string   `json:"level"`
			Category    string   `json:"category"`
			Description string   `json:"description"`
			Remediation string   `json:"remediation"`
			Objects     []string `json:"objects"`
		}
		findings := []finding{}

		for _, check := range advisorChecks {
			if in.Type != "" && !strings.EqualFold(in.Type, check.Category) {
				continue
			}
			out, err := a.mcpExecSQL(ctx, project.ProjectRef, "postgres", check.SQL)
			if err != nil {
				a.logger.Warn("Advisor check failed", "check", check.Name, "project", project.ProjectRef, "error", err.Error())
				continue
			}
			var objects []string
			for i, line := range strings.Split(strings.TrimSpace(out), "\n") {
				if i == 0 {
					continue // CSV header
				}
				if line = strings.TrimSpace(line); line != "" {
					objects = append(objects, line)
				}
			}
			if len(objects) > 0 {
				findings = append(findings, finding{
					Name:        check.Name,
					Level:       check.Level,
					Category:    check.Category,
					Description: check.Description,
					Remediation: check.Remediation,
					Objects:     objects,
				})
			}
		}

		return nil, map[string]any{"findings": findings}, nil
	})
}
