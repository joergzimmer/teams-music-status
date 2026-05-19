package teams

import "time"

// ─── Request-Typen (Graph API JSON Bodies) ─────────────────

// StatusMessageRequest is the body for POST /users/{id}/presence/setStatusMessage
type StatusMessageRequest struct {
	StatusMessage StatusMessage `json:"statusMessage"`
}

type StatusMessage struct {
	Message        ItemBody        `json:"message"`
	ExpiryDateTime *ExpiryDateTime `json:"expiryDateTime,omitempty"`
}

type ItemBody struct {
	Content     string `json:"content"`
	ContentType string `json:"contentType"` // "text" or "html"
}

type ExpiryDateTime struct {
	DateTime string `json:"dateTime"`
	TimeZone string `json:"timeZone"`
}

// ─── Response-Typen ────────────────────────────────────────

// Presence is the response from GET /me/presence
type Presence struct {
	ID           string             `json:"id"`
	Availability string             `json:"availability"`
	Activity     string             `json:"activity"`
	StatusMsg    *PresenceStatusMsg `json:"statusMessage,omitempty"`
}

type PresenceStatusMsg struct {
	Message        *ItemBody       `json:"message,omitempty"`
	ExpiryDateTime *ExpiryDateTime `json:"expiryDateTime,omitempty"`
}

// UserProfile is the minimal response from GET /me
type UserProfile struct {
	ID                string `json:"id"`
	DisplayName       string `json:"displayName"`
	UserPrincipalName string `json:"userPrincipalName"`
}

// GraphError represents a Graph API error response.
type GraphError struct {
	Error GraphErrorBody `json:"error"`
}

type GraphErrorBody struct {
	Code       string         `json:"code"`
	Message    string         `json:"message"`
	InnerError *GraphInnerErr `json:"innerError,omitempty"`
}

type GraphInnerErr struct {
	Code      string `json:"code"`
	Date      string `json:"date"`
	RequestID string `json:"request-id"`
}

// ─── Status Update Parameters ─────────────────────────────

// StatusUpdate contains data for a high-level status update.
type StatusUpdate struct {
	Message       string
	ExpiryMinutes int
}

// NewStatusUpdate creates a StatusUpdate with default expiry.
func NewStatusUpdate(message string, expiryMinutes int) StatusUpdate {
	if expiryMinutes <= 0 {
		expiryMinutes = 10
	}
	return StatusUpdate{
		Message:       message,
		ExpiryMinutes: expiryMinutes,
	}
}

// ToRequest converts StatusUpdate to the Graph API request body.
func (su StatusUpdate) ToRequest(timeZone string) StatusMessageRequest {
	if timeZone == "" {
		timeZone = "W. Europe Standard Time"
	}

	expiry := time.Now().Add(time.Duration(su.ExpiryMinutes) * time.Minute)

	return StatusMessageRequest{
		StatusMessage: StatusMessage{
			Message: ItemBody{
				Content:     su.Message,
				ContentType: "text",
			},
			ExpiryDateTime: &ExpiryDateTime{
				DateTime: expiry.Format("2006-01-02T15:04:05"),
				TimeZone: timeZone,
			},
		},
	}
}
