package api

import (
	"bytes"
	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
	"image/png"
	"net/http"
	"supadash/database"
)

type MfaVerifyBody struct {
	TotpCode string `json:"totp_code" binding:"required"`
}

func (a *Api) postMfaSetup(c *gin.Context) {
	account, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	if account.TotpEnabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "2FA is already enabled"})
		return
	}

	// Generate a new TOTP key
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "SupaDash",
		AccountName: account.Email,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate TOTP secret"})
		return
	}

	// Save the secret temporarily or directly into the account but not enabled yet
	err = a.queries.Setup2FA(c.Request.Context(), database.Setup2FAParams{
		ID:         account.ID,
		TotpSecret: []byte(key.Secret()),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save TOTP secret"})
		return
	}

	// Generate QR code
	var buf bytes.Buffer
	img, err := key.Image(200, 200)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate QR code"})
		return
	}
	_ = png.Encode(&buf, img)

	c.JSON(http.StatusOK, gin.H{
		"secret": key.Secret(),
		"qr_uri": key.URL(), // UI can use a QR code library to render this URI, or we could return base64 image
	})
}

func (a *Api) postMfaVerify(c *gin.Context) {
	account, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var body MfaVerifyBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if account.TotpEnabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "2FA is already enabled"})
		return
	}

	if string(account.TotpSecret) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "2FA setup has not been initiated"})
		return
	}

	// Validate the provided code against the secret
	valid := totp.Validate(body.TotpCode, string(account.TotpSecret))
	if !valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid TOTP code"})
		return
	}

	// Enable 2FA
	err = a.queries.Enable2FA(c.Request.Context(), account.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to enable 2FA"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "2FA successfully enabled"})
}

func (a *Api) deleteMfa(c *gin.Context) {
	account, err := a.GetAccountFromRequest(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Could require password/MFA to disable, but keeping it simple for now
	err = a.queries.Disable2FA(c.Request.Context(), account.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to disable 2FA"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "2FA successfully disabled"})
}
