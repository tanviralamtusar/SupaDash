package database

import (
	"context"
)

// --- Project Branches ---

const projectBranchColumns = `id, parent_project_ref, branch_name, db_name, status, git_branch, created_at, updated_at`

func scanProjectBranch(row interface{ Scan(...interface{}) error }, b *ProjectBranch) error {
	return row.Scan(
		&b.ID, &b.ParentProjectRef, &b.BranchName, &b.DbName,
		&b.Status, &b.GitBranch, &b.CreatedAt, &b.UpdatedAt,
	)
}

const createProjectBranch = `
INSERT INTO project_branches (parent_project_ref, branch_name, db_name, status, git_branch)
VALUES ($1, $2, $3, $4, $5)
RETURNING ` + projectBranchColumns + `
`

type CreateProjectBranchParams struct {
	ParentProjectRef string
	BranchName       string
	DbName           string
	Status           string
	GitBranch        string
}

func (q *Queries) CreateProjectBranch(ctx context.Context, arg CreateProjectBranchParams) (ProjectBranch, error) {
	row := q.db.QueryRow(ctx, createProjectBranch,
		arg.ParentProjectRef, arg.BranchName, arg.DbName, arg.Status, arg.GitBranch,
	)
	var b ProjectBranch
	err := scanProjectBranch(row, &b)
	return b, err
}

const getProjectBranches = `
SELECT ` + projectBranchColumns + `
FROM project_branches WHERE parent_project_ref = $1 ORDER BY branch_name
`

func (q *Queries) GetProjectBranches(ctx context.Context, parentProjectRef string) ([]ProjectBranch, error) {
	rows, err := q.db.Query(ctx, getProjectBranches, parentProjectRef)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ProjectBranch
	for rows.Next() {
		var b ProjectBranch
		if err := scanProjectBranch(rows, &b); err != nil {
			return nil, err
		}
		items = append(items, b)
	}
	return items, rows.Err()
}

const getProjectBranch = `
SELECT ` + projectBranchColumns + `
FROM project_branches WHERE id = $1
`

func (q *Queries) GetProjectBranch(ctx context.Context, id int32) (ProjectBranch, error) {
	row := q.db.QueryRow(ctx, getProjectBranch, id)
	var b ProjectBranch
	err := scanProjectBranch(row, &b)
	return b, err
}

const updateProjectBranchStatus = `
UPDATE project_branches SET status = $2, updated_at = now() WHERE id = $1
`

func (q *Queries) UpdateProjectBranchStatus(ctx context.Context, id int32, status string) error {
	_, err := q.db.Exec(ctx, updateProjectBranchStatus, id, status)
	return err
}

const deleteProjectBranch = `DELETE FROM project_branches WHERE id = $1`

func (q *Queries) DeleteProjectBranch(ctx context.Context, id int32) error {
	_, err := q.db.Exec(ctx, deleteProjectBranch, id)
	return err
}
