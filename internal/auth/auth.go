package auth

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// AzureConfig contains the Entra ID app registration data.
type AzureConfig struct {
	TenantID string   `yaml:"tenant_id"`
	ClientID string   `yaml:"client_id"`
	Scopes   []string `yaml:"scopes"` // e.g. ["Presence.ReadWrite", "offline_access"]
}

// TokenManager manages OAuth2 tokens (fetch, refresh, persistence).
type TokenManager struct {
	cfg    AzureConfig
	store  *TokenStore
	token  *Token
	mu     sync.RWMutex
	logger *slog.Logger
	client *http.Client
}

// NewTokenManager creates a new TokenManager.
func NewTokenManager(cfg AzureConfig, tokenPath string, logger *slog.Logger) *TokenManager {
	if logger == nil {
		logger = slog.Default()
	}

	// Scopes: always include offline_access (for refresh tokens)
	hasOffline := false
	for _, s := range cfg.Scopes {
		if s == "offline_access" {
			hasOffline = true
			break
		}
	}
	if !hasOffline {
		cfg.Scopes = append(cfg.Scopes, "offline_access")
	}

	return &TokenManager{
		cfg:    cfg,
		store:  NewTokenStore(tokenPath),
		logger: logger,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// oidcScopes are standard OIDC scopes that must not be prefixed with the Graph base URL.
var oidcScopes = map[string]bool{
	"offline_access": true,
	"openid":         true,
	"profile":        true,
	"email":          true,
}

// scopeString returns scopes as a space-separated string for token requests.
// OIDC scopes (offline_access, openid, …) are sent as-is; all others are
// prefixed with the Microsoft Graph base URL.
func (tm *TokenManager) scopeString() string {
	s := ""
	for i, sc := range tm.cfg.Scopes {
		if i > 0 {
			s += " "
		}
		if oidcScopes[sc] || strings.HasPrefix(sc, "https://") {
			s += sc
		} else {
			s += "https://graph.microsoft.com/" + sc
		}
	}
	return s
}

// GetAccessToken returns a valid access token.
// - Loads a stored token from file (if present)
// - Automatically refreshes when expired
// - Starts device code flow if no token is present
func (tm *TokenManager) GetAccessToken(ctx context.Context) (string, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// 1. Token from memory
	if tm.token != nil && tm.token.IsValid() {
		return tm.token.AccessToken, nil
	}

	// 2. Load token from file
	stored, err := tm.store.Load()
	if err == nil && stored != nil {
		tm.logger.Info("Token aus Datei geladen")

		// Still valid?
		if stored.IsValid() {
			tm.token = stored
			return stored.AccessToken, nil
		}

		// Expired but refresh token available?
		if stored.RefreshToken != "" {
			tm.logger.Info("Access Token abgelaufen, versuche Refresh...")
			refreshed, err := tm.refreshToken(ctx, stored.RefreshToken)
			if err == nil {
				tm.token = refreshed
				if saveErr := tm.store.Save(refreshed); saveErr != nil {
					tm.logger.Warn("Token konnte nicht gespeichert werden", "error", saveErr)
				}
				return refreshed.AccessToken, nil
			}
			tm.logger.Warn("Token Refresh fehlgeschlagen, starte Device Code Flow", "error", err)
		}
	}

	// 3. No valid token -> device code flow
	tm.logger.Info("Kein gültiges Token vorhanden, starte Device Code Flow...")
	newToken, err := tm.deviceCodeFlow(ctx)
	if err != nil {
		return "", fmt.Errorf("device code flow fehlgeschlagen: %w", err)
	}

	tm.token = newToken
	if saveErr := tm.store.Save(newToken); saveErr != nil {
		tm.logger.Warn("Token konnte nicht gespeichert werden", "error", saveErr)
	}

	return newToken.AccessToken, nil
}

// InvalidateToken clears the current token (e.g. after a 401 response).
func (tm *TokenManager) InvalidateToken() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.token = nil
	tm.logger.Info("Token invalidiert")
}
