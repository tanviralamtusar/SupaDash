package api

import (
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"supadash/database"
)

func (a *Api) postProjectResume(c *gin.Context) {
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

	if project.Status != "PAUSED" {
		c.JSON(400, gin.H{"error": "Project is not paused"})
		return
	}

	// Update status to COMING_UP
	if _, err := a.queries.UpdateProjectStatus(c.Request.Context(), database.UpdateProjectStatusParams{
		ProjectRef: projectRef,
		Status:     "COMING_UP",
	}); err != nil {
		a.logger.Error(fmt.Sprintf("Failed to update status to COMING_UP: %v", err))
	}
	a.wsHub.BroadcastStatus(projectRef, "COMING_UP")

	// Resume via provisioner asynchronously
	if a.provisioner != nil {
		go func() {
			ctx := context.Background()
			a.logger.Info(fmt.Sprintf("Resuming project %s", projectRef))

			if err := a.provisioner.ResumeProject(ctx, projectRef); err != nil {
				a.logger.Error(fmt.Sprintf("Failed to resume project %s: %v", projectRef, err))
				a.queries.UpdateProjectStatus(ctx, database.UpdateProjectStatusParams{
					ProjectRef: projectRef,
					Status:     "PAUSED",
				})
				a.wsHub.BroadcastStatus(projectRef, "PAUSED")
				return
			}

			a.queries.UpdateProjectStatus(ctx, database.UpdateProjectStatusParams{
				ProjectRef: projectRef,
				Status:     "ACTIVE_HEALTHY",
			})
			a.wsHub.BroadcastStatus(projectRef, "ACTIVE_HEALTHY")
			a.logger.Info(fmt.Sprintf("Project %s resumed successfully", projectRef))
		}()
	} else {
		a.queries.UpdateProjectStatus(c.Request.Context(), database.UpdateProjectStatusParams{
			ProjectRef: projectRef,
			Status:     "ACTIVE_HEALTHY",
		})
	}

	c.JSON(http.StatusOK, gin.H{"status": "COMING_UP"})
}
