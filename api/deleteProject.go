package api

import (
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"supadash/database"
)

func (a *Api) deleteProject(c *gin.Context) {
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

	// Update status to GOING_DOWN
	if _, err := a.queries.UpdateProjectStatus(c.Request.Context(), database.UpdateProjectStatusParams{
		ProjectRef: projectRef,
		Status:     "GOING_DOWN",
	}); err != nil {
		a.logger.Error(fmt.Sprintf("Failed to update status to GOING_DOWN: %v", err))
	}
	a.wsHub.BroadcastStatus(projectRef, "GOING_DOWN")

	// Delete via provisioner + clean up DB asynchronously
	go func() {
		ctx := context.Background()
		a.logger.Info(fmt.Sprintf("Deleting project %s", projectRef))

		// Step 1: Stop and remove Docker containers
		if a.provisioner != nil {
			if err := a.provisioner.DeleteProject(ctx, projectRef); err != nil {
				a.logger.Error(fmt.Sprintf("Failed to delete containers for %s: %v", projectRef, err))
				// Continue with DB cleanup anyway
			}
		}

		// Step 2: Delete env vars from DB
		if err := a.queries.DeleteProjectEnvVars(ctx, projectRef); err != nil {
			a.logger.Warn(fmt.Sprintf("Failed to delete env vars for %s: %v", projectRef, err))
		}

		// Step 3: Delete project from DB
		if err := a.queries.DeleteProject(ctx, projectRef); err != nil {
			a.logger.Error(fmt.Sprintf("Failed to delete project %s from DB: %v", projectRef, err))
			return
		}

		a.logger.Info(fmt.Sprintf("Project %s deleted successfully", projectRef))
	}()

	c.JSON(http.StatusOK, gin.H{"status": "GOING_DOWN"})
}
