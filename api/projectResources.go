package api

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"supadash/database"
	"supadash/provisioner"
	"time"
)

// --- Project Resources ---

type ResourcesResponse struct {
	ProjectRef        string  `json:"project_ref"`
	Plan              string  `json:"plan"`
	CPULimit          float64 `json:"cpu_limit"`
	CPUReservation    float64 `json:"cpu_reservation"`
	MemoryLimitMB     int64   `json:"memory_limit_mb"`
	MemoryReservMB    int64   `json:"memory_reservation_mb"`
	BurstEligible     bool    `json:"burst_eligible"`
	BurstPriority     int32   `json:"burst_priority"`
}

type UpdateResourcesBody struct {
	Plan              string  `json:"plan"`
	CPULimit          float64 `json:"cpu_limit"`
	MemoryLimitMB     int64   `json:"memory_limit_mb"`
	BurstEligible     *bool   `json:"burst_eligible,omitempty"`
}

func (a *Api) getProjectResources(c *gin.Context) {
	_, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(401, gin.H{"error": "Unauthorized"})
		return
	}

	projectRef := c.Param("ref")
	res, err := a.queries.GetProjectResources(c.Request.Context(), projectRef)
	if err != nil {
		// Return defaults if no resource record exists
		defaults := provisioner.GetDefaultQuotas(provisioner.PlanFree)
		c.JSON(http.StatusOK, ResourcesResponse{
			ProjectRef:     projectRef,
			Plan:           "FREE",
			CPULimit:       defaults.CPULimit,
			CPUReservation: defaults.CPULimit / 2,
			MemoryLimitMB:  defaults.MemoryLimit / (1024 * 1024),
			MemoryReservMB: defaults.MemoryLimit / (1024 * 1024) / 2,
			BurstEligible:  true,
			BurstPriority:  0,
		})
		return
	}

	c.JSON(http.StatusOK, ResourcesResponse{
		ProjectRef:     res.ProjectRef,
		Plan:           res.Plan,
		CPULimit:       res.CPULimit,
		CPUReservation: res.CPUReservation,
		MemoryLimitMB:  res.MemoryLimit / (1024 * 1024),
		MemoryReservMB: res.MemoryReservation / (1024 * 1024),
		BurstEligible:  res.BurstEligible,
		BurstPriority:  res.BurstPriority,
	})
}

func (a *Api) putProjectResources(c *gin.Context) {
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

	var body UpdateResourcesBody
	if err := c.BindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": "Bad Request"})
		return
	}

	// Validate plan
	plan := provisioner.QuotaPlan(body.Plan)
	defaults := provisioner.GetDefaultQuotas(plan)

	cpuLimit := body.CPULimit
	if cpuLimit <= 0 {
		cpuLimit = defaults.CPULimit
	}
	memoryLimit := body.MemoryLimitMB * 1024 * 1024
	if memoryLimit <= 0 {
		memoryLimit = defaults.MemoryLimit
	}

	burstEligible := true
	if body.BurstEligible != nil {
		burstEligible = *body.BurstEligible
	}

	// Save to DB
	res, err := a.queries.UpsertProjectResources(c.Request.Context(), database.UpsertProjectResourcesParams{
		ProjectRef:        projectRef,
		Plan:              string(plan),
		CPULimit:          cpuLimit,
		CPUReservation:    cpuLimit / 2,
		MemoryLimit:       memoryLimit,
		MemoryReservation: memoryLimit / 2,
		BurstEligible:     burstEligible,
		BurstPriority:     0,
	})
	if err != nil {
		a.logger.Error(fmt.Sprintf("Failed to update resources for %s: %v", projectRef, err))
		c.JSON(500, gin.H{"error": "Internal Server Error"})
		return
	}

	// Apply limits via provisioner (async — don't block API)
	if a.resourceManager != nil {
		go func() {
			if err := a.resourceManager.SetProjectResources(c.Request.Context(), projectRef, cpuLimit, memoryLimit); err != nil {
				a.logger.Error(fmt.Sprintf("Failed to apply resource limits for %s: %v", projectRef, err))
			}
		}()
	}

	c.JSON(http.StatusOK, ResourcesResponse{
		ProjectRef:     res.ProjectRef,
		Plan:           res.Plan,
		CPULimit:       res.CPULimit,
		CPUReservation: res.CPUReservation,
		MemoryLimitMB:  res.MemoryLimit / (1024 * 1024),
		MemoryReservMB: res.MemoryReservation / (1024 * 1024),
		BurstEligible:  res.BurstEligible,
		BurstPriority:  res.BurstPriority,
	})
}

