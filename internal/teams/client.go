package teams

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	graphBaseURL  = "https://graph.microsoft.com/v1.0"
	maxRetries    = 3
	defaultExpiry = 10 // minutes
)

// TokenProvider is the interface to the auth module.
type TokenProvider interface {
	GetAccessToken(ctx context.Context) (string, error)
	InvalidateToken()
}

// ClientConfig contains configuration for GraphClient.
type ClientConfig struct {
	TimeZone      string // e.g. "W. Europe Standard Time"
	ExpiryMinutes int    // default expiry for status messages
}

// GraphClient is the HTTP client for the Microsoft Graph API.
type GraphClient struct {
	auth     TokenProvider
	client   *http.Client
	cfg      ClientConfig
	logger   *slog.Logger
	userID   string
	userOnce sync.Once
}

// NewGraphClient creates a new GraphClient.
func NewGraphClient(auth TokenProvider, cfg ClientConfig, logger *slog.Logger) *GraphClient {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.TimeZone == "" {
		cfg.TimeZone = "W. Europe Standard Time"
	}
	if cfg.ExpiryMinutes <= 0 {
		cfg.ExpiryMinutes = defaultExpiry
	}

	return &GraphClient{
		auth:   auth,
		client: &http.Client{Timeout: 30 * time.Second},
		cfg:    cfg,
		logger: logger,
	}
}

// resolveUserID retrieves user ID once via GET /me.
// Required because setStatusMessage only works via /users/{id}/...
func (gc *GraphClient) resolveUserID(ctx context.Context) (string, error) {
	var resolveErr error
	gc.userOnce.Do(func() {
		gc.logger.Debug("Lade User-ID via GET /me...")

		resp, err := gc.doRequest(ctx, http.MethodGet, "/me?$select=id,displayName,userPrincipalName", nil)
		if err != nil {
			resolveErr = fmt.Errorf("GET /me: %w", err)
			// Reset Once so it can be retried next time
			gc.userOnce = sync.Once{}
			return
		}
		defer resp.Body.Close()

		var profile UserProfile
		if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
			resolveErr = fmt.Errorf("GET /me JSON decode: %w", err)
			gc.userOnce = sync.Once{}
			return
		}

		gc.userID = profile.ID
		gc.logger.Info("User-ID aufgelöst",
			"id", profile.ID,
			"name", profile.DisplayName,
			"upn", profile.UserPrincipalName,
		)
	})

	if resolveErr != nil {
		return "", resolveErr
	}
	if gc.userID == "" {
		return "", fmt.Errorf("user-ID konnte nicht aufgelöst werden")
	}
	return gc.userID, nil
}

// doRequest performs an authenticated HTTP request against Graph API.
// Includes automatic retries for 429 (throttling) and 401 (expired token).
func (gc *GraphClient) doRequest(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var bodyReader io.Reader

	if body != nil {
		jsonBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("JSON marshal: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBytes)
		gc.logger.Debug("Request Body", "json", string(jsonBytes))
	}

	url := graphBaseURL + path

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Recreate body on retry (reader is consumed after first read)
		if attempt > 0 && body != nil {
			jsonBytes, _ := json.Marshal(body)
			bodyReader = bytes.NewReader(jsonBytes)
		}

		token, err := gc.auth.GetAccessToken(ctx)
		if err != nil {
			return nil, fmt.Errorf("access token: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
		if err != nil {
			return nil, fmt.Errorf("request erstellen: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := gc.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("HTTP %s %s: %w", method, path, err)
		}

		gc.logger.Debug("Graph API Response",
			"method", method,
			"path", path,
			"status", resp.StatusCode,
			"attempt", attempt+1,
		)

		switch {
		// Success
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			return resp, nil

		// Throttling -> respect Retry-After
		case resp.StatusCode == http.StatusTooManyRequests:
			resp.Body.Close()
			retryAfter := gc.parseRetryAfter(resp)
			gc.logger.Warn("Graph API Throttling (429)",
				"retry_after_sec", retryAfter.Seconds(),
				"attempt", attempt+1,
			)
			if attempt < maxRetries {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(retryAfter):
					continue
				}
			}
			return nil, fmt.Errorf("throttling nach %d Versuchen: %s %s", maxRetries+1, method, path)

		// Unauthorized -> invalidate token and fetch a new one
		case resp.StatusCode == http.StatusUnauthorized:
			resp.Body.Close()
			gc.logger.Warn("Graph API 401 – Token wird invalidiert", "attempt", attempt+1)
			gc.auth.InvalidateToken()
			if attempt < maxRetries {
				continue
			}
			return nil, fmt.Errorf("unauthorized nach %d Versuchen: %s %s", maxRetries+1, method, path)

		// Other error
		default:
			defer resp.Body.Close()
			errBody, _ := io.ReadAll(resp.Body)

			var graphErr GraphError
			if json.Unmarshal(errBody, &graphErr) == nil && graphErr.Error.Code != "" {
				return nil, fmt.Errorf("graph API fehler (HTTP %d): [%s] %s",
					resp.StatusCode, graphErr.Error.Code, graphErr.Error.Message)
			}
			return nil, fmt.Errorf("graph API fehler (HTTP %d): %s", resp.StatusCode, string(errBody))
		}
	}

	return nil, fmt.Errorf("max retries erreicht: %s %s", method, path)
}

// parseRetryAfter reads the Retry-After header.
// Fallback: Exponentielles Backoff (5s, 10s, 20s).
func (gc *GraphClient) parseRetryAfter(resp *http.Response) time.Duration {
	if val := resp.Header.Get("Retry-After"); val != "" {
		if secs, err := strconv.Atoi(val); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	// Fallback: 5 seconds (Graph recommends at least the given wait time)
	return 5 * time.Second
}
