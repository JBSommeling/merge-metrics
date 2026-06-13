package auth

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenSource provides GitHub API tokens.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// PATTokenSource uses a personal access token directly.
type PATTokenSource struct {
	token string
}

// NewPATTokenSource creates a PATTokenSource from the given PAT string.
func NewPATTokenSource(token string) *PATTokenSource {
	return &PATTokenSource{token: token}
}

// Token returns the personal access token.
func (p *PATTokenSource) Token(_ context.Context) (string, error) {
	return p.token, nil
}

// AppTokenSource uses a GitHub App JWT to obtain installation access tokens.
type AppTokenSource struct {
	AppID          int64
	InstallationID int64
	PrivateKey     []byte // PEM-encoded RSA private key
	BaseURL        string // default: "https://api.github.com"
}

// NewAppTokenSource creates an AppTokenSource. BaseURL defaults to
// "https://api.github.com".
func NewAppTokenSource(appID, installationID int64, privateKeyPEM []byte) *AppTokenSource {
	return &AppTokenSource{
		AppID:          appID,
		InstallationID: installationID,
		PrivateKey:     privateKeyPEM,
		BaseURL:        "https://api.github.com",
	}
}

// Token generates a GitHub App JWT, exchanges it for an installation access
// token, and returns that token.
func (a *AppTokenSource) Token(ctx context.Context) (string, error) {
	jwtToken, err := a.generateJWT()
	if err != nil {
		return "", fmt.Errorf("auth: generating JWT: %w", err)
	}

	baseURL := a.BaseURL
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}

	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", baseURL, a.InstallationID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(""))
	if err != nil {
		return "", fmt.Errorf("auth: creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("auth: exchanging JWT for installation token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("auth: unexpected status %d from GitHub API", resp.StatusCode)
	}

	var payload struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("auth: decoding installation token response: %w", err)
	}
	if payload.Token == "" {
		return "", fmt.Errorf("auth: installation token response contained empty token")
	}

	return payload.Token, nil
}

// generateJWT creates a signed RS256 JWT for GitHub App authentication.
// The token is valid for 10 minutes and uses the App ID as the issuer.
func (a *AppTokenSource) generateJWT() (string, error) {
	key, err := parseRSAPrivateKey(a.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("auth: parsing private key: %w", err)
	}

	now := time.Now()
	claims := jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(10 * time.Minute)),
		Issuer:    fmt.Sprintf("%d", a.AppID),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := token.SignedString(key)
	if err != nil {
		return "", fmt.Errorf("auth: signing JWT: %w", err)
	}
	return signed, nil
}

// parseRSAPrivateKey decodes a PEM-encoded RSA private key.
func parseRSAPrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}

	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("PKCS8 key is not RSA")
		}
		return rsaKey, nil
	default:
		return nil, fmt.Errorf("unsupported PEM block type: %s", block.Type)
	}
}
