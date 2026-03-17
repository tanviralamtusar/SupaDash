package api

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
	"supadash/database"
)

type EnvVarResponse struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	IsSecret bool   `json:"is_secret"`
}

type EnvVarUpdateBody struct {
	Vars []struct {
		Key      string `json:"key"`
		Value    string `json:"value"`
		IsSecret bool   `json:"is_secret"`
	} `json:"vars"`
}

// getProjectEnvVars returns all environment variables for a project
// Secret values are masked unless ?reveal=true is passed
func (a *Api) getProjectEnvVars(c *gin.Context) {
	_, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(401, gin.H{"error": "Unauthorized"})
		return
	}

	projectRef := c.Param("ref")
	_, err = a.queries.GetProjectByRef(c.Request.Context(), projectRef)
	if err != nil {
		c.JSON(404, gin.H{"error": "Project not found"})
		return
	}

	envVars, err := a.queries.GetProjectEnvVars(c.Request.Context(), projectRef)
	if err != nil {
		a.logger.Error(fmt.Sprintf("Failed to get env vars for %s: %v", projectRef, err))
		c.JSON(500, gin.H{"error": "Internal Server Error"})
		return
	}

	reveal := c.Query("reveal") == "true"
	response := make([]EnvVarResponse, 0, len(envVars))
	for _, ev := range envVars {
		value := ev.Value
		if ev.IsSecret && !reveal {
			// Mask secret values: show first 4 chars + ****
			if len(value) > 4 {
				value = value[:4] + strings.Repeat("*", len(value)-4)
			} else {
				value = "****"
			}
		}
		response = append(response, EnvVarResponse{
			Key:      ev.Key,
			Value:    value,
			IsSecret: ev.IsSecret,
		})
	}

	c.JSON(http.StatusOK, response)
}

// putProjectEnvVars updates environment variables for a project
func (a *Api) putProjectEnvVars(c *gin.Context) {
	_, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(401, gin.H{"error": "Unauthorized"})
		return
	}

	projectRef := c.Param("ref")
	_, err = a.queries.GetProjectByRef(c.Request.Context(), projectRef)
	if err != nil {
		c.JSON(404, gin.H{"error": "Project not found"})
		return
	}

	var body EnvVarUpdateBody
	if err := c.BindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": "Bad Request"})
		return
	}

	// Upsert each var
	for _, v := range body.Vars {
		if err := a.queries.UpsertProjectEnvVar(c.Request.Context(), database.UpsertProjectEnvVarParams{
			ProjectRef: projectRef,
			Key:        v.Key,
			Value:      v.Value,
			IsSecret:   v.IsSecret,
		}); err != nil {
			a.logger.Error(fmt.Sprintf("Failed to upsert env var %s for %s: %v", v.Key, projectRef, err))
			c.JSON(500, gin.H{"error": fmt.Sprintf("Failed to save env var: %s", v.Key)})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": "updated", "count": len(body.Vars)})
}
