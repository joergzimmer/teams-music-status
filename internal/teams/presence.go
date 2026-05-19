package teams

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// SetStatusMessage sets the authenticated user's status message.
// Example: "🎵 Bohemian Rhapsody – Queen"
func (gc *GraphClient) SetStatusMessage(ctx context.Context, update StatusUpdate) error {
	userID, err := gc.resolveUserID(ctx)
	if err != nil {
		return fmt.Errorf("user-ID auflösen: %w", err)
	}

	if update.ExpiryMinutes <= 0 {
		update.ExpiryMinutes = gc.cfg.ExpiryMinutes
	}

	body := update.ToRequest(gc.cfg.TimeZone)
	path := fmt.Sprintf("/users/%s/presence/setStatusMessage", userID)

	resp, err := gc.doRequest(ctx, http.MethodPost, path, body)
	if err != nil {
		return fmt.Errorf("setStatusMessage: %w", err)
	}
	defer resp.Body.Close()

	gc.logger.Info("Status-Nachricht gesetzt",
		"message", update.Message,
		"expiry_min", update.ExpiryMinutes,
	)

	return nil
}

// ClearStatusMessage clears the status message (sets empty content).
// Graph API hat keinen dedizierten "clearStatusMessage" Endpoint,
// so an empty message with short expiry is set.
func (gc *GraphClient) ClearStatusMessage(ctx context.Context) error {
	userID, err := gc.resolveUserID(ctx)
	if err != nil {
		return fmt.Errorf("user-ID auflösen: %w", err)
	}

	body := StatusMessageRequest{
		StatusMessage: StatusMessage{
			Message: ItemBody{
				Content:     "",
				ContentType: "text",
			},
		},
	}

	path := fmt.Sprintf("/users/%s/presence/setStatusMessage", userID)

	resp, err := gc.doRequest(ctx, http.MethodPost, path, body)
	if err != nil {
		return fmt.Errorf("clearStatusMessage: %w", err)
	}
	defer resp.Body.Close()

	gc.logger.Info("Status-Nachricht gelöscht")
	return nil
}

// GetPresence fetches the current presence status (availability + activity + status message).
func (gc *GraphClient) GetPresence(ctx context.Context) (*Presence, error) {
	resp, err := gc.doRequest(ctx, http.MethodGet, "/me/presence", nil)
	if err != nil {
		return nil, fmt.Errorf("getPresence: %w", err)
	}
	defer resp.Body.Close()

	var presence Presence
	if err := json.NewDecoder(resp.Body).Decode(&presence); err != nil {
		return nil, fmt.Errorf("getPresence JSON decode: %w", err)
	}

	gc.logger.Debug("Presence abgerufen",
		"availability", presence.Availability,
		"activity", presence.Activity,
	)

	return &presence, nil
}
