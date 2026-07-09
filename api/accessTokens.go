package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"supadash/database"
)

// patPrefix matches the Supabase PAT convention so existing MCP client
// configs (sbp_... tokens) work unchanged.
const patPrefix = "sbp_"

// generateAccessToken returns a new PAT and its storage hash.
func generateAccessToken() (token string, hash string, err error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token = patPrefix + hex.EncodeToString(raw)
	return token, hashAccessToken(token), nil
}

func hashAccessToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

type AccessTokenResponse struct {
	Id         int32  `json:"id"`
	Name       string `json:"name"`
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at,omitempty"`
	Token      string `json:"token,omitempty"` // only present on creation
}

func accessTokenResponseFromRow(t database.PersonalAccessToken) AccessTokenResponse {
	resp := AccessTokenResponse{
		Id:        t.ID,
		Name:      t.Name,
		CreatedAt: t.CreatedAt.Time.Format(time.RFC3339),
	}
	if t.LastUsedAt.Valid {
		resp.LastUsedAt = t.LastUsedAt.Time.Format(time.RFC3339)
	}
	return resp
}

// GET /profile/access-tokens
func (a *Api) getAccessTokens(c *gin.Context) {
	account, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	tokens, err := a.queries.GetAccessTokensForAccount(c.Request.Context(), account.ID)
	if err != nil {
		a.logger.Error(fmt.Sprintf("Failed to list access tokens: %v", err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
		return
	}

	response := make([]AccessTokenResponse, 0, len(tokens))
	for _, t := range tokens {
		response = append(response, accessTokenResponseFromRow(t))
	}
	c.JSON(http.StatusOK, response)
}

type CreateAccessTokenBody struct {
	Name string `json:"name" binding:"required"`
}

// POST /profile/access-tokens
func (a *Api) postAccessTokens(c *gin.Context) {
	account, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var body CreateAccessTokenBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Token name is required"})
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Token name is required"})
		return
	}

	token, hash, err := generateAccessToken()
	if err != nil {
		a.logger.Error(fmt.Sprintf("Failed to generate access token: %v", err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
		return
	}

	row, err := a.queries.CreateAccessToken(c.Request.Context(), database.CreateAccessTokenParams{
		AccountID: account.ID,
		Name:      body.Name,
		TokenHash: hash,
	})
	if err != nil {
		a.logger.Error(fmt.Sprintf("Failed to store access token: %v", err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
		return
	}

	// The plaintext token is shown exactly once
	response := accessTokenResponseFromRow(row)
	response.Token = token
	c.JSON(http.StatusCreated, response)
}

// DELETE /profile/access-tokens/:id
func (a *Api) deleteAccessToken(c *gin.Context) {
	account, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid token id"})
		return
	}

	if err := a.queries.DeleteAccessToken(c.Request.Context(), int32(id), account.ID); err != nil {
		a.logger.Error(fmt.Sprintf("Failed to delete access token: %v", err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Token revoked"})
}
