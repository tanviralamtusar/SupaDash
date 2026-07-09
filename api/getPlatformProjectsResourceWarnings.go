package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// exhaustionLevel returns Studio's severity string for a usage/limit pair,
// or nil when usage is healthy or the limit is unlimited (0).
func exhaustionLevel(usage, limit int64) interface{} {
	if limit <= 0 {
		return nil
	}
	pct := float64(usage) / float64(limit)
	switch {
	case pct >= 1.0:
		return "critical"
	case pct >= 0.8:
		return "warning"
	default:
		return nil
	}
}

func (a *Api) getPlatformProjectsResourceWarnings(c *gin.Context) {
	account, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(401, gin.H{"error": "Unauthorized"})
		return
	}

	// Only surface warnings for projects the caller can see
	projects, err := a.queries.GetProjectsForAccountId(c.Request.Context(), account.ID)
	if err != nil {
		c.JSON(http.StatusOK, []interface{}{})
		return
	}

	warnings := make([]gin.H, 0, len(projects))
	for _, p := range projects {
		res, err := a.queries.GetProjectResources(c.Request.Context(), p.ProjectRef)
		if err != nil {
			continue
		}

		dbLevel := exhaustionLevel(res.DatabaseSizeBytes, res.DatabaseSizeLimitBytes)
		storageLevel := exhaustionLevel(res.StorageSizeBytes, res.StorageSizeLimitBytes)

		// Combined disk severity: worst of the two
		diskLevel := dbLevel
		if storageLevel == "critical" || diskLevel == nil {
			diskLevel = storageLevel
		}

		if diskLevel == nil && !res.WritesBlocked {
			continue
		}

		warnings = append(warnings, gin.H{
			"project":                   p.ProjectRef,
			"is_readonly_mode_enabled":  res.WritesBlocked,
			"disk_space_exhaustion":     diskLevel,
			"database_size_exhaustion":  dbLevel,
			"storage_size_exhaustion":   storageLevel,
			"cpu_exhaustion":            nil,
			"memory_and_swap_exhaustion": nil,
		})
	}

	c.JSON(http.StatusOK, warnings)
}