// --- Analysis ---

func (a *Api) getProjectAnalysis(c *gin.Context) {
	_, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(401, gin.H{"error": "Unauthorized"})
		return
	}

	projectRef := c.Param("ref")

	// Get recent snapshots (last hour)
	snapshots, err := a.queries.GetRecentSnapshots(c.Request.Context(), projectRef, time.Now().Add(-1*time.Hour))
	if err != nil {
		c.JSON(500, gin.H{"error": "Internal Server Error"})
		return
	}

	// Get active recommendations
	recommendations, err := a.queries.GetActiveRecommendations(c.Request.Context(), projectRef)
	if err != nil {
		recommendations = []database.ResourceRecommendation{}
	}

	c.JSON(http.StatusOK, gin.H{
		"project_ref":     projectRef,
		"snapshot_count":  len(snapshots),
		"snapshots":       snapshots,
		"recommendations": recommendations,
	})
}

func (a *Api) getProjectAnalysisHistory(c *gin.Context) {
	_, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(401, gin.H{"error": "Unauthorized"})
		return
	}

	projectRef := c.Param("ref")
	rangeParam := c.DefaultQuery("range", "24h")

	var since time.Time
	switch rangeParam {
	case "1h":
		since = time.Now().Add(-1 * time.Hour)
	case "24h":
		since = time.Now().Add(-24 * time.Hour)
	case "7d":
		since = time.Now().Add(-7 * 24 * time.Hour)
	case "30d":
		since = time.Now().Add(-30 * 24 * time.Hour)
	default:
		since = time.Now().Add(-24 * time.Hour)
	}

	hourly, err := a.queries.GetHourlySnapshots(c.Request.Context(), projectRef, since)
	if err != nil {
		c.JSON(500, gin.H{"error": "Internal Server Error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"project_ref": projectRef,
		"range":       rangeParam,
		"data_points": len(hourly),
		"history":     hourly,
	})
}

func (a *Api) getProjectRecommendations(c *gin.Context) {
	_, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(401, gin.H{"error": "Unauthorized"})
		return
	}

	projectRef := c.Param("ref")
	recommendations, err := a.queries.GetActiveRecommendations(c.Request.Context(), projectRef)
	if err != nil {
		c.JSON(500, gin.H{"error": "Internal Server Error"})
		return
	}

	c.JSON(http.StatusOK, recommendations)
}

func (a *Api) dismissRecommendation(c *gin.Context) {
	_, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(401, gin.H{"error": "Unauthorized"})
		return
	}

	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid recommendation ID"})
		return
	}

	if err := a.queries.DismissRecommendation(c.Request.Context(), int32(id)); err != nil {
		c.JSON(500, gin.H{"error": "Internal Server Error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "dismissed"})
}

// --- Server-level Resources ---

func (a *Api) getServerResources(c *gin.Context) {
	if a.resourceManager == nil {
		c.JSON(503, gin.H{"error": "Resource manager not available"})
		return
	}

	capacity := a.resourceManager.GetServerCapacity()
	c.JSON(http.StatusOK, capacity)
}

func (a *Api) getServerCapacity(c *gin.Context) {
	if a.resourceManager == nil {
		c.JSON(503, gin.H{"error": "Resource manager not available"})
		return
	}

	capacity := a.resourceManager.GetServerCapacity()

	// Add burst pool info
	var burstStatus interface{}
	if a.burstPool != nil {
		burstStatus = a.burstPool.GetStatus()
	}

	c.JSON(http.StatusOK, gin.H{
		"capacity":   capacity,
		"burst_pool": burstStatus,
	})
}
