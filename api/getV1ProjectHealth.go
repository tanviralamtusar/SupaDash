package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// getV1ProjectHealth serves GET /v1/projects/:ref/health?services=auth,realtime,rest,storage
//
// Studio's useProjectServiceStatusQuery expects an ARRAY of
// { name, healthy, status } objects (Management API v1 shape) — NOT the
// { "services": [...] } object that the legacy getProjectHealth returns. The
// `services` query param filters which services to report on. For self-hosted
// projects every requested service is reported healthy.
func (a *Api) getV1ProjectHealth(c *gin.Context) {
	_, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(401, gin.H{"error": "Unauthorized"})
		return
	}

	requested := c.Query("services")
	services := []string{}
	if requested != "" {
		for _, s := range strings.Split(requested, ",") {
			if s = strings.TrimSpace(s); s != "" {
				services = append(services, s)
			}
		}
	}
	if len(services) == 0 {
		services = []string{"auth", "realtime", "rest", "storage", "db"}
	}

	statuses := make([]gin.H, 0, len(services))
	for _, name := range services {
		statuses = append(statuses, gin.H{
			"name":    name,
			"healthy": true,
			"status":  "ACTIVE_HEALTHY",
		})
	}

	c.JSON(http.StatusOK, statuses)
}
