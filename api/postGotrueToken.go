package api

import (
	"crypto/rand"
	"encoding/base64"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/matthewhartstonge/argon2"
	"github.com/pquerna/otp/totp"
	"net/http"
	"supadash/database"
	"time"
)

type GotrueToken struct {
	Email              string `json:"email"`
	Password           string `json:"password"`
	TempToken          string `json:"temp_token"`
	TotpCode           string `json:"totp_code"`
	GotrueMetaSecurity struct {
		CaptchaToken string `json:"captcha_token"`
	} `json:"gotrue_meta_security"`
}

func (a *Api) postGotrueToken(c *gin.Context) {
	var body GotrueToken
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	var account database.Account

	// Handle MFA verification flow
	if body.TempToken != "" && body.TotpCode != "" {
		token, err := jwt.Parse(body.TempToken, func(token *jwt.Token) (interface{}, error) {
			return []byte(a.config.JwtSecret), nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired temp token"})
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok || claims["type"] != "mfa_temp" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token type"})
			return
		}

		gotrueID := claims["sub"].(string)
		account, err = a.queries.GetAccountByGoTrueID(c.Request.Context(), gotrueID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Account not found"})
			return
		}

		if !account.TotpEnabled || string(account.TotpSecret) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "2FA is not enabled for this account"})
			return
		}

		valid := totp.Validate(body.TotpCode, string(account.TotpSecret))
		if !valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid TOTP code"})
			return
		}
	} else {
		// Normal login flow
		var err error
		account, err = a.queries.GetAccountByEmail(c.Request.Context(), body.Email)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Account not found"})
			return
		}

		if verified, err := argon2.VerifyEncoded([]byte(body.Password), []byte(account.PasswordHash)); err != nil || !verified {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid password"})
			return
		}

		// Check if 2FA is enabled
		if account.TotpEnabled {
			// Generate a short-lived temp token for MFA verification (5 minutes)
			tempClaims := jwt.MapClaims{
				"iss":  "supadash.io",
				"sub":  account.GotrueID,
				"aud":  []string{"supadash.io"},
				"type": "mfa_temp",
				"exp":  time.Now().Add(5 * time.Minute).Unix(),
				"iat":  time.Now().Unix(),
			}
			tempTokenJwt := jwt.NewWithClaims(jwt.SigningMethodHS256, tempClaims)
			signedTempToken, err := tempTokenJwt.SignedString([]byte(a.config.JwtSecret))
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create temp token"})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"requires_2fa": true,
				"temp_token":   signedTempToken,
			})
			return
		}
	}

	// Validation successful (either normal or MFA), issue final Access Token
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	// Generate secure refresh token
	refreshBytes := make([]byte, 32)
	if _, err := rand.Read(refreshBytes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate refresh token"})
		return
	}
	refreshToken := base64.URLEncoding.EncodeToString(refreshBytes)

	// Save refresh token to DB (valid for 30 days)
	_, err = a.queries.InsertRefreshToken(c.Request.Context(), database.InsertRefreshTokenParams{
		AccountID: account.ID,
		Token:     refreshToken,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().AddDate(0, 0, 30), Valid: true},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save refresh token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token":  signedJwt,
		"token_type":    "Bearer",
		"expires_in":    3600,
		"refresh_token": refreshToken,
		"user": gin.H{
			"id":    account.ID,
			"email": account.Email,
			"app_metadata": gin.H{
				"provider": "email",
			},
		},
	})
}
