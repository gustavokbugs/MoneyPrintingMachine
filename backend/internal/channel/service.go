package channel

import (
	"context"
	"errors"
	"fmt"
)

// Page requests a bounded slice of a collection using keyset pagination.
type Page struct {
	Limit  int
	Cursor string // Opaque; empty for the first page.
}

// Normalize clamps the limit to a safe range.
func (p Page) Normalize() Page {
	if p.Limit <= 0 {
		p.Limit = 25
	}
	if p.Limit > 100 {
		p.Limit = 100
	}
	return p
}

// Repository persists channels. Every method is scoped to an owner.
type Repository interface {
	Create(ctx context.Context, c *Channel) error
	Get(ctx context.Context, ownerID, id string) (*Channel, error)
	List(ctx context.Context, ownerID string, page Page) (items []Channel, nextCursor string, err error)
	// Save writes all mutable fields if c.Version still matches the stored
	// version, then increments c.Version. When ev is non-nil the status
	// event is recorded in the same transaction.
	Save(ctx context.Context, c *Channel, ev *StatusEvent) error
}

// ConnectionCounter reports verified external platform connections.
type ConnectionCounter interface {
	CountVerified(ctx context.Context, ownerID, channelID string) (int, error)
}

// Service implements channel use cases.
type Service struct {
	repo        Repository
	connections ConnectionCounter
}

// NewService builds a channel Service.
func NewService(repo Repository, connections ConnectionCounter) *Service {
	return &Service{repo: repo, connections: connections}
}

// CreateDraft creates a new Draft channel.
func (s *Service) CreateDraft(ctx context.Context, ownerID, niche, language, geo string) (*Channel, error) {
	c, err := NewDraft(ownerID, niche, language, geo)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, c); err != nil {
		return nil, fmt.Errorf("create channel: %w", err)
	}
	return c, nil
}

// Get returns one channel owned by ownerID.
func (s *Service) Get(ctx context.Context, ownerID, id string) (*Channel, error) {
	return s.repo.Get(ctx, ownerID, id)
}

// List returns a page of channels owned by ownerID.
func (s *Service) List(ctx context.Context, ownerID string, page Page) ([]Channel, string, error) {
	return s.repo.List(ctx, ownerID, page.Normalize())
}

// Mutate loads a channel, applies fn, and saves it, retrying a bounded
// number of times on optimistic-concurrency conflicts.
func (s *Service) Mutate(ctx context.Context, ownerID, id string, fn func(*Channel) (*StatusEvent, error)) (*Channel, error) {
	const maxAttempts = 3
	var lastErr error
	for range maxAttempts {
		c, err := s.repo.Get(ctx, ownerID, id)
		if err != nil {
			return nil, err
		}
		ev, err := fn(c)
		if err != nil {
			return nil, err
		}
		err = s.repo.Save(ctx, c, ev)
		if err == nil {
			return c, nil
		}
		if !errors.Is(err, ErrConflict) {
			return nil, fmt.Errorf("save channel: %w", err)
		}
		lastErr = err
	}
	return nil, lastErr
}

// UpdateSettings edits name, niche, language, or geographic target.
func (s *Service) UpdateSettings(ctx context.Context, ownerID, id string, u SettingsUpdate) (*Channel, error) {
	return s.Mutate(ctx, ownerID, id, func(c *Channel) (*StatusEvent, error) {
		return nil, c.ApplySettings(u)
	})
}

// ChangeStatus performs a lifecycle transition.
func (s *Service) ChangeStatus(ctx context.Context, ownerID, id string, to Status, reason string) (*Channel, error) {
	verified, err := s.connections.CountVerified(ctx, ownerID, id)
	if err != nil {
		return nil, fmt.Errorf("count verified connections: %w", err)
	}
	return s.Mutate(ctx, ownerID, id, func(c *Channel) (*StatusEvent, error) {
		ev, err := c.TransitionTo(to, reason, Prerequisites{VerifiedPlatformConnections: verified})
		if err != nil {
			return nil, err
		}
		return &ev, nil
	})
}

// BulkResult is the outcome of one item in a bulk operation.
type BulkResult struct {
	ID     string
	Status Status
	Err    error
}

