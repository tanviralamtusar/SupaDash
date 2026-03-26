package api

import (
	"context"
	"time"

	"supadash/database"
)

// Querier is an interface wrapping all database.Queries methods used by
// this package. It enables injecting a fake in unit tests without a real DB.
// *database.Queries satisfies this interface automatically.
type Querier interface {
	// Accounts
	GetAccountByEmail(ctx context.Context, email string) (database.Account, error)
	GetAccountByGoTrueID(ctx context.Context, gotrueID string) (database.Account, error)
	GetAccountByID(ctx context.Context, id int32) (database.Account, error)
	CreateAccount(ctx context.Context, arg database.CreateAccountParams) (database.Account, error)
	SetAccountName(ctx context.Context, arg database.SetAccountNameParams) error

	// Auth / refresh tokens
	InsertRefreshToken(ctx context.Context, arg database.InsertRefreshTokenParams) (database.RefreshToken, error)
	GetRefreshToken(ctx context.Context, token string) (database.RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, token string) error
	RevokeAllRefreshTokensForUser(ctx context.Context, accountID int32) error

	// 2FA
	Setup2FA(ctx context.Context, arg database.Setup2FAParams) error
	Enable2FA(ctx context.Context, id int32) error
	Disable2FA(ctx context.Context, id int32) error

	// Organizations
	CreateOrganization(ctx context.Context, name string) (database.Organization, error)
	GetOrganizationBySlug(ctx context.Context, slug string) (database.Organization, error)
	GetOrganizationById(ctx context.Context, id string) (database.Organization, error)
	GetOrganizationIdsForAccountId(ctx context.Context, accountID int32) ([]int32, error)
	GetOrganizationsForAccountId(ctx context.Context, accountID int32) ([]database.GetOrganizationsForAccountIdRow, error)
	CreateOrganizationMembership(ctx context.Context, arg database.CreateOrganizationMembershipParams) (database.OrganizationMembership, error)
	GetOrganizationMembers(ctx context.Context, organizationID int32) ([]database.GetOrganizationMembersRow, error)
	UpdateOrganizationMemberRole(ctx context.Context, arg database.UpdateOrganizationMemberRoleParams) error
	RemoveOrganizationMember(ctx context.Context, arg database.RemoveOrganizationMemberParams) error
	GetOrganizationMembershipBySlug(ctx context.Context, arg database.GetOrganizationMembershipBySlugParams) (database.OrganizationMembership, error)
	GetOrganizationMembershipByProjectRef(ctx context.Context, arg database.GetOrganizationMembershipByProjectRefParams) (database.OrganizationMembership, error)

	// Projects
	CreateProject(ctx context.Context, arg database.CreateProjectParams) (database.Project, error)
	GetProjectByRef(ctx context.Context, projectRef string) (database.Project, error)
	GetProjectsForAccountId(ctx context.Context, accountID int32) ([]database.Project, error)
	GetProjectsByStatus(ctx context.Context, status string) ([]database.Project, error)
	UpdateProjectStatus(ctx context.Context, arg database.UpdateProjectStatusParams) (database.Project, error)
	UpdateProjectInfrastructure(ctx context.Context, arg database.UpdateProjectInfrastructureParams) (database.Project, error)
	UpdateProjectJwtSecret(ctx context.Context, arg database.UpdateProjectJwtSecretParams) error
	DeleteProject(ctx context.Context, projectRef string) error

	// Project env vars
	GetProjectEnvVars(ctx context.Context, projectRef string) ([]database.ProjectEnvVar, error)
	UpsertProjectEnvVar(ctx context.Context, arg database.UpsertProjectEnvVarParams) error
	DeleteProjectEnvVar(ctx context.Context, projectRef string, key string) error
	DeleteProjectEnvVars(ctx context.Context, projectRef string) error

	// Resource management
	GetProjectResources(ctx context.Context, projectRef string) (database.ProjectResource, error)
	UpsertProjectResources(ctx context.Context, arg database.UpsertProjectResourcesParams) (database.ProjectResource, error)
	GetAllProjectResources(ctx context.Context) ([]database.ProjectResource, error)
	InsertResourceSnapshot(ctx context.Context, arg database.InsertResourceSnapshotParams) error
	GetRecentSnapshots(ctx context.Context, projectRef string, since time.Time) ([]database.ResourceSnapshot, error)
	DeleteOldSnapshots(ctx context.Context, before time.Time) error
	GetHourlySnapshots(ctx context.Context, projectRef string, since time.Time) ([]database.ResourceSnapshotHourly, error)
	UpsertHourlySnapshot(ctx context.Context, arg database.UpsertHourlySnapshotParams) error
	GetActiveRecommendations(ctx context.Context, projectRef string) ([]database.ResourceRecommendation, error)
	InsertRecommendation(ctx context.Context, arg database.InsertRecommendationParams) error
	DismissRecommendation(ctx context.Context, id int32) error

	// Audit logs
	InsertAuditLog(ctx context.Context, arg database.InsertAuditLogParams) (database.AuditLog, error)
	GetProjectAuditLogs(ctx context.Context, arg database.GetProjectAuditLogsParams) ([]database.AuditLog, error)
	GetOrganizationAuditLogs(ctx context.Context, arg database.GetOrganizationAuditLogsParams) ([]database.AuditLog, error)

	// Migrations
	GetMigrations(ctx context.Context) ([]database.Migration, error)
	GetMigration(ctx context.Context, id string) (database.Migration, error)
	PutMigration(ctx context.Context, arg database.PutMigrationParams) error
}

// Compile-time assertion: *database.Queries must satisfy Querier.
var _ Querier = (*database.Queries)(nil)
