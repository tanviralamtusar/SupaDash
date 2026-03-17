package api

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
)

func (a *Api) getPlatformProjectSettings(c *gin.Context) {
	_, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(401, gin.H{"error": "Unauthorized"})
		return
	}

	projectRef := c.Param("ref")
	proj, err := a.queries.GetProjectByRef(c, projectRef)
	if err != nil {
		c.JSON(500, gin.H{"error": "Internal Server Error"})
		return
	}

	domain := a.config.Domain.Base
	apiEndpoint := fmt.Sprintf("%s.%s", proj.ProjectRef, domain)

	// Use real keys from DB, fallback to placeholder
	anonKey := "not-yet-provisioned"
	serviceKey := "not-yet-provisioned"
	if proj.AnonKey.Valid && proj.AnonKey.String != "" {
		anonKey = proj.AnonKey.String
	}
	if proj.ServiceRoleKey.Valid && proj.ServiceRoleKey.String != "" {
		serviceKey = proj.ServiceRoleKey.String
	}

	// Build real service URLs based on allocated ports
	dbPort := int32(5432)
	apiPort := int32(8000)
	if proj.PostgresPort.Valid {
		dbPort = proj.PostgresPort.Int32
	}
	if proj.KongHttpPort.Valid {
		apiPort = proj.KongHttpPort.Int32
	}

	insertedAt := ""
	if proj.CreatedAt.Valid {
		insertedAt = proj.CreatedAt.Time.Format("2006-01-02T15:04:05.999Z")
	}

	c.JSON(http.StatusOK, gin.H{
		"project": Project{
			Id:                       proj.ID,
			Ref:                      proj.ProjectRef,
			Name:                     proj.ProjectName,
			Status:                   proj.Status,
			OrganizationId:           proj.OrganizationID,
			InsertedAt:               insertedAt,
			SubscriptionId:           "self-hosted",
			CloudProvider:            proj.CloudProvider,
			Region:                   proj.Region,
			DiskVolumeSizeGb:         0,
			Size:                     "",
			DbUserSupabase:           "postgres",
			DbPassSupabase:           "",
			DbDnsName:                "localhost",
			DbHost:                   "localhost",
			DbPort:                   dbPort,
			DbName:                   "postgres",
			SslEnforced:              false,
			WalgEnabled:              false,
			InfraComputeSize:         "self-hosted",
			PreviewBranchRefs:        []interface{}{},
			IsBranchEnabled:          false,
			IsPhysicalBackupsEnabled: false,
			JwtSecret:                proj.JwtSecret,
		},
		"services": []interface{}{
			ProjectAutoApiService{
				Id:   0,
				Name: "Default API",
				AppConfig: struct {
					Endpoint string `json:"endpoint"`
					DbSchema string `json:"db_schema"`
				}{
					Endpoint: apiEndpoint,
					DbSchema: "public",
				},
				App: struct {
					Id   int    `json:"id"`
					Name string `json:"name"`
				}{
					Id:   1,
					Name: "Auto API",
				},
				ServiceApiKeys: []struct {
					Tags string `json:"tags"`
					Name string `json:"name"`
				}{
					{Tags: "anon", Name: "anon key"},
					{Tags: "service_role", Name: "service_role key"},
				},
				Protocol:      "http",
				Endpoint:      apiEndpoint,
				RestUrl:       fmt.Sprintf("http://localhost:%d/rest/v1/", apiPort),
				Project:       struct{ Ref string `json:"ref"` }{Ref: proj.ProjectRef},
				DefaultApiKey: anonKey,
				ServiceApiKey: serviceKey,
			},
		},
	})
}
