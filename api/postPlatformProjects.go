package api

import (
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
	"net/http"
	"supadash/database"
	"supadash/provisioner"
	"supadash/utils"
)

type ProjectCreationBody struct {
	CloudProvider                  string `json:"cloud_provider"`
	OrganizationSlug               string `json:"organization_slug"`
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

func (a *Api) postPlatformProjects(c *gin.Context) {
	_, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(401, gin.H{"error": "Unauthorized"})
		return
	}

	var createProject ProjectCreationBody
	if err := c.BindJSON(&createProject); err != nil {
		c.JSON(400, gin.H{"error": "Bad Request"})
		return
	}

	org, err := a.queries.GetOrganizationBySlug(c, createProject.OrganizationSlug)
	if err != nil {
		a.logger.Error(fmt.Sprintf("Failed to find organization by slug %s: %v", createProject.OrganizationSlug, err))
		c.JSON(400, gin.H{"error": "Invalid organization slug"})
		return
	}

	// Generate secrets for the new project
	secrets, err := provisioner.GenerateProjectSecrets()
	if err != nil {
		a.logger.Error(fmt.Sprintf("Failed to generate secrets: %v", err))
		c.JSON(500, gin.H{"error": "Failed to generate project secrets"})
		return
	}

	projectRef := utils.GenerateProjectRef(createProject.Name)

	// Create project in database with generated keys
	proj, err := a.queries.CreateProject(c.Request.Context(), database.CreateProjectParams{
		ProjectRef:     projectRef,
		ProjectName:    createProject.Name,
		OrganizationID: int32(org.ID),
		JwtSecret:      secrets.JWTSecret,
		CloudProvider:  "DOCKER",
		Region:         "LOCAL",
	})

	if err != nil {
		a.logger.Error(fmt.Sprintf("Failed to create project in database: %v", err))
		c.JSON(500, gin.H{"error": "Internal Server Error"})
		return
	}

	// Update project with generated keys immediately
	if _, updateErr := a.queries.UpdateProjectInfrastructure(c.Request.Context(), database.UpdateProjectInfrastructureParams{
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

			dbPassword := secrets.DBPassword
			if createProject.DbPass != "" {
				dbPassword = createProject.DbPass
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

			a.logger.Info(fmt.Sprintf("Provisioning completed for project %s — API: %s", proj.ProjectRef, info.Endpoint))
		}()
	}

	c.JSON(http.StatusCreated, ProjectCreationResponse{
		Id:                       proj.ID,
		Ref:                      proj.ProjectRef,
		Name:                     proj.ProjectName,
		Status:                   proj.Status,
		OrganizationId:           proj.OrganizationID,
		OrganizationSlug:         createProject.OrganizationSlug,
		CloudProvider:            "DOCKER",
		Region:                   "LOCAL",
		InsertedAt:               proj.CreatedAt.Time.Format("2006-01-02T15:04:05.999Z"),
		Endpoint:                 fmt.Sprintf("https://%s.%s", proj.ProjectRef, a.config.Domain.Base),
		AnonKey:                  secrets.AnonKey,
		ServiceKey:               secrets.ServiceKey,
		IsBranchEnabled:          false,
		PreviewBranchRefs:        []string{},
		IsPhysicalBackupsEnabled: false,
		IsReadReplicasEnabled:    false,
		DiskVolumeSizeGb:         0,
		SubscriptionId:           "self-hosted",
	})
}
