package api

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func (a *Api) getProjectUpgradeEligibility(c *gin.Context) {
	_, err := a.GetAccountFromRequest(c)
	if err != nil {
		println(err.Error())
		c.JSON(401, gin.H{"error": "Unauthorized"})
		return
	}

	// Return upgrade eligibility info. For self-hosted, upgrades are managed
	// differently, so nothing is ever eligible. The field names must match
	// Supabase's real shape: Studio reads `current_app_version` (and derives
	// the Postgres major version from it via split("-")/split(".")), and
	// dereferences it WITHOUT a null guard — a missing field crashes the page.
	c.JSON(http.StatusOK, gin.H{
		"eligible":                            false,
		"current_app_version":                 DefaultAppVersion,
		"current_app_version_release_channel": "ga",
		"latest_app_version":                  DefaultAppVersion,
		"target_upgrade_versions":             []interface{}{},
		"potential_breaking_changes":          []interface{}{},
		"duration_estimate_hours":             0,
		"legacy_auth_custom_roles":            []interface{}{},
		"extension_dependent_objects":         []interface{}{},
	})
}
