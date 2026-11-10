package platforms

import "time"

// Connection is the read model of one external platform account linked to
// a channel. Status reaches "verified" only after the platform API
// confirmed the account and its permissions.
type Connection struct {
	Platform         string    `json:"platform"`
	Status           string    `json:"status"`
	ExternalID       string    `json:"externalId,omitempty"`
	ExternalName     string    `json:"externalName,omitempty"`
	CanPublish       bool      `json:"canPublish"`
	CanReadAnalytics bool      `json:"canReadAnalytics"`
	StatusReason     string    `json:"statusReason,omitempty"`
	UpdatedAt        time.Time `json:"updatedAt"`
}
