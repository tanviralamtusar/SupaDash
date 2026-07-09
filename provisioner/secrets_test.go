package provisioner

import (
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateRandomString(t *testing.T) {
	t.Run("returns correct length", func(t *testing.T) {
		s, err := GenerateRandomString(32)
		require.NoError(t, err)
		assert.Len(t, s, 32)
	})

	t.Run("returns hex characters only", func(t *testing.T) {
		s, err := GenerateRandomString(64)
		require.NoError(t, err)
		for _, c := range s {
			assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
				"expected hex char, got %c", c)
		}
	})

	t.Run("returns unique values", func(t *testing.T) {
		a, _ := GenerateRandomString(32)
		b, _ := GenerateRandomString(32)
		assert.NotEqual(t, a, b)
	})

	t.Run("handles small length", func(t *testing.T) {
		s, err := GenerateRandomString(1)
		require.NoError(t, err)
		assert.Len(t, s, 1)
	})
}

func TestGenerateRandomBase64(t *testing.T) {
	t.Run("returns non-empty string", func(t *testing.T) {
		s, err := GenerateRandomBase64(48)
		require.NoError(t, err)
		assert.NotEmpty(t, s)
	})

	t.Run("returns URL-safe base64", func(t *testing.T) {
		s, err := GenerateRandomBase64(32)
		require.NoError(t, err)
		// URL-safe base64 must not contain + or /
		assert.False(t, strings.ContainsAny(s, "+/"), "should not contain + or /")
	})

	t.Run("returns unique values", func(t *testing.T) {
		a, _ := GenerateRandomBase64(32)
		b, _ := GenerateRandomBase64(32)
		assert.NotEqual(t, a, b)
	})
}

func TestGenerateJWT(t *testing.T) {
	secret := "test-secret-key-1234567890abcdef"

	t.Run("generates valid HS256 token", func(t *testing.T) {
		tokenStr, err := GenerateJWT("anon", secret)
		require.NoError(t, err)
		assert.NotEmpty(t, tokenStr)

		// Parse and verify
		token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
			return []byte(secret), nil
		})
		require.NoError(t, err)
		assert.True(t, token.Valid)
	})

	t.Run("contains correct claims", func(t *testing.T) {
		tokenStr, err := GenerateJWT("service_role", secret)
		require.NoError(t, err)

		token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
			return []byte(secret), nil
		})
		require.NoError(t, err)

		claims, ok := token.Claims.(jwt.MapClaims)
		require.True(t, ok)
		assert.Equal(t, "service_role", claims["role"])
		assert.Equal(t, "supabase", claims["iss"])
		assert.NotNil(t, claims["iat"])
		assert.NotNil(t, claims["exp"])
	})

	t.Run("fails with wrong secret", func(t *testing.T) {
		tokenStr, err := GenerateJWT("anon", secret)
		require.NoError(t, err)

		_, err = jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
			return []byte("wrong-secret"), nil
		})
		assert.Error(t, err)
	})
}

func TestGenerateProjectSecrets(t *testing.T) {
	secrets, err := GenerateProjectSecrets()
	require.NoError(t, err)
	require.NotNil(t, secrets)

	t.Run("all fields non-empty", func(t *testing.T) {
		assert.NotEmpty(t, secrets.JWTSecret)
		assert.NotEmpty(t, secrets.AnonKey)
		assert.NotEmpty(t, secrets.ServiceKey)
		assert.NotEmpty(t, secrets.DBPassword)
		assert.NotEmpty(t, secrets.DashboardPass)
		assert.NotEmpty(t, secrets.SecretKeyBase)
		assert.NotEmpty(t, secrets.VaultEncKey)
		assert.NotEmpty(t, secrets.LogflareKey)
		assert.Equal(t, "supabase", secrets.DashboardUser)
	})

	t.Run("AnonKey is valid JWT signed by JWTSecret", func(t *testing.T) {
		token, err := jwt.Parse(secrets.AnonKey, func(token *jwt.Token) (interface{}, error) {
			return []byte(secrets.JWTSecret), nil
		})
		require.NoError(t, err)
		assert.True(t, token.Valid)

		claims := token.Claims.(jwt.MapClaims)
		assert.Equal(t, "anon", claims["role"])
	})

	t.Run("ServiceKey is valid JWT with service_role", func(t *testing.T) {
		token, err := jwt.Parse(secrets.ServiceKey, func(token *jwt.Token) (interface{}, error) {
			return []byte(secrets.JWTSecret), nil
		})
		require.NoError(t, err)
		assert.True(t, token.Valid)

		claims := token.Claims.(jwt.MapClaims)
		assert.Equal(t, "service_role", claims["role"])
	})

	t.Run("two calls produce different secrets", func(t *testing.T) {
		secrets2, err := GenerateProjectSecrets()
		require.NoError(t, err)
		assert.NotEqual(t, secrets.JWTSecret, secrets2.JWTSecret)
		assert.NotEqual(t, secrets.DBPassword, secrets2.DBPassword)
	})
}

func TestIsInfraSafePassword(t *testing.T) {
	safe := []string{"abc123", "DEADBEEF00", "Password2026", "a"}
	for _, pw := range safe {
		if !IsInfraSafePassword(pw) {
			t.Errorf("IsInfraSafePassword(%q) = false, want true", pw)
		}
	}
	unsafe := []string{"", "@Tanvir@2006@", "has space", "a:b", "p@ss", "a/b", "under_score", "quote\"", "dash-dash"}
	for _, pw := range unsafe {
		if IsInfraSafePassword(pw) {
			t.Errorf("IsInfraSafePassword(%q) = true, want false", pw)
		}
	}
}
