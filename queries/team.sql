-- name: GetOrganizationMembers :many
SELECT a.id, a.gotrue_id, a.email, a.username, a.first_name, a.last_name, om.role, om.created_at
FROM organization_membership om
JOIN accounts a ON om.account_id = a.id
WHERE om.organization_id = $1;

-- name: UpdateOrganizationMemberRole :exec
UPDATE organization_membership
SET role = $3, updated_at = now()
WHERE organization_id = $1 AND account_id = $2;

-- name: RemoveOrganizationMember :exec
DELETE FROM organization_membership
WHERE organization_id = $1 AND account_id = $2;

-- name: GetOrganizationMembershipBySlug :one
SELECT om.*
FROM organization_membership om
JOIN organizations o ON om.organization_id = o.id
WHERE o.slug = $1 AND om.account_id = $2 LIMIT 1;

-- name: GetOrganizationMembershipByProjectRef :one
SELECT om.*
FROM organization_membership om
JOIN project p ON om.organization_id = p.organization_id
WHERE p.project_ref = $1 AND om.account_id = $2 LIMIT 1;
