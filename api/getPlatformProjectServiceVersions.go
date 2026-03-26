package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (a *Api) getPlatformProjectServiceVersions(c *gin.Context) {
	_, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(401, gin.H{"error": "Unauthorized"})
		return
	}

	// Return stub service versions for self-hosted projects
	c.JSON(http.StatusOK, gin.H{
		"gotrue":   "self-hosted",
		"postgrest": "self-hosted",
		"kong":     "self-hosted",
		"realtime": "self-hosted",
		"storage":  "self-hosted",
	})
}
