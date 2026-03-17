package database

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

type ProjectEnvVar struct {
	ID         int32
	ProjectRef string
	Key        string
	Value      string
	IsSecret   bool
	CreatedAt  pgtype.Timestamptz
	UpdatedAt  pgtype.Timestamptz
}

const getProjectEnvVars = `
SELECT id, project_ref, key, value, is_secret, created_at, updated_at
FROM project_env_vars
WHERE project_ref = $1
ORDER BY key
`

func (q *Queries) GetProjectEnvVars(ctx context.Context, projectRef string) ([]ProjectEnvVar, error) {
	rows, err := q.db.Query(ctx, getProjectEnvVars, projectRef)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ProjectEnvVar
	for rows.Next() {
		var i ProjectEnvVar
		if err := rows.Scan(
			&i.ID, &i.ProjectRef, &i.Key, &i.Value, &i.IsSecret, &i.CreatedAt, &i.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

const upsertProjectEnvVar = `
INSERT INTO project_env_vars (project_ref, key, value, is_secret, updated_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (project_ref, key) DO UPDATE
SET value = $3, is_secret = $4, updated_at = now()
`

type UpsertProjectEnvVarParams struct {
	ProjectRef string
	Key        string
	Value      string
	IsSecret   bool
}

func (q *Queries) UpsertProjectEnvVar(ctx context.Context, arg UpsertProjectEnvVarParams) error {
	_, err := q.db.Exec(ctx, upsertProjectEnvVar, arg.ProjectRef, arg.Key, arg.Value, arg.IsSecret)
	return err
}

const deleteProjectEnvVar = `
DELETE FROM project_env_vars
WHERE project_ref = $1 AND key = $2
`

func (q *Queries) DeleteProjectEnvVar(ctx context.Context, projectRef string, key string) error {
	_, err := q.db.Exec(ctx, deleteProjectEnvVar, projectRef, key)
	return err
}

const deleteProjectEnvVars = `
DELETE FROM project_env_vars
WHERE project_ref = $1
`

func (q *Queries) DeleteProjectEnvVars(ctx context.Context, projectRef string) error {
	_, err := q.db.Exec(ctx, deleteProjectEnvVars, projectRef)
	return err
}
