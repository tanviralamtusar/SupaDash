package api

import (
	"crypto/rand"
	"encoding/base64"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"net/http"
	"strings"
	"supadash/database"
	"time"
)

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

func (a *Api) postAuthToken(c *gin.Context) {
	grantType := c.Query("grant_type")
	if grantType != "refresh_token" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported grant_type. Only refresh_token is supported."})
		return
	}

	var body RefreshTokenRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: refresh_token is required"})
		return
	}

	// 1. Validate refresh token against DB
	tokenRecord, err := a.queries.GetRefreshToken(c.Request.Context(), body.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid refresh token"})
		return
	}

	if tokenRecord.Revoked || time.Now().After(tokenRecord.ExpiresAt.Time) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Refresh token expired or revoked"})
		return
	}

	// 2. Get Account
	account, err := a.queries.GetAccountByID(c.Request.Context(), tokenRecord.AccountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Account not found"})
		return
	}

	// 3. Revoke old refresh token (rotate it)
	_ = a.queries.RevokeRefreshToken(c.Request.Context(), body.RefreshToken)

	// 4. Generate new Access Token
	claims := jwt.RegisteredClaims{
		Issuer:    "supadash.io",
		Subject:   account.GotrueID,
		Audience:  []string{"supadash.io"},
		ExpiresAt: &jwt.NumericDate{Time: time.Now().Add(1 * time.Hour)},
		NotBefore: &jwt.NumericDate{Time: time.Now()},
		IssuedAt:  &jwt.NumericDate{Time: time.Now()},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedJwt, err := token.SignedString([]byte(a.config.JwtSecret))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to sign new token"})
		return
	}

	// 5. Generate new Refresh Token
	refreshBytes := make([]byte, 32)
	if _, err := rand.Read(refreshBytes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate new refresh token"})
		return
	}
	newRefreshToken := base64.URLEncoding.EncodeToString(refreshBytes)

	// 6. Save new Refresh Token to DB
	_, err = a.queries.InsertRefreshToken(c.Request.Context(), database.InsertRefreshTokenParams{
		AccountID: account.ID,
		Token:     newRefreshToken,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().AddDate(0, 0, 30), Valid: true},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save new refresh token"})
		return
	}

	// 7. Return payload
	c.JSON(http.StatusOK, gin.H{
		"access_token":  signedJwt,
		"token_type":    "Bearer",
		"expires_in":    3600,
		"refresh_token": newRefreshToken,
		"user": gin.H{
			"id":    account.ID,
			"email": account.Email,
			"app_metadata": gin.H{
				"provider": "email",
			},
		},
	})
}

func (a *Api) postAuthLogout(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing Authorization header"})
		return
	}

	// We revoke all refresh tokens for this user
	account, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	err = a.queries.RevokeAllRefreshTokensForUser(c.Request.Context(), account.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke tokens"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}
