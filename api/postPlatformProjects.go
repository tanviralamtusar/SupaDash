package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
	"supadash/database"
	"supadash/provisioner"
	"supadash/utils"
)

type ProjectCreationBody struct {
	CloudProvider                  string `json:"cloud_provider"`
	OrgId                          int32  `json:"org_id"`
	Name                           string `json:"name"`
	DbPass                         string `json:"db_pass"`
	DbRegion                       string `json:"db_region"`
	CustomSupabaseInternalRequests struct {
		Ami struct {
			SearchTags struct {
				TagPostgresVersion string `json:"tag:postgresVersion"`
			} `json:"search_tags"`
		} `json:"ami"`
	} `json:"custom_supabase_internal_requests"`
	DesiredInstanceSize string `json:"desired_instance_size"`
}

type ProjectCreationResponse struct {
	Id                       int32    `json:"id"`
	Ref                      string   `json:"ref"`
	Name                     string   `json:"name"`
	Status                   string   `json:"status"`
	OrganizationId           int32    `json:"organization_id"`
	OrganizationSlug         string   `json:"organization_slug"`
	CloudProvider            string   `json:"cloud_provider"`
	Region                   string   `json:"region"`
	InsertedAt               string   `json:"inserted_at"`
	Endpoint                 string   `json:"endpoint"`
	AnonKey                  string   `json:"anon_key"`
	ServiceKey               string   `json:"service_key"`
	IsBranchEnabled          bool     `json:"is_branch_enabled"`
	PreviewBranchRefs        []string `json:"preview_branch_refs"`
	IsPhysicalBackupsEnabled bool     `json:"is_physical_backups_enabled"`
	IsReadReplicasEnabled    bool     `json:"is_read_replicas_enabled"`
	DiskVolumeSizeGb         int32    `json:"disk_volume_size_gb"`
	SubscriptionId           string   `json:"subscription_id"`
}

// createProjectCore creates a project record, persists its resource plan and
// kicks off async provisioning. The org must already be resolved by the caller
// (the HTTP handler resolves it by numeric org_id, the MCP tool by slug).
func (a *Api) createProjectCore(ctx context.Context, org database.Organization, name, dbPass, instanceSize string) (database.Project, *provisioner.ProjectSecrets, error) {
	var zero database.Project

	// Generate secrets for the new project
	secrets, err := provisioner.GenerateProjectSecrets()
	if err != nil {
		a.logger.Error(fmt.Sprintf("Failed to generate secrets: %v", err))
		return zero, nil, fmt.Errorf("failed to generate project secrets")
	}

	projectRef := utils.GenerateProjectRef(name)

	// Create project in database with generated keys
	proj, err := a.queries.CreateProject(ctx, database.CreateProjectParams{
		ProjectRef:     projectRef,
		ProjectName:    name,
		OrganizationID: int32(org.ID),
		JwtSecret:      secrets.JWTSecret,
		CloudProvider:  "DOCKER",
		Region:         "LOCAL",
	})
	if err != nil {
		a.logger.Error(fmt.Sprintf("Failed to create project in database: %v", err))
		return zero, nil, fmt.Errorf("failed to create project")
	}

	// Broadcast initial project status
	a.wsHub.BroadcastStatus(projectRef, proj.Status)

	// Persist the resource plan for the project (from desired_instance_size).
	// Plan defaults already respect the platform minimums (500 MB RAM, 1 GB storage, 500 MB DB).
	plan := provisioner.PlanForInstanceSize(instanceSize)
	quotas := provisioner.GetDefaultQuotas(plan)
	if _, resErr := a.queries.UpsertProjectResources(ctx, database.UpsertProjectResourcesParams{
		ProjectRef:             proj.ProjectRef,
		Plan:                   string(plan),
		CPULimit:               quotas.CPULimit,
		CPUReservation:         quotas.CPULimit / 2,
		MemoryLimit:            quotas.MemoryLimit,
		MemoryReservation:      quotas.MemoryLimit / 2,
		BurstEligible:          true,
		BurstPriority:          0,
		DatabaseSizeLimitBytes: quotas.DatabaseSize,
		StorageSizeLimitBytes:  quotas.StorageSize,
	}); resErr != nil {
		a.logger.Warn(fmt.Sprintf("Failed to persist resource plan for %s: %v", proj.ProjectRef, resErr))
	}

	// Update project with generated keys immediately
	if _, updateErr := a.queries.UpdateProjectInfrastructure(ctx, database.UpdateProjectInfrastructureParams{
		ProjectRef:     proj.ProjectRef,
		AnonKey:        pgtype.Text{String: secrets.AnonKey, Valid: true},
		ServiceRoleKey: pgtype.Text{String: secrets.ServiceKey, Valid: true},
	}); updateErr != nil {
		a.logger.Warn(fmt.Sprintf("Failed to save keys to DB: %v", updateErr))
	}

	// Trigger async provisioning if provisioner is available
	if a.provisioner != nil {
		go func() {
			ctx := context.Background()
			a.logger.Info(fmt.Sprintf("Starting async provisioning for project %s (%s)", proj.ProjectRef, proj.ProjectName))

			// The Postgres password is interpolated into the compose YAML and
			// into non-URL-encoded connection strings, so it must be
			// alphanumeric. Fall back to the generated (hex, safe) password if
			// the supplied one contains characters that would break either.
			dbPassword := secrets.DBPassword
			if dbPass != "" {
				if provisioner.IsInfraSafePassword(dbPass) {
					dbPassword = dbPass
				} else {
					a.logger.Warn(fmt.Sprintf("Supplied db password for %s contains unsupported characters; using a generated password instead", proj.ProjectRef))
				}
			}

			config := &provisioner.ProjectConfig{
				ProjectID:      proj.ProjectRef,
				ProjectName:    proj.ProjectName,
				OrganizationID: fmt.Sprintf("%d", proj.OrganizationID),
				Region:         proj.Region,
				DBPassword:     dbPassword,
				JWTSecret:      secrets.JWTSecret,
				AnonKey:        secrets.AnonKey,
				ServiceKey:     secrets.ServiceKey,
				DashboardUser:  secrets.DashboardUser,
				DashboardPass:  secrets.DashboardPass,
			}

			info, provErr := a.provisioner.CreateProject(ctx, config)
			if provErr != nil {
				a.logger.Error(fmt.Sprintf("Provisioning failed for project %s: %v", proj.ProjectRef, provErr))
				// Update status to FAILED
				if _, statusErr := a.queries.UpdateProjectStatus(ctx, database.UpdateProjectStatusParams{
					ProjectRef: proj.ProjectRef,
					Status:     "FAILED",
				}); statusErr != nil {
					a.logger.Error(fmt.Sprintf("Failed to update status to FAILED: %v", statusErr))
				}
				a.wsHub.BroadcastStatus(proj.ProjectRef, "FAILED")
				return
			}

			// Update project with infrastructure info
			if _, infraErr := a.queries.UpdateProjectInfrastructure(ctx, database.UpdateProjectInfrastructureParams{
				ProjectRef:        proj.ProjectRef,
				DockerComposePath: pgtype.Text{String: info.Endpoint, Valid: true},
				DockerNetworkName: pgtype.Text{String: fmt.Sprintf("supabase-%s", proj.ProjectRef), Valid: true},
				AnonKey:           pgtype.Text{String: secrets.AnonKey, Valid: true},
				ServiceRoleKey:    pgtype.Text{String: secrets.ServiceKey, Valid: true},
			}); infraErr != nil {
				a.logger.Error(fmt.Sprintf("Failed to update infrastructure for %s: %v", proj.ProjectRef, infraErr))
			}

			// Update status to ACTIVE
			if _, statusErr := a.queries.UpdateProjectStatus(ctx, database.UpdateProjectStatusParams{
				ProjectRef: proj.ProjectRef,
				Status:     "ACTIVE_HEALTHY",
			}); statusErr != nil {
				a.logger.Error(fmt.Sprintf("Failed to update status to ACTIVE: %v", statusErr))
			}
			a.wsHub.BroadcastStatus(proj.ProjectRef, "ACTIVE_HEALTHY")

			// Apply the plan's CPU/memory limits to the freshly provisioned containers.
			// A zero limit means "unlimited" — nothing to apply.
			if a.resourceManager != nil && quotas.CPULimit > 0 && quotas.MemoryLimit > 0 {
				if limitErr := a.resourceManager.SetProjectResources(ctx, proj.ProjectRef, quotas.CPULimit, quotas.MemoryLimit); limitErr != nil {
					a.logger.Warn(fmt.Sprintf("Failed to apply resource limits for %s: %v", proj.ProjectRef, limitErr))
				}
			}

			a.logger.Info(fmt.Sprintf("Provisioning completed for project %s — API: %s", proj.ProjectRef, info.Endpoint))
		}()
	}

	return proj, secrets, nil
}

