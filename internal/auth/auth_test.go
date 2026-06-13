package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// generateTestRSAKey creates a 2048-bit RSA key and returns the private key
// along with its PEM encoding.
func generateTestRSAKey(t *testing.T) (*rsa.PrivateKey, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating RSA key: %v", err)
	}
	der := x509.MarshalPKCS1PrivateKey(key)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der})
	return key, pemBytes
}

// --- PAT token source ---

func TestPATTokenSource_ReturnsToken(t *testing.T) {
	const want = "ghp_testtoken123"
	src := NewPATTokenSource(want)
	got, err := src.Token(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- JWT generation ---

func TestAppTokenSource_GenerateJWT_ValidClaims(t *testing.T) {
	const appID int64 = 12345
	key, pemBytes := generateTestRSAKey(t)

	src := &AppTokenSource{
		AppID:          appID,
		InstallationID: 99,
		PrivateKey:     pemBytes,
		BaseURL:        "https://api.github.com",
	}

	before := time.Now().Add(-time.Second)
	tokenStr, err := src.generateJWT()
	after := time.Now().Add(time.Second)
	if err != nil {
		t.Fatalf("generateJWT returned error: %v", err)
	}

	// Parse and verify the JWT using the public key.
	parsed, err := jwt.ParseWithClaims(tokenStr, &jwt.RegisteredClaims{}, func(tok *jwt.Token) (interface{}, error) {
		if _, ok := tok.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", tok.Header["alg"])
		}
		return &key.PublicKey, nil
	})
	if err != nil {
		t.Fatalf("parsing JWT: %v", err)
	}
	if !parsed.Valid {
		t.Fatal("JWT is not valid")
	}

	claims, ok := parsed.Claims.(*jwt.RegisteredClaims)
	if !ok {
		t.Fatal("unexpected claims type")
	}

	// Issuer must be the app ID as a string.
	wantIssuer := fmt.Sprintf("%d", appID)
	if claims.Issuer != wantIssuer {
		t.Errorf("issuer: got %q, want %q", claims.Issuer, wantIssuer)
	}

	// IssuedAt must be between before and after.
	if claims.IssuedAt == nil {
		t.Fatal("iat claim is missing")
	}
	iat := claims.IssuedAt.Time
	if iat.Before(before) || iat.After(after) {
		t.Errorf("iat %v not in expected range [%v, %v]", iat, before, after)
	}

	// ExpiresAt must be approximately 10 minutes after iat.
	if claims.ExpiresAt == nil {
		t.Fatal("exp claim is missing")
	}
	exp := claims.ExpiresAt.Time
	diff := exp.Sub(iat)
	if diff < 9*time.Minute+59*time.Second || diff > 10*time.Minute+time.Second {
		t.Errorf("expiry duration %v not close to 10 minutes", diff)
	}
}

// --- App token source exchange ---

func TestAppTokenSource_Token_ExchangesJWTForInstallationToken(t *testing.T) {
	const wantInstallationToken = "ghs_installation_token_abc"
	const installationID int64 = 42

	_, pemBytes := generateTestRSAKey(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := fmt.Sprintf("/app/installations/%d/access_tokens", installationID)
		if r.Method != http.MethodPost {
			t.Errorf("method: got %s, want POST", r.Method)
		}
		if r.URL.Path != wantPath {
			t.Errorf("path: got %s, want %s", r.URL.Path, wantPath)
		}
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) < 8 || authHeader[:7] != "Bearer " {
			t.Errorf("Authorization header missing or malformed: %q", authHeader)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": wantInstallationToken})
	}))
	defer srv.Close()

	src := &AppTokenSource{
		AppID:          1,
		InstallationID: installationID,
		PrivateKey:     pemBytes,
		BaseURL:        srv.URL,
	}

	got, err := src.Token(context.Background())
	if err != nil {
		t.Fatalf("Token returned error: %v", err)
	}
	if got != wantInstallationToken {
		t.Errorf("got %q, want %q", got, wantInstallationToken)
	}
}

func TestAppTokenSource_Token_HandlesHTTPError(t *testing.T) {
	_, pemBytes := generateTestRSAKey(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	src := &AppTokenSource{
		AppID:          1,
		InstallationID: 10,
		PrivateKey:     pemBytes,
		BaseURL:        srv.URL,
	}

	_, err := src.Token(context.Background())
	if err == nil {
		t.Fatal("expected error for non-201 response, got nil")
	}
}

func TestAppTokenSource_Token_HandlesNetworkError(t *testing.T) {
	_, pemBytes := generateTestRSAKey(t)

	// Use a server that is immediately closed so the request fails.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	src := &AppTokenSource{
		AppID:          1,
		InstallationID: 10,
		PrivateKey:     pemBytes,
		BaseURL:        srv.URL,
	}

	_, err := src.Token(context.Background())
	if err == nil {
		t.Fatal("expected error for network failure, got nil")
	}
}
