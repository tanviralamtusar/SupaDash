package provisioner

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ProjectSecrets contains all generated secrets for a new project
type ProjectSecrets struct {
	JWTSecret     string
	AnonKey       string
	ServiceKey    string
	DBPassword    string
	DashboardUser string
	DashboardPass string
	SecretKeyBase string
	VaultEncKey   string
	LogflareKey   string
}

// GenerateRandomString generates a cryptographically secure random string of the given length
func GenerateRandomString(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random string: %w", err)
	}
	return hex.EncodeToString(bytes)[:length], nil
}

// GenerateRandomBase64 generates a cryptographically secure random base64 string
func GenerateRandomBase64(byteLength int) (string, error) {
	bytes := make([]byte, byteLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random base64: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

// GenerateJWT creates a properly signed HS256 JWT token for Supabase
func GenerateJWT(role string, jwtSecret string) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"role": role,
		"iss":  "supabase",
		"iat":  now.Unix(),
		"exp":  now.Add(365 * 24 * time.Hour).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT for role %s: %w", role, err)
	}
	return signedToken, nil
}

// GenerateProjectSecrets generates all secrets needed for a new Supabase project
func GenerateProjectSecrets() (*ProjectSecrets, error) {
	jwtSecret, err := GenerateRandomString(64)
	if err != nil {
		return nil, fmt.Errorf("failed to generate JWT secret: %w", err)
	}

	anonKey, err := GenerateJWT("anon", jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to generate anon key: %w", err)
	}

	serviceKey, err := GenerateJWT("service_role", jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to generate service key: %w", err)
	}

	dbPassword, err := GenerateRandomString(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate DB password: %w", err)
	}

	dashboardPass, err := GenerateRandomString(16)
	if err != nil {
		return nil, fmt.Errorf("failed to generate dashboard password: %w", err)
	}

	secretKeyBase, err := GenerateRandomBase64(48)
	if err != nil {
		return nil, fmt.Errorf("failed to generate secret key base: %w", err)
	}

	vaultEncKey, err := GenerateRandomString(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate vault enc key: %w", err)
	}

	logflareKey, err := GenerateRandomString(64)
	if err != nil {
		return nil, fmt.Errorf("failed to generate logflare key: %w", err)
	}

	return &ProjectSecrets{
		JWTSecret:     jwtSecret,
		AnonKey:       anonKey,
		ServiceKey:    serviceKey,
		DBPassword:    dbPassword,
		DashboardUser: "supabase",
		DashboardPass: dashboardPass,
		SecretKeyBase: secretKeyBase,
		VaultEncKey:   vaultEncKey,
		LogflareKey:   logflareKey,
	}, nil
}
