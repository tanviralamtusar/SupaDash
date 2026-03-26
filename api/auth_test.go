package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"supadash/conf"
	"supadash/database"

	"github.com/gin-gonic/gin"
	"github.com/matthewhartstonge/argon2"
	"github.com/pquerna/otp/totp"
)

// fakeAuthQuerier stubs the methods used by the auth handlers.
// Methods not relevant to auth tests panic so missing coverage is obvious.
type fakeAuthQuerier struct {
	unimplementedQuerier
	accounts      map[string]database.Account // keyed by email
	accountsByGID map[string]database.Account // keyed by gotrue_id
}

func newFakeAuthQuerier() *fakeAuthQuerier {
	return &fakeAuthQuerier{
		accounts:      make(map[string]database.Account),
		accountsByGID: make(map[string]database.Account),
	}
}

func (f *fakeAuthQuerier) GetAccountByEmail(_ context.Context, email string) (database.Account, error) {
	a, ok := f.accounts[email]
	if !ok {
		return database.Account{}, fmt.Errorf("not found")
	}
	return a, nil
}

func (f *fakeAuthQuerier) GetAccountByGoTrueID(_ context.Context, gotrueID string) (database.Account, error) {
	a, ok := f.accountsByGID[gotrueID]
	if !ok {
		return database.Account{}, fmt.Errorf("not found")
	}
	return a, nil
}

func (f *fakeAuthQuerier) InsertRefreshToken(_ context.Context, _ database.InsertRefreshTokenParams) (database.RefreshToken, error) {
	return database.RefreshToken{}, nil
}

// --- test helpers ---

func setupTestApi(t *testing.T, q Querier) (*Api, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	cfg := &conf.Config{JwtSecret: "test-jwt-secret-at-least-32-chars!!"}
	argonCfg := argon2.DefaultConfig()

	api := &Api{
		config:  cfg,
		queries: q,
		argon:   argonCfg,
	}

	r := gin.New()
	r.POST("/auth/token", api.postGotrueToken)
	return api, r
}

func hashPassword(t *testing.T, password string) string {
	t.Helper()
	cfg := argon2.DefaultConfig()
	encoded, err := cfg.HashEncoded([]byte(password))
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	return string(encoded)
}

func postJSON(r *gin.Engine, path string, body interface{}) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// --- Tests ---

func TestPostGotrueToken_ValidLogin(t *testing.T) {
	q := newFakeAuthQuerier()
	q.accounts["user@test.com"] = database.Account{
		ID:           1,
		GotrueID:     "gid-1",
		Email:        "user@test.com",
		PasswordHash: hashPassword(t, "correct-password"),
		TotpEnabled:  false,
	}

	_, r := setupTestApi(t, q)

	w := postJSON(r, "/auth/token", map[string]string{
		"email":    "user@test.com",
		"password": "correct-password",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["access_token"] == nil || resp["access_token"] == "" {
		t.Fatal("expected access_token in response")
	}
	if resp["refresh_token"] == nil || resp["refresh_token"] == "" {
		t.Fatal("expected refresh_token in response")
	}
}

func TestPostGotrueToken_WrongPassword(t *testing.T) {
	q := newFakeAuthQuerier()
	q.accounts["user@test.com"] = database.Account{
		ID:           1,
		GotrueID:     "gid-1",
		Email:        "user@test.com",
		PasswordHash: hashPassword(t, "correct-password"),
	}

	_, r := setupTestApi(t, q)

	w := postJSON(r, "/auth/token", map[string]string{
		"email":    "user@test.com",
		"password": "wrong-password",
	})

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPostGotrueToken_UnknownEmail(t *testing.T) {
	q := newFakeAuthQuerier()
	_, r := setupTestApi(t, q)

	w := postJSON(r, "/auth/token", map[string]string{
		"email":    "nobody@test.com",
		"password": "whatever",
	})

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPostGotrueToken_MissingBody(t *testing.T) {
	q := newFakeAuthQuerier()
	_, r := setupTestApi(t, q)

	req := httptest.NewRequest(http.MethodPost, "/auth/token", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPostGotrueToken_2FA_ReturnsTempToken(t *testing.T) {
	q := newFakeAuthQuerier()
	q.accounts["mfa@test.com"] = database.Account{
		ID:           2,
		GotrueID:     "gid-2",
		Email:        "mfa@test.com",
		PasswordHash: hashPassword(t, "mfa-password"),
		TotpEnabled:  true,
		TotpSecret:   []byte("JBSWY3DPEHPK3PXP"), // test secret
	}

	_, r := setupTestApi(t, q)

	w := postJSON(r, "/auth/token", map[string]string{
		"email":    "mfa@test.com",
		"password": "mfa-password",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["requires_2fa"] != true {
		t.Fatal("expected requires_2fa: true")
	}
	if resp["temp_token"] == nil || resp["temp_token"] == "" {
		t.Fatal("expected temp_token in response")
	}
	if resp["access_token"] != nil {
		t.Fatal("should NOT return access_token when 2FA is required")
	}
}

func TestPostGotrueToken_2FA_ValidTOTP(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP"
	q := newFakeAuthQuerier()
	q.accounts["mfa@test.com"] = database.Account{
		ID:           2,
		GotrueID:     "gid-2",
		Email:        "mfa@test.com",
		PasswordHash: hashPassword(t, "mfa-password"),
		TotpEnabled:  true,
		TotpSecret:   []byte(secret),
	}
	q.accountsByGID["gid-2"] = q.accounts["mfa@test.com"]

	api, r := setupTestApi(t, q)

	// Step 1: login to get temp_token
	w := postJSON(r, "/auth/token", map[string]string{
		"email":    "mfa@test.com",
		"password": "mfa-password",
	})

	var step1 map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &step1)
	tempToken, ok := step1["temp_token"].(string)
	if !ok || tempToken == "" {
		t.Fatal("expected temp_token")
	}

	// Step 2: complete 2FA with valid TOTP code
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("generate totp: %v", err)
	}

	_ = api // api used to set up router
	w2 := postJSON(r, "/auth/token", map[string]string{
		"temp_token": tempToken,
		"totp_code":  code,
	})

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var step2 map[string]interface{}
	json.Unmarshal(w2.Body.Bytes(), &step2)

	if step2["access_token"] == nil || step2["access_token"] == "" {
		t.Fatal("expected access_token after successful 2FA")
	}
}

func TestPostGotrueToken_2FA_InvalidTOTP(t *testing.T) {
	secret := "JBSWY3DPEHPK3PXP"
	q := newFakeAuthQuerier()
	q.accounts["mfa@test.com"] = database.Account{
		ID:           2,
		GotrueID:     "gid-2",
		Email:        "mfa@test.com",
		PasswordHash: hashPassword(t, "mfa-password"),
		TotpEnabled:  true,
		TotpSecret:   []byte(secret),
	}
	q.accountsByGID["gid-2"] = q.accounts["mfa@test.com"]

	_, r := setupTestApi(t, q)

	// Step 1: login to get temp token
	w := postJSON(r, "/auth/token", map[string]string{
		"email":    "mfa@test.com",
		"password": "mfa-password",
	})
	var step1 map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &step1)
	tempToken := step1["temp_token"].(string)

	// Step 2: send invalid TOTP code
	w2 := postJSON(r, "/auth/token", map[string]string{
		"temp_token": tempToken,
		"totp_code":  "000000",
	})

	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w2.Code, w2.Body.String())
	}
}
