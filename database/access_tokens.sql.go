package database

import (
	"context"
)

// --- Personal Access Tokens ---

const patColumns = `id, account_id, name, token_hash, created_at, last_used_at`

const createAccessToken = `
INSERT INTO personal_access_tokens (account_id, name, token_hash)
VALUES ($1, $2, $3)
RETURNING ` + patColumns + `
`

type CreateAccessTokenParams struct {
	AccountID int32
	Name      string
	TokenHash string
}

func (q *Queries) CreateAccessToken(ctx context.Context, arg CreateAccessTokenParams) (PersonalAccessToken, error) {
	row := q.db.QueryRow(ctx, createAccessToken, arg.AccountID, arg.Name, arg.TokenHash)
	var t PersonalAccessToken
	err := row.Scan(&t.ID, &t.AccountID, &t.Name, &t.TokenHash, &t.CreatedAt, &t.LastUsedAt)
	return t, err
}

const getAccessTokenByHash = `
SELECT ` + patColumns + `
FROM personal_access_tokens WHERE token_hash = $1
`

func (q *Queries) GetAccessTokenByHash(ctx context.Context, tokenHash string) (PersonalAccessToken, error) {
	row := q.db.QueryRow(ctx, getAccessTokenByHash, tokenHash)
	var t PersonalAccessToken
	err := row.Scan(&t.ID, &t.AccountID, &t.Name, &t.TokenHash, &t.CreatedAt, &t.LastUsedAt)
	return t, err
}

const getAccessTokensForAccount = `
SELECT ` + patColumns + `
FROM personal_access_tokens WHERE account_id = $1 ORDER BY created_at DESC
`

func (q *Queries) GetAccessTokensForAccount(ctx context.Context, accountID int32) ([]PersonalAccessToken, error) {
	rows, err := q.db.Query(ctx, getAccessTokensForAccount, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []PersonalAccessToken
	for rows.Next() {
		var t PersonalAccessToken
		if err := rows.Scan(&t.ID, &t.AccountID, &t.Name, &t.TokenHash, &t.CreatedAt, &t.LastUsedAt); err != nil {
			return nil, err
		}
		items = append(items, t)
	}
	return items, rows.Err()
}

const touchAccessToken = `
UPDATE personal_access_tokens SET last_used_at = now() WHERE id = $1
`

func (q *Queries) TouchAccessToken(ctx context.Context, id int32) error {
	_, err := q.db.Exec(ctx, touchAccessToken, id)
	return err
}

const deleteAccessToken = `
DELETE FROM personal_access_tokens WHERE id = $1 AND account_id = $2
`

func (q *Queries) DeleteAccessToken(ctx context.Context, id int32, accountID int32) error {
	_, err := q.db.Exec(ctx, deleteAccessToken, id, accountID)
	return err
}
