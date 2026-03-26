package api

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

type OrganizationProjectsResponse struct {
	Projects   []Project  `json:"projects"`
	Pagination Pagination `json:"pagination"`
}

type Pagination struct {
	Count int32 `json:"count"`
}

func (a *Api) getPlatformOrganizationProjects(c *gin.Context) {
	account, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	slug := c.Param("slug")
	org, err := a.queries.GetOrganizationBySlug(c, slug)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Organization not found"})
		return
	}

	allProjects, err := a.queries.GetProjectsForAccountId(c, account.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
		return
	}

	supaProjects := []Project{}
	for _, project := range allProjects {
		if project.OrganizationID != org.ID {
			continue
		}

		dbPort := int32(0)
		if project.PostgresPort.Valid {
			dbPort = project.PostgresPort.Int32
		}

		insertedAt := ""
		if project.CreatedAt.Valid {
			insertedAt = project.CreatedAt.Time.Format("2006-01-02T15:04:05.999Z")
		}

		dbHost := "localhost"
		apiEndpoint := fmt.Sprintf("http://localhost:%d", project.KongHttpPort.Int32)

		supaProjects = append(supaProjects, Project{
			Id:                       project.ID,
			Ref:                      project.ProjectRef,
			Name:                     project.ProjectName,
			Status:                   project.Status,
			OrganizationId:           project.OrganizationID,
			InsertedAt:               insertedAt,
			SubscriptionId:           "self-hosted",
			CloudProvider:            project.CloudProvider,
			Region:                   project.Region,
			DiskVolumeSizeGb:         0,
			Size:                     "",
			DbUserSupabase:           "postgres",
			DbPassSupabase:           "",
			DbDnsName:                dbHost,
			DbHost:                   dbHost,
			DbPort:                   dbPort,
			DbName:                   "postgres",
			SslEnforced:              false,
			WalgEnabled:              false,
			InfraComputeSize:         "self-hosted",
			PreviewBranchRefs:        []interface{}{},
			IsBranchEnabled:          false,
			IsPhysicalBackupsEnabled: false,
			JwtSecret:                "",
			Databases: []Database{
				{
					Identifier:       project.ProjectRef,
					InfraComputeSize: "self-hosted",
				},
			},
		})

		_ = apiEndpoint // Used for future service URL response extensions
	}

	if supaProjects == nil {
		supaProjects = []Project{}
	}

	response := OrganizationProjectsResponse{
		Projects: supaProjects,
		Pagination: Pagination{
			Count: int32(len(supaProjects)),
		},
	}

	c.JSON(http.StatusOK, response)
}
