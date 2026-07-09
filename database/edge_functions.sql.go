package database

import (
	"context"
)

// --- Edge Functions ---

const edgeFunctionColumns = `id, project_ref, slug, name, status, verify_jwt, entrypoint_path, version, created_at, updated_at`

func scanEdgeFunction(row interface{ Scan(...interface{}) error }, f *EdgeFunction) error {
	return row.Scan(
		&f.ID, &f.ProjectRef, &f.Slug, &f.Name, &f.Status,
		&f.VerifyJwt, &f.EntrypointPath, &f.Version, &f.CreatedAt, &f.UpdatedAt,
	)
}

const getEdgeFunctions = `
SELECT ` + edgeFunctionColumns + `
FROM edge_functions WHERE project_ref = $1 ORDER BY slug
`

func (q *Queries) GetEdgeFunctions(ctx context.Context, projectRef string) ([]EdgeFunction, error) {
	rows, err := q.db.Query(ctx, getEdgeFunctions, projectRef)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []EdgeFunction
	for rows.Next() {
		var f EdgeFunction
		if err := scanEdgeFunction(rows, &f); err != nil {
			return nil, err
		}
		items = append(items, f)
	}
	return items, rows.Err()
}

const getEdgeFunction = `
SELECT ` + edgeFunctionColumns + `
FROM edge_functions WHERE project_ref = $1 AND slug = $2
`

func (q *Queries) GetEdgeFunction(ctx context.Context, projectRef string, slug string) (EdgeFunction, error) {
	row := q.db.QueryRow(ctx, getEdgeFunction, projectRef, slug)
	var f EdgeFunction
	err := scanEdgeFunction(row, &f)
	return f, err
}

const upsertEdgeFunction = `
INSERT INTO edge_functions (project_ref, slug, name, status, verify_jwt, entrypoint_path, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, now())
ON CONFLICT (project_ref, slug) DO UPDATE
SET name = $3, status = $4, verify_jwt = $5, entrypoint_path = $6,
    version = edge_functions.version + 1, updated_at = now()
RETURNING ` + edgeFunctionColumns + `
`

type UpsertEdgeFunctionParams struct {
	ProjectRef     string
	Slug           string
	Name           string
	Status         string
	VerifyJwt      bool
	EntrypointPath string
}

func (q *Queries) UpsertEdgeFunction(ctx context.Context, arg UpsertEdgeFunctionParams) (EdgeFunction, error) {
	row := q.db.QueryRow(ctx, upsertEdgeFunction,
		arg.ProjectRef, arg.Slug, arg.Name, arg.Status, arg.VerifyJwt, arg.EntrypointPath,
	)
	var f EdgeFunction
	err := scanEdgeFunction(row, &f)
	return f, err
}

const deleteEdgeFunction = `DELETE FROM edge_functions WHERE project_ref = $1 AND slug = $2`

func (q *Queries) DeleteEdgeFunction(ctx context.Context, projectRef string, slug string) error {
	_, err := q.db.Exec(ctx, deleteEdgeFunction, projectRef, slug)
	return err
}

// --- Project Secrets (encrypted env vars for the edge runtime) ---

const getProjectSecrets = `
SELECT id, project_ref, name, value_encrypted, created_at, updated_at
FROM project_secrets WHERE project_ref = $1 ORDER BY name
`

func (q *Queries) GetProjectSecrets(ctx context.Context, projectRef string) ([]ProjectSecret, error) {
	rows, err := q.db.Query(ctx, getProjectSecrets, projectRef)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ProjectSecret
	for rows.Next() {
		var s ProjectSecret
		if err := rows.Scan(&s.ID, &s.ProjectRef, &s.Name, &s.ValueEncrypted, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, s)
	}
	return items, rows.Err()
}

const upsertProjectSecret = `
INSERT INTO project_secrets (project_ref, name, value_encrypted, updated_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (project_ref, name) DO UPDATE
SET value_encrypted = $3, updated_at = now()
`

type UpsertProjectSecretParams struct {
	ProjectRef     string
	Name           string
	ValueEncrypted string
}

func (q *Queries) UpsertProjectSecret(ctx context.Context, arg UpsertProjectSecretParams) error {
	_, err := q.db.Exec(ctx, upsertProjectSecret, arg.ProjectRef, arg.Name, arg.ValueEncrypted)
	return err
}

const deleteProjectSecret = `DELETE FROM project_secrets WHERE project_ref = $1 AND name = $2`

func (q *Queries) DeleteProjectSecret(ctx context.Context, projectRef string, name string) error {
	_, err := q.db.Exec(ctx, deleteProjectSecret, projectRef, name)
	return err
}
