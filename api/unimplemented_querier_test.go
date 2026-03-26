package api

// unimplementedQuerier provides default panic implementations for the Querier
// interface. Embed it in test fakes so only the methods under test need stubs.

import (
	"context"
	"time"

	"supadash/database"
)

type unimplementedQuerier struct{}

func (unimplementedQuerier) GetAccountByEmail(context.Context, string) (database.Account, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) GetAccountByGoTrueID(context.Context, string) (database.Account, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) GetAccountByID(context.Context, int32) (database.Account, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) CreateAccount(context.Context, database.CreateAccountParams) (database.Account, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) SetAccountName(context.Context, database.SetAccountNameParams) error {
	panic("unimplemented")
}
func (unimplementedQuerier) InsertRefreshToken(context.Context, database.InsertRefreshTokenParams) (database.RefreshToken, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) GetRefreshToken(context.Context, string) (database.RefreshToken, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) RevokeRefreshToken(context.Context, string) error {
	panic("unimplemented")
}
func (unimplementedQuerier) RevokeAllRefreshTokensForUser(context.Context, int32) error {
	panic("unimplemented")
}
func (unimplementedQuerier) Setup2FA(context.Context, database.Setup2FAParams) error {
	panic("unimplemented")
}
func (unimplementedQuerier) Enable2FA(context.Context, int32) error {
	panic("unimplemented")
}
func (unimplementedQuerier) Disable2FA(context.Context, int32) error {
	panic("unimplemented")
}
func (unimplementedQuerier) CreateOrganization(context.Context, string) (database.Organization, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) GetOrganizationBySlug(context.Context, string) (database.Organization, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) GetOrganizationById(context.Context, string) (database.Organization, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) GetOrganizationIdsForAccountId(context.Context, int32) ([]int32, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) GetOrganizationsForAccountId(context.Context, int32) ([]database.GetOrganizationsForAccountIdRow, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) CreateOrganizationMembership(context.Context, database.CreateOrganizationMembershipParams) (database.OrganizationMembership, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) GetOrganizationMembers(context.Context, int32) ([]database.GetOrganizationMembersRow, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) UpdateOrganizationMemberRole(context.Context, database.UpdateOrganizationMemberRoleParams) error {
	panic("unimplemented")
}
func (unimplementedQuerier) RemoveOrganizationMember(context.Context, database.RemoveOrganizationMemberParams) error {
	panic("unimplemented")
}
func (unimplementedQuerier) GetOrganizationMembershipBySlug(context.Context, database.GetOrganizationMembershipBySlugParams) (database.OrganizationMembership, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) GetOrganizationMembershipByProjectRef(context.Context, database.GetOrganizationMembershipByProjectRefParams) (database.OrganizationMembership, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) CreateProject(context.Context, database.CreateProjectParams) (database.Project, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) GetProjectByRef(context.Context, string) (database.Project, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) GetProjectsForAccountId(context.Context, int32) ([]database.Project, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) GetProjectsByStatus(context.Context, string) ([]database.Project, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) UpdateProjectStatus(context.Context, database.UpdateProjectStatusParams) (database.Project, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) UpdateProjectInfrastructure(context.Context, database.UpdateProjectInfrastructureParams) (database.Project, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) UpdateProjectJwtSecret(context.Context, database.UpdateProjectJwtSecretParams) error {
	panic("unimplemented")
}
func (unimplementedQuerier) DeleteProject(context.Context, string) error {
	panic("unimplemented")
}
func (unimplementedQuerier) GetProjectEnvVars(context.Context, string) ([]database.ProjectEnvVar, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) UpsertProjectEnvVar(context.Context, database.UpsertProjectEnvVarParams) error {
	panic("unimplemented")
}
func (unimplementedQuerier) DeleteProjectEnvVar(context.Context, string, string) error {
	panic("unimplemented")
}
func (unimplementedQuerier) DeleteProjectEnvVars(context.Context, string) error {
	panic("unimplemented")
}
func (unimplementedQuerier) GetProjectResources(context.Context, string) (database.ProjectResource, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) UpsertProjectResources(context.Context, database.UpsertProjectResourcesParams) (database.ProjectResource, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) GetAllProjectResources(context.Context) ([]database.ProjectResource, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) InsertResourceSnapshot(context.Context, database.InsertResourceSnapshotParams) error {
	panic("unimplemented")
}
func (unimplementedQuerier) GetRecentSnapshots(context.Context, string, time.Time) ([]database.ResourceSnapshot, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) DeleteOldSnapshots(context.Context, time.Time) error {
	panic("unimplemented")
}
func (unimplementedQuerier) GetHourlySnapshots(context.Context, string, time.Time) ([]database.ResourceSnapshotHourly, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) UpsertHourlySnapshot(context.Context, database.UpsertHourlySnapshotParams) error {
	panic("unimplemented")
}
func (unimplementedQuerier) GetActiveRecommendations(context.Context, string) ([]database.ResourceRecommendation, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) InsertRecommendation(context.Context, database.InsertRecommendationParams) error {
	panic("unimplemented")
}
func (unimplementedQuerier) DismissRecommendation(context.Context, int32) error {
	panic("unimplemented")
}
func (unimplementedQuerier) InsertAuditLog(context.Context, database.InsertAuditLogParams) (database.AuditLog, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) GetProjectAuditLogs(context.Context, database.GetProjectAuditLogsParams) ([]database.AuditLog, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) GetOrganizationAuditLogs(context.Context, database.GetOrganizationAuditLogsParams) ([]database.AuditLog, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) GetMigrations(context.Context) ([]database.Migration, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) GetMigration(context.Context, string) (database.Migration, error) {
	panic("unimplemented")
}
func (unimplementedQuerier) PutMigration(context.Context, database.PutMigrationParams) error {
	panic("unimplemented")
}
