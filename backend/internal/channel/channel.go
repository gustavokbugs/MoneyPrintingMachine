// Package channel contains the channel domain model, lifecycle rules, and
// channel use cases. A channel is an internal brand workspace; it is never
// coupled to a single external platform account.
package channel

import (
	"regexp"
	"strings"
	"time"
)

// Channel is the internal brand workspace and the root aggregate for
// strategy, identity, and content planning.
type Channel struct {
	ID           string
	OwnerID      string
	Name         string
	Status       Status
	StatusReason string
	Niche        string
	Language     string // BCP 47 tag, e.g. "en-US".
	GeoTarget    string // ISO 3166-1 alpha-2 country code, e.g. "US".
	Strategy     *Strategy
	Identity     *Identity
	Blueprint    *Blueprint
	Version      int // Optimistic concurrency token.
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Default localization values for new channels.
const (
	DefaultLanguage  = "en-US"
	DefaultGeoTarget = "US"
)

// Field length limits enforced at the domain boundary.
const (
	MaxNameLength  = 100
	MaxNicheLength = 120
	MaxReasonLen   = 500
)

var (
	languageTagRe = regexp.MustCompile(`^[a-z]{2,3}(-[A-Z]{2})?$`)
	countryCodeRe = regexp.MustCompile(`^[A-Z]{2}$`)
)

// NewDraft builds a new channel in the Draft state with validated settings.
func NewDraft(ownerID, niche, language, geo string) (*Channel, error) {
	c := &Channel{
		OwnerID:   ownerID,
		Status:    StatusDraft,
		Niche:     strings.TrimSpace(niche),
		Language:  strings.TrimSpace(language),
		GeoTarget: strings.ToUpper(strings.TrimSpace(geo)),
	}
	if c.Language == "" {
		c.Language = DefaultLanguage
	}
	if c.GeoTarget == "" {
		c.GeoTarget = DefaultGeoTarget
	}
	if err := c.validateSettings(); err != nil {
		return nil, err
	}
	return c, nil
}

// SettingsUpdate holds optional edits to basic channel settings.
type SettingsUpdate struct {
	Name      *string
	Niche     *string
	Language  *string
	GeoTarget *string
}

// ApplySettings applies and validates a settings update. Archived channels
// are read-only.
func (c *Channel) ApplySettings(u SettingsUpdate) error {
	if c.Status == StatusArchived {
		return &ValidationError{Field: "status", Message: "archived channels cannot be edited"}
	}
	next := *c
	if u.Name != nil {
		next.Name = strings.TrimSpace(*u.Name)
	}
	if u.Niche != nil {
		next.Niche = strings.TrimSpace(*u.Niche)
	}
	if u.Language != nil {
		next.Language = strings.TrimSpace(*u.Language)
	}
	if u.GeoTarget != nil {
		next.GeoTarget = strings.ToUpper(strings.TrimSpace(*u.GeoTarget))
	}
	if err := next.validateSettings(); err != nil {
		return err
	}
	*c = next
	return nil
}

func (c *Channel) validateSettings() error {
	if c.OwnerID == "" {
		return &ValidationError{Field: "ownerId", Message: "is required"}
	}
	if len(c.Name) > MaxNameLength {
		return &ValidationError{Field: "name", Message: "must be at most 100 characters"}
	}
	if len(c.Niche) > MaxNicheLength {
		return &ValidationError{Field: "niche", Message: "must be at most 120 characters"}
	}
	if !languageTagRe.MatchString(c.Language) {
		return &ValidationError{Field: "language", Message: "must be a language tag such as en-US"}
	}
	if !countryCodeRe.MatchString(c.GeoTarget) {
		return &ValidationError{Field: "geoTarget", Message: "must be a two-letter country code such as US"}
	}
	return nil
}

// SelectName chooses the channel name, typically from identity suggestions.
func (c *Channel) SelectName(name string) error {
	return c.ApplySettings(SettingsUpdate{Name: &name})
}
