package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"supadash/database"
)

type CreateOrgParams struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	Size string `json:"size"`
	Tier string `json:"tier"`
}

func (a *Api) postPlatformOrganizations(c *gin.Context) {
	account, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(401, gin.H{"error": "Unauthorized"})
		return
	}

	var params CreateOrgParams
	if err := c.ShouldBindJSON(&params); err != nil {
		c.JSON(400, gin.H{"error": "Bad Request"})
		return
	}

	params.Name = strings.TrimSpace(params.Name)
	if params.Name == "" {
		c.JSON(400, gin.H{"error": "Organization name is required"})
		return
	}
	if len(params.Name) > 100 {
		c.JSON(400, gin.H{"error": "Organization name must be 100 characters or fewer"})
		return
	}

	// Create the organization and the creator's owner membership atomically:
	// an org without an owner would be permanently inaccessible.
	tx, err := a.pgPool.BeginTx(c.Request.Context(), pgx.TxOptions{})
	if err != nil {
		a.logger.Error(fmt.Sprintf("Failed to begin transaction: %v", err))
		c.JSON(500, gin.H{"error": "Internal Server Error"})
		return
	}
	defer tx.Rollback(c.Request.Context())

	qtx := database.New(tx)

	org, err := qtx.CreateOrganization(c.Request.Context(), params.Name)
	if err != nil {
		a.logger.Error(fmt.Sprintf("Failed to create organization: %v", err))
		c.JSON(500, gin.H{"error": "Internal Server Error"})
		return
	}

	if _, err := qtx.CreateOrganizationMembership(c.Request.Context(), database.CreateOrganizationMembershipParams{
		OrganizationID: org.ID,
		AccountID:      account.ID,
		Role:           "owner",
	}); err != nil {
		a.logger.Error(fmt.Sprintf("Failed to create owner membership for org %d: %v", org.ID, err))
		c.JSON(500, gin.H{"error": "Internal Server Error"})
		return
	}

	if err := tx.Commit(c.Request.Context()); err != nil {
		a.logger.Error(fmt.Sprintf("Failed to commit organization creation: %v", err))
		c.JSON(500, gin.H{"error": "Internal Server Error"})
		return
	}

	// Studio reads the new org (slug in particular) from the response
	c.JSON(http.StatusCreated, Organization{
		Slug:         org.Slug,
		Name:         org.Name,
		BillingEmail: account.Email,
		IsOwner:      true,
		OptInTags:    []string{},
		Id:           org.ID,
		Plan: OrganizationPlan{
			Id:   "enterprise",
			Name: "Enterprise",
		},
	})
}
