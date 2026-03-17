package api

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
)

func (a *Api) getPlatformProjects(c *gin.Context) {
	account, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(401, gin.H{"error": "Unauthorized"})
		return
	}

	projects, err := a.queries.GetProjectsForAccountId(c, account.ID)
	if err != nil {
		c.JSON(500, gin.H{"error": "Internal Server Error"})
		return
	}

	supaProjects := []Project{}
	for _, project := range projects {
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
		})

		_ = apiEndpoint // Used for future service URL response extensions
	}

	c.JSON(http.StatusOK, supaProjects)
}
