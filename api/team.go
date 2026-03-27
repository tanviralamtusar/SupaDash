package api

import (
	"fmt"
	"net/http"
	"supadash/database"
	"supadash/provisioner"

	"github.com/gin-gonic/gin"
)

type InviteMemberRequest struct {
	Email string `json:"email" binding:"required"`
	Role  string `json:"role" binding:"required"`
}

type UpdateRoleRequest struct {
	Role string `json:"role" binding:"required"`
}

func (a *Api) getTeamMembers(c *gin.Context) {
	slug := c.Param("slug")

	org, err := a.queries.GetOrganizationBySlug(c.Request.Context(), slug)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Organization not found"})
		return
	}

	members, err := a.queries.GetOrganizationMembers(c.Request.Context(), org.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch members"})
		return
	}

	c.JSON(http.StatusOK, members)
}

func (a *Api) inviteTeamMember(c *gin.Context) {
	slug := c.Param("slug")

	var req InviteMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email and role are required"})
		return
	}

	// Caller must be Owner or Admin (enforced by middleware)
	account, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	org, err := a.queries.GetOrganizationBySlug(c.Request.Context(), slug)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Organization not found"})
		return
	}

	// See if target account exists
	targetAccount, err := a.queries.GetAccountByEmail(c.Request.Context(), req.Email)
	if err != nil {
		// Logically you'd create a pending invite here, but for simplicity:
		c.JSON(http.StatusNotFound, gin.H{"error": "Account not found for email"})
		return
	}

	// Add user to membership
	_, err = a.queries.CreateOrganizationMembership(c.Request.Context(), database.CreateOrganizationMembershipParams{
		OrganizationID: org.ID,
		AccountID:      targetAccount.ID,
		Role:           req.Role,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add member, they might already be in the organization"})
		return
	}

	// Send Email
	mailer := provisioner.NewSMTPMailer()
	err = mailer.SendInvitationEmail(req.Email, account.Email, org.Name, req.Role)
	if err != nil {
		a.logger.Error(fmt.Sprintf("Failed to send invitation email to %s: %v", req.Email, err))
		// Optional: Still return success since they were added to the DB
	}

	c.JSON(http.StatusOK, gin.H{"message": "Member invited successfully"})
}

func (a *Api) updateTeamMemberRole(c *gin.Context) {
	slug := c.Param("slug")
	gotrueID := c.Param("id")

	var req UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Role is required"})
		return
	}

	org, err := a.queries.GetOrganizationBySlug(c.Request.Context(), slug)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Organization not found"})
		return
	}

	targetAccount, err := a.queries.GetAccountByGoTrueID(c.Request.Context(), gotrueID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Member account not found"})
		return
	}

	err = a.queries.UpdateOrganizationMemberRole(c.Request.Context(), database.UpdateOrganizationMemberRoleParams{
		OrganizationID: org.ID,
		AccountID:      targetAccount.ID,
		Role:           req.Role,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update member role"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Role updated successfully"})
}

func (a *Api) removeTeamMember(c *gin.Context) {
	slug := c.Param("slug")
	gotrueID := c.Param("id")

	org, err := a.queries.GetOrganizationBySlug(c.Request.Context(), slug)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Organization not found"})
		return
	}

	targetAccount, err := a.queries.GetAccountByGoTrueID(c.Request.Context(), gotrueID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Member account not found"})
		return
	}

	err = a.queries.RemoveOrganizationMember(c.Request.Context(), database.RemoveOrganizationMemberParams{
		OrganizationID: org.ID,
		AccountID:      targetAccount.ID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove member"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Member removed successfully"})
}
