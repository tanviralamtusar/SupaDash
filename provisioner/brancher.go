package provisioner

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// Brancher defines database-branching operations for a project.
//
// The current implementation (DBBrancher) is DB-only: a branch is a separate
// database inside the parent project's Postgres container. A future
// implementation may provision a full container stack per branch — API
// callers must not assume either strategy.
type Brancher interface {
	// CreateBranch clones the parent's main database into a new branch database.
	CreateBranch(ctx context.Context, projectID, dbName string) error

	// DeleteBranch drops the branch database.
	DeleteBranch(ctx context.Context, projectID, dbName string) error

	// MergeBranch replays migrations present on the branch but not on the
	// parent onto the parent database.
	MergeBranch(ctx context.Context, projectID, dbName string) error

	// ResetBranch discards the branch database and re-clones it from the parent.
	ResetBranch(ctx context.Context, projectID, dbName string) error

	// RebaseBranch re-clones the branch from the current parent state and
	// re-applies the branch's own (unmerged) migrations on top.
	RebaseBranch(ctx context.Context, projectID, dbName string) error
}

var branchNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,62}$`)

// ValidateBranchName rejects branch names unsuitable for deriving a database name.
func ValidateBranchName(name string) error {
	if !branchNamePattern.MatchString(name) {
		return fmt.Errorf("invalid branch name %q: use letters, digits, '-' and '_' (max 63 chars)", name)
	}
	if strings.EqualFold(name, "main") || strings.EqualFold(name, "postgres") {
		return fmt.Errorf("branch name %q is reserved", name)
	}
	return nil
}

// BranchDBName derives the Postgres database name for a branch.
func BranchDBName(branchName string) string {
	sanitized := strings.ToLower(branchName)
	sanitized = strings.ReplaceAll(sanitized, "-", "_")
	return "br_" + sanitized
}

// dbNamePattern is the shape every derived branch DB name must have. Guards
// against SQL injection wherever the name is interpolated into DDL.
var dbNamePattern = regexp.MustCompile(`^br_[a-z0-9_]{1,60}$`)

func validateBranchDBName(dbName string) error {
	if !dbNamePattern.MatchString(dbName) {
		return fmt.Errorf("invalid branch database name %q", dbName)
	}
	return nil
}

// DBBrancher implements Brancher by managing databases inside the parent
// project's Postgres container via the Docker provisioner.
type DBBrancher struct {
	provisioner *DockerProvisioner
}

// NewDBBrancher creates a DB-only brancher backed by the Docker provisioner.
func NewDBBrancher(p *DockerProvisioner) *DBBrancher {
	return &DBBrancher{provisioner: p}
}

// psql runs a SQL command against a database in the project's db container.
func (b *DBBrancher) psql(ctx context.Context, projectID, dbName, sql string) (string, error) {
	return b.provisioner.ExecuteCommand(ctx, projectID, "db",
		[]string{"psql", "-U", "postgres", "-d", dbName, "-v", "ON_ERROR_STOP=1", "-tAc", sql})
}

// ensureMigrationsTable creates the migration-tracking schema Supabase
// tooling uses, so merge/rebase can diff migration histories.
func (b *DBBrancher) ensureMigrationsTable(ctx context.Context, projectID, dbName string) error {
	_, err := b.psql(ctx, projectID, dbName,
		`CREATE SCHEMA IF NOT EXISTS supabase_migrations;
		 CREATE TABLE IF NOT EXISTS supabase_migrations.schema_migrations (
		   version text primary key, name text, statements text[]
		 )`)
	return err
}

// cloneDatabase clones sourceDB into destDB (which must not exist) using
// pg_dump piped into psql — CREATE DATABASE ... TEMPLATE cannot be used
// while services hold connections to the source.
func (b *DBBrancher) cloneDatabase(ctx context.Context, projectID, sourceDB, destDB string) error {
	if _, err := b.psql(ctx, projectID, "postgres",
		fmt.Sprintf("CREATE DATABASE %s", destDB)); err != nil {
		return fmt.Errorf("failed to create branch database: %w", err)
	}

	// Pipe dump into the new database inside the container.
	// ON_ERROR_STOP is intentionally off: dumps of managed schemas emit
	// benign ownership/duplicate notices on restore.
	dumpCmd := fmt.Sprintf("pg_dump -U postgres -d %s | psql -U postgres -d %s", sourceDB, destDB)
	if _, err := b.provisioner.ExecuteCommand(ctx, projectID, "db", []string{"sh", "-c", dumpCmd}); err != nil {
		// Best-effort cleanup of the partially restored database
		b.psql(ctx, projectID, "postgres", fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", destDB))
		return fmt.Errorf("failed to clone database: %w", err)
	}

	return nil
}

// migrationVersions returns the ordered migration versions recorded in a database.
func (b *DBBrancher) migrationVersions(ctx context.Context, projectID, dbName string) ([]string, error) {
	if err := b.ensureMigrationsTable(ctx, projectID, dbName); err != nil {
		return nil, err
	}
	out, err := b.psql(ctx, projectID, dbName,
		"SELECT version FROM supabase_migrations.schema_migrations ORDER BY version")
	if err != nil {
		return nil, err
	}
	var versions []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if v := strings.TrimSpace(line); v != "" {
			versions = append(versions, v)
		}
	}
	return versions, nil
}

// branchOnlyMigration holds one migration that exists on the branch but not the parent.
type branchOnlyMigration struct {
	Version string
	Name    string
	SQL     string
}

// branchOnlyMigrations diffs branch vs parent migration histories and loads
// the SQL of migrations unique to the branch.
func (b *DBBrancher) branchOnlyMigrations(ctx context.Context, projectID, dbName string) ([]branchOnlyMigration, error) {
	branchVersions, err := b.migrationVersions(ctx, projectID, dbName)
	if err != nil {
		return nil, err
	}
	parentVersions, err := b.migrationVersions(ctx, projectID, "postgres")
	if err != nil {
		return nil, err
	}

	inParent := make(map[string]bool, len(parentVersions))
	for _, v := range parentVersions {
		inParent[v] = true
	}

	var missing []branchOnlyMigration
	for _, v := range branchVersions {
		if inParent[v] {
			continue
		}
		name, err := b.psql(ctx, projectID, dbName, fmt.Sprintf(
			"SELECT coalesce(name, '') FROM supabase_migrations.schema_migrations WHERE version = %s", quoteLiteral(v)))
		if err != nil {
			return nil, err
		}
		sql, err := b.psql(ctx, projectID, dbName, fmt.Sprintf(
			"SELECT array_to_string(statements, E';\\n') FROM supabase_migrations.schema_migrations WHERE version = %s", quoteLiteral(v)))
		if err != nil {
			return nil, err
		}
		missing = append(missing, branchOnlyMigration{
			Version: v,
			Name:    strings.TrimSpace(name),
			SQL:     strings.TrimSpace(sql),
		})
	}
	return missing, nil
}

// applyMigration executes a migration's SQL against a database and records it
// in that database's migration history.
func (b *DBBrancher) applyMigration(ctx context.Context, projectID, dbName string, m branchOnlyMigration) error {
	if m.SQL != "" {
		if _, err := b.psql(ctx, projectID, dbName, m.SQL); err != nil {
			return fmt.Errorf("migration %s failed: %w", m.Version, err)
		}
	}
	record := fmt.Sprintf(
		"INSERT INTO supabase_migrations.schema_migrations (version, name, statements) VALUES (%s, %s, ARRAY[%s]) ON CONFLICT (version) DO NOTHING",
		quoteLiteral(m.Version), quoteLiteral(m.Name), quoteLiteral(m.SQL))
	if _, err := b.psql(ctx, projectID, dbName, record); err != nil {
		return fmt.Errorf("failed to record migration %s: %w", m.Version, err)
	}
	return nil
}

// quoteLiteral makes a safe Postgres string literal (standard '' escaping).
func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// ApplyMigration executes migration SQL against a project database and
// records it in the database's migration history (used by merge/rebase
// diffing). dbName is "postgres" for the main database or a branch DB name.
func (b *DBBrancher) ApplyMigration(ctx context.Context, projectID, dbName, version, name, sql string) error {
	if dbName != "postgres" {
		if err := validateBranchDBName(dbName); err != nil {
			return err
		}
	}
	if err := b.ensureMigrationsTable(ctx, projectID, dbName); err != nil {
		return err
	}
	return b.applyMigration(ctx, projectID, dbName, branchOnlyMigration{
		Version: version,
		Name:    name,
		SQL:     sql,
	})
}

// --- Brancher implementation ---

func (b *DBBrancher) CreateBranch(ctx context.Context, projectID, dbName string) error {
	if err := validateBranchDBName(dbName); err != nil {
		return err
	}
	if err := b.ensureMigrationsTable(ctx, projectID, "postgres"); err != nil {
		return err
	}
	if err := b.cloneDatabase(ctx, projectID, "postgres", dbName); err != nil {
		return err
	}
	return b.ensureMigrationsTable(ctx, projectID, dbName)
}

func (b *DBBrancher) DeleteBranch(ctx context.Context, projectID, dbName string) error {
	if err := validateBranchDBName(dbName); err != nil {
		return err
	}
	_, err := b.psql(ctx, projectID, "postgres",
		fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", dbName))
	return err
}

func (b *DBBrancher) MergeBranch(ctx context.Context, projectID, dbName string) error {
	if err := validateBranchDBName(dbName); err != nil {
		return err
	}
	missing, err := b.branchOnlyMigrations(ctx, projectID, dbName)
	if err != nil {
		return err
	}
	for _, m := range missing {
		if err := b.applyMigration(ctx, projectID, "postgres", m); err != nil {
			return err
		}
	}
	return nil
}

func (b *DBBrancher) ResetBranch(ctx context.Context, projectID, dbName string) error {
	if err := b.DeleteBranch(ctx, projectID, dbName); err != nil {
		return err
	}
	return b.CreateBranch(ctx, projectID, dbName)
}

func (b *DBBrancher) RebaseBranch(ctx context.Context, projectID, dbName string) error {
	if err := validateBranchDBName(dbName); err != nil {
		return err
	}

	// Preserve the branch's own migrations before discarding it
	missing, err := b.branchOnlyMigrations(ctx, projectID, dbName)
	if err != nil {
		return err
	}

	if err := b.ResetBranch(ctx, projectID, dbName); err != nil {
		return err
	}

	// Re-apply the branch's migrations on top of the fresh parent clone
	for _, m := range missing {
		if err := b.applyMigration(ctx, projectID, dbName, m); err != nil {
			return err
		}
	}
	return nil
}