func (a *Api) postPlatformProjects(c *gin.Context) {
	account, err := a.GetAccountFromRequest(c)
	if err != nil {
		errJSON(c, 401, "Unauthorized")
		return
	}

	var createProject ProjectCreationBody
	if err := c.BindJSON(&createProject); err != nil {
		errJSON(c, 400, "Bad Request")
		return
	}

	if createProject.OrgId == 0 {
		errJSON(c, 400, "org_id is required")
		return
	}
	if createProject.Name == "" {
		errJSON(c, 400, "Project name is required")
		return
	}

	// Resolve the org by its numeric id (Studio sends org_id) and confirm the
	// caller is a member of it.
	org, err := a.queries.GetOrganizationByNumericId(c.Request.Context(), createProject.OrgId)
	if err != nil {
		errJSON(c, 400, "Invalid organization")
		return
	}
	if _, err := a.queries.GetOrganizationMembershipBySlug(c.Request.Context(), database.GetOrganizationMembershipBySlugParams{
		Slug:      org.Slug,
		AccountID: account.ID,
	}); err != nil {
		errJSON(c, 403, "You are not a member of this organization")
		return
	}

	proj, secrets, err := a.createProjectCore(c.Request.Context(),
		org, createProject.Name, createProject.DbPass, createProject.DesiredInstanceSize)
	if err != nil {
		errJSON(c, 400, err.Error())
		return
	}

	c.JSON(http.StatusCreated, ProjectCreationResponse{
		Id:                       proj.ID,
		Ref:                      proj.ProjectRef,
		Name:                     proj.ProjectName,
		Status:                   proj.Status,
		OrganizationId:           proj.OrganizationID,
		OrganizationSlug:         org.Slug,
		CloudProvider:            "DOCKER",
		Region:                   "LOCAL",
		InsertedAt:               proj.CreatedAt.Time.Format("2006-01-02T15:04:05.999Z"),
		Endpoint:                 fmt.Sprintf("https://%s.%s", proj.ProjectRef, a.config.Domain.Base),
		AnonKey:                  secrets.AnonKey,
		ServiceKey:               secrets.ServiceKey,
		IsBranchEnabled:          true,
		PreviewBranchRefs:        []string{},
		IsPhysicalBackupsEnabled: false,
		IsReadReplicasEnabled:    false,
		DiskVolumeSizeGb:         0,
		SubscriptionId:           "self-hosted",
	})
}
