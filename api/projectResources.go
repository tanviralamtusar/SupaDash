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
	ProjectRef          string  `json:"project_ref"`
	Plan                string  `json:"plan"`
	CPULimit            float64 `json:"cpu_limit"`
	CPUReservation      float64 `json:"cpu_reservation"`
	MemoryLimitMB       int64   `json:"memory_limit_mb"`
	MemoryReservMB      int64   `json:"memory_reservation_mb"`
	BurstEligible       bool    `json:"burst_eligible"`
	BurstPriority       int32   `json:"burst_priority"`
	DatabaseSizeLimitMB int64   `json:"database_size_limit_mb"`
	StorageSizeLimitMB  int64   `json:"storage_size_limit_mb"`
	DatabaseSizeMB      int64   `json:"database_size_mb"`
	StorageSizeMB       int64   `json:"storage_size_mb"`
	WritesBlocked       bool    `json:"writes_blocked"`
}

type UpdateResourcesBody struct {
	Plan                string  `json:"plan"`
	CPULimit            float64 `json:"cpu_limit"`
	MemoryLimitMB       int64   `json:"memory_limit_mb"`
	DatabaseSizeLimitMB int64   `json:"database_size_limit_mb"`
	StorageSizeLimitMB  int64   `json:"storage_size_limit_mb"`
	BurstEligible       *bool   `json:"burst_eligible,omitempty"`
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
			ProjectRef:          projectRef,
			Plan:                "FREE",
			CPULimit:            defaults.CPULimit,
			CPUReservation:      defaults.CPULimit / 2,
			MemoryLimitMB:       defaults.MemoryLimit / (1024 * 1024),
			MemoryReservMB:      defaults.MemoryLimit / (1024 * 1024) / 2,
			BurstEligible:       true,
			BurstPriority:       0,
			DatabaseSizeLimitMB: defaults.DatabaseSize / (1024 * 1024),
			StorageSizeLimitMB:  defaults.StorageSize / (1024 * 1024),
		})
		return
	}

	c.JSON(http.StatusOK, resourcesResponseFromRow(res))
}

// resourcesResponseFromRow converts a DB row into the API response shape.
func resourcesResponseFromRow(res database.ProjectResource) ResourcesResponse {
	cpuLimitVal, _ := res.CpuLimit.Float64Value()
	cpuReservationVal, _ := res.CpuReservation.Float64Value()

	return ResourcesResponse{
		ProjectRef:          res.ProjectRef,
		Plan:                res.Plan,
		CPULimit:            cpuLimitVal.Float64,
		CPUReservation:      cpuReservationVal.Float64,
		MemoryLimitMB:       res.MemoryLimit / (1024 * 1024),
		MemoryReservMB:      res.MemoryReservation / (1024 * 1024),
		BurstEligible:       res.BurstEligible,
		BurstPriority:       res.BurstPriority,
		DatabaseSizeLimitMB: res.DatabaseSizeLimitBytes / (1024 * 1024),
		StorageSizeLimitMB:  res.StorageSizeLimitBytes / (1024 * 1024),
		DatabaseSizeMB:      res.DatabaseSizeBytes / (1024 * 1024),
		StorageSizeMB:       res.StorageSizeBytes / (1024 * 1024),
		WritesBlocked:       res.WritesBlocked,
	}
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

	// Reject requests below platform minimums (500 MB RAM, 1 GB storage, 500 MB DB)
	if err := provisioner.ValidateResourceFloor(
		body.MemoryLimitMB*1024*1024,
		body.StorageSizeLimitMB*1024*1024,
		body.DatabaseSizeLimitMB*1024*1024,
	); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cpuLimit := body.CPULimit
	if cpuLimit <= 0 {
		cpuLimit = defaults.CPULimit
	}
	memoryLimit := body.MemoryLimitMB * 1024 * 1024
	if memoryLimit <= 0 {
		memoryLimit = defaults.MemoryLimit
	}
	// 0 falls back to the plan default (plan defaults are all >= the platform
	// minimums, except unlimited plans where 0 means "no limit").
	databaseSizeLimit := body.DatabaseSizeLimitMB * 1024 * 1024
	if databaseSizeLimit <= 0 {
		databaseSizeLimit = defaults.DatabaseSize
	}
	storageSizeLimit := body.StorageSizeLimitMB * 1024 * 1024
	if storageSizeLimit <= 0 {
		storageSizeLimit = defaults.StorageSize
	}

	burstEligible := true
	if body.BurstEligible != nil {
		burstEligible = *body.BurstEligible
	}

	// Save to DB
	res, err := a.queries.UpsertProjectResources(c.Request.Context(), database.UpsertProjectResourcesParams{
		ProjectRef:             projectRef,
		Plan:                   string(plan),
		CPULimit:               cpuLimit,
		CPUReservation:         cpuLimit / 2,
		MemoryLimit:            memoryLimit,
		MemoryReservation:      memoryLimit / 2,
		BurstEligible:          burstEligible,
		BurstPriority:          0,
		DatabaseSizeLimitBytes: databaseSizeLimit,
		StorageSizeLimitBytes:  storageSizeLimit,
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

	c.JSON(http.StatusOK, resourcesResponseFromRow(res))
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
