package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// getProjectPoolingConfig serves GET /platform/projects/:ref/config/supavisor
//
// Studio's usePoolingConfigurationQuery expects a SupavisorConfigResponse
// array. Self-hosted SupaDash projects don't run a per-project Supavisor
// pooler, so we return an empty array: the query resolves 200 (clearing the
// console 404), and the Connect dialog simply omits a pooler option instead of
// showing a broken one. Studio guards the empty case with optional chaining.
func (a *Api) getProjectPoolingConfig(c *gin.Context) {
	_, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(401, gin.H{"error": "Unauthorized"})
		return
	}

	c.JSON(http.StatusOK, []interface{}{})
}
