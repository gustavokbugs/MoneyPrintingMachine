package launchpad

import (
	"encoding/json"
	"errors"
	"time"
)

// ErrSavedNicheNotFound is returned when a saved niche does not exist.
var ErrSavedNicheNotFound = errors.New("saved niche not found")

// ErrSavedNicheExists is returned when the owner already saved that name.
var ErrSavedNicheExists = errors.New("a saved niche with this name already exists")

// SavedNiche is a niche idea the user bookmarked.
type SavedNiche struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Notes     string          `json:"notes"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"createdAt"`
}
