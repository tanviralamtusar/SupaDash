package api

import (
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"supadash/database"
)

func (a *Api) postProjectPause(c *gin.Context) {
	_, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(401, gin.H{"error": "Unauthorized"})
		return
	}

	projectRef := c.Param("ref")
	project, err := a.queries.GetProjectByRef(c.Request.Context(), projectRef)
	if err != nil {
		c.JSON(404, gin.H{"error": "Project not found"})
		return
	}

	if project.Status == "PAUSED" {
		c.JSON(400, gin.H{"error": "Project is already paused"})
		return
	}

	// Update status to PAUSING
	if _, err := a.queries.UpdateProjectStatus(c.Request.Context(), database.UpdateProjectStatusParams{
		ProjectRef: projectRef,
		Status:     "PAUSING",
	}); err != nil {
		a.logger.Error(fmt.Sprintf("Failed to update status to PAUSING: %v", err))
	}
	a.wsHub.BroadcastStatus(projectRef, "PAUSING")

	// Pause via provisioner asynchronously
	if a.provisioner != nil {
		go func() {
			ctx := context.Background()
			a.logger.Info(fmt.Sprintf("Pausing project %s", projectRef))

			if err := a.provisioner.PauseProject(ctx, projectRef); err != nil {
				a.logger.Error(fmt.Sprintf("Failed to pause project %s: %v", projectRef, err))
				a.queries.UpdateProjectStatus(ctx, database.UpdateProjectStatusParams{
					ProjectRef: projectRef,
					Status:     "ACTIVE_HEALTHY",
				})
				a.wsHub.BroadcastStatus(projectRef, "ACTIVE_HEALTHY")
				return
			}

			a.queries.UpdateProjectStatus(ctx, database.UpdateProjectStatusParams{
				ProjectRef: projectRef,
				Status:     "PAUSED",
			})
			a.wsHub.BroadcastStatus(projectRef, "PAUSED")
			a.logger.Info(fmt.Sprintf("Project %s paused successfully", projectRef))
		}()
	} else {
		// No provisioner — just update DB status
		a.queries.UpdateProjectStatus(c.Request.Context(), database.UpdateProjectStatusParams{
			ProjectRef: projectRef,
			Status:     "PAUSED",
		})
	}

	c.JSON(http.StatusOK, gin.H{"status": "PAUSING"})
}
