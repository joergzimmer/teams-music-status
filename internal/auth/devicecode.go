package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// deviceCodeResponse contains the response to the device code request.
type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
	Message         string `json:"message"`
}

// tokenResponse contains the response to the token request.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

// Token represents an OAuth2 token with expiry time.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scope        string    `json:"scope"`
}

// IsValid checks whether the token is still valid for at least 2 minutes.
func (t *Token) IsValid() bool {
	if t == nil || t.AccessToken == "" {
		return false
	}
	return time.Now().Add(2 * time.Minute).Before(t.ExpiresAt)
}

// deviceCodeFlow performs the full device code flow.
func (tm *TokenManager) deviceCodeFlow(ctx context.Context) (*Token, error) {
	// 1. Request device code
	dcResp, err := tm.requestDeviceCode(ctx)
	if err != nil {
		return nil, fmt.Errorf("device code request: %w", err)
	}

	// 2. Show user instructions
	tm.logger.Info("Device Code Flow gestartet",
		"url", dcResp.VerificationURI,
		"code", dcResp.UserCode,
	)
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════╗")
	fmt.Println("║  Authentifizierung erforderlich                         ║")
	fmt.Println("╠══════════════════════════════════════════════════════════╣")
	fmt.Printf("║  1. Öffne:  %-44s ║\n", dcResp.VerificationURI)
	fmt.Printf("║  2. Code:   %-44s ║\n", dcResp.UserCode)
	fmt.Println("║  3. Melde dich mit deinem Microsoft-Konto an            ║")
	fmt.Println("╚══════════════════════════════════════════════════════════╝")
	fmt.Println()

	// 3. Poll for token
	interval := time.Duration(dcResp.Interval) * time.Second
	if interval < 5*time.Second {
		interval = 5 * time.Second // Minimum according to spec
	}
	deadline := time.Now().Add(time.Duration(dcResp.ExpiresIn) * time.Second)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("device code flow timeout: code abgelaufen")
			}

			token, err := tm.pollForToken(ctx, dcResp.DeviceCode)
			if err != nil {
				return nil, err
			}
			if token != nil {
				tm.logger.Info("Authentifizierung erfolgreich")
				return token, nil
			}
			// token == nil -> authorization_pending -> continue polling
		}
	}
}

// requestDeviceCode requests a device code.
func (tm *TokenManager) requestDeviceCode(ctx context.Context) (*deviceCodeResponse, error) {
	endpoint := fmt.Sprintf(
		"https://login.microsoftonline.com/%s/oauth2/v2.0/devicecode",
		tm.cfg.TenantID,
	)

	data := url.Values{
		"client_id": {tm.cfg.ClientID},
		"scope":     {tm.scopeString()},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := tm.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request fehlgeschlagen: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device code request fehlgeschlagen (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var dcResp deviceCodeResponse
	if err := json.Unmarshal(body, &dcResp); err != nil {
		return nil, fmt.Errorf("JSON decode: %w", err)
	}

	return &dcResp, nil
}

// pollForToken polls the token endpoint.
// Returns (nil, nil) on "authorization_pending" (continue polling).
func (tm *TokenManager) pollForToken(ctx context.Context, deviceCode string) (*Token, error) {
	endpoint := fmt.Sprintf(
		"https://login.microsoftonline.com/%s/oauth2/v2.0/token",
		tm.cfg.TenantID,
	)

	data := url.Values{
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		"client_id":   {tm.cfg.ClientID},
		"device_code": {deviceCode},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := tm.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request fehlgeschlagen: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var tokenResp tokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("JSON decode: %w", err)
	}

	// Pending -> continue polling
	if tokenResp.Error == "authorization_pending" {
		return nil, nil
	}

	// Slow down -> increase interval (not handled in caller, but safe)
	if tokenResp.Error == "slow_down" {
		tm.logger.Debug("Token-Endpoint: slow_down, warte länger...")
		return nil, nil
	}

	// Actual error
	if tokenResp.Error != "" {
		return nil, fmt.Errorf("token error: %s – %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	// Success
	return &Token{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		Scope:        tokenResp.Scope,
	}, nil
}

// refreshToken renews an access token via refresh token.
func (tm *TokenManager) refreshToken(ctx context.Context, refreshToken string) (*Token, error) {
	endpoint := fmt.Sprintf(
		"https://login.microsoftonline.com/%s/oauth2/v2.0/token",
		tm.cfg.TenantID,
	)

	data := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {tm.cfg.ClientID},
		"refresh_token": {refreshToken},
		"scope":         {tm.scopeString()},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := tm.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request fehlgeschlagen: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var tokenResp tokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("JSON decode: %w", err)
	}

	if tokenResp.Error != "" {
		return nil, fmt.Errorf("refresh error: %s – %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	// Apply new refresh token (if rotated)
	newRefresh := tokenResp.RefreshToken
	if newRefresh == "" {
		newRefresh = refreshToken // keep old one if none returned
	}

	return &Token{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: newRefresh,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		Scope:        tokenResp.Scope,
	}, nil
}
