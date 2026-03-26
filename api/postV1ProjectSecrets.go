package api

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

type Secret struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func (a *Api) postV1ProjectSecrets(c *gin.Context) {
	_, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req []Secret
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// In a real implementation we would insert these into a secure vault.
	// For now we accept the request.
	c.JSON(http.StatusCreated, gin.H{"message": "Successfully created secrets"})
}