// MaxBulkItems bounds bulk operations.
const MaxBulkItems = 100

// BulkChangeStatus applies a transition to several channels independently.
// Only reversible operational states are allowed in bulk.
func (s *Service) BulkChangeStatus(ctx context.Context, ownerID string, ids []string, to Status, reason string) ([]BulkResult, error) {
	if to != StatusPaused && to != StatusActive {
		return nil, &ValidationError{Field: "status", Message: "bulk operations only support paused and active"}
	}
	if len(ids) == 0 || len(ids) > MaxBulkItems {
		return nil, &ValidationError{Field: "ids", Message: "must contain between 1 and 100 channel IDs"}
	}
	results := make([]BulkResult, 0, len(ids))
	for _, id := range ids {
		c, err := s.ChangeStatus(ctx, ownerID, id, to, reason)
		r := BulkResult{ID: id, Err: err}
		if c != nil {
			r.Status = c.Status
		}
		results = append(results, r)
	}
	return results, nil
}

// SetStrategy stores a generated or edited strategy.
func (s *Service) SetStrategy(ctx context.Context, ownerID, id string, st Strategy) (*Channel, error) {
	if err := st.Validate(); err != nil {
		return nil, err
	}
	return s.Mutate(ctx, ownerID, id, func(c *Channel) (*StatusEvent, error) {
		if c.Status == StatusArchived {
			return nil, &ValidationError{Field: "status", Message: "archived channels cannot be edited"}
		}
		c.Strategy = &st
		return nil, nil
	})
}

// SetIdentity stores a generated or edited identity.
func (s *Service) SetIdentity(ctx context.Context, ownerID, id string, idn Identity) (*Channel, error) {
	if err := idn.Validate(); err != nil {
		return nil, err
	}
	return s.Mutate(ctx, ownerID, id, func(c *Channel) (*StatusEvent, error) {
		if c.Status == StatusArchived {
			return nil, &ValidationError{Field: "status", Message: "archived channels cannot be edited"}
		}
		c.Identity = &idn
		return nil, nil
	})
}

// SetBlueprint stores a generated or edited blueprint.
func (s *Service) SetBlueprint(ctx context.Context, ownerID, id string, bp Blueprint) (*Channel, error) {
	if err := bp.Validate(); err != nil {
		return nil, err
	}
	return s.Mutate(ctx, ownerID, id, func(c *Channel) (*StatusEvent, error) {
		if c.Status == StatusArchived {
			return nil, &ValidationError{Field: "status", Message: "archived channels cannot be edited"}
		}
		c.Blueprint = &bp
		return nil, nil
	})
}

// EditIdentity stores a user-edited identity. Name check results cannot be
// set by the client: results are kept only for names that were already
// checked server-side, and every other name is marked not checked.
func (s *Service) EditIdentity(ctx context.Context, ownerID, id string, idn Identity) (*Channel, error) {
	idn.Provenance.EditedByUser = true
	return s.Mutate(ctx, ownerID, id, func(c *Channel) (*StatusEvent, error) {
		if c.Status == StatusArchived {
			return nil, &ValidationError{Field: "status", Message: "archived channels cannot be edited"}
		}
		next := idn
		next.NameSuggestions = carryNameChecks(c.Identity, idn.NameSuggestions)
		if err := next.Validate(); err != nil {
			return nil, err
		}
		c.Identity = &next
		return nil, nil
	})
}

func carryNameChecks(prev *Identity, names []NameSuggestion) []NameSuggestion {
	checked := map[string]NameSuggestion{}
	if prev != nil {
		for _, n := range prev.NameSuggestions {
			checked[n.Name] = n
		}
	}
	out := make([]NameSuggestion, len(names))
	for i, n := range names {
		out[i] = NameSuggestion{Name: n.Name, Rationale: n.Rationale, Availability: AvailabilityNotChecked}
		if p, ok := checked[n.Name]; ok {
			out[i].Availability, out[i].Handle, out[i].CheckedAt = p.Availability, p.Handle, p.CheckedAt
		}
	}
	return out
}
