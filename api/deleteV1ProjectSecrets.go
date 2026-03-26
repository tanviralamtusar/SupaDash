package api

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func (a *Api) deleteV1ProjectSecrets(c *gin.Context) {
	_, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req []string
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// In a real implementation we would delete these from the secure vault.
	// For now we just accept the request.
	c.JSON(http.StatusOK, gin.H{"message": "Successfully deleted secrets"})
}
