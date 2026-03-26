package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (a *Api) getPlatformProjectMembers(c *gin.Context) {
	account, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(401, gin.H{"error": "Unauthorized"})
		return
	}

	// Return the current user as the sole project member with owner role
	c.JSON(http.StatusOK, []gin.H{
		{
			"gotrue_id":     account.GotrueID,
			"primary_email": account.Email,
			"username":      account.Username,
			"role_ids":      []int{4}, // Owner role
		},
	})
}
