package channel

import (
	"fmt"
	"slices"
	"strings"
)

// Status is a channel lifecycle state. See docs/channel-lifecycle.md.
type Status string

// Lifecycle states.
const (
	StatusDraft                 Status = "draft"
	StatusBranding              Status = "branding"
	StatusAwaitingPlatformSetup Status = "awaiting_platform_setup"
	StatusReady                 Status = "ready"
	StatusActive                Status = "active"
	StatusPaused                Status = "paused"
	StatusError                 Status = "error"
	StatusArchived              Status = "archived"
)

// AllStatuses lists every lifecycle state in display order.
var AllStatuses = []Status{
	StatusDraft, StatusBranding, StatusAwaitingPlatformSetup, StatusReady,
	StatusActive, StatusPaused, StatusError, StatusArchived,
}

var transitions = map[Status][]Status{
	StatusDraft:                 {StatusBranding, StatusArchived},
	StatusBranding:              {StatusDraft, StatusAwaitingPlatformSetup, StatusError, StatusArchived},
	StatusAwaitingPlatformSetup: {StatusBranding, StatusReady, StatusError, StatusArchived},
	StatusReady:                 {StatusAwaitingPlatformSetup, StatusActive, StatusError, StatusArchived},
	StatusActive:                {StatusPaused, StatusError, StatusArchived},
	StatusPaused:                {StatusActive, StatusError, StatusArchived},
	StatusError:                 {StatusDraft, StatusBranding, StatusAwaitingPlatformSetup, StatusPaused, StatusArchived},
	StatusArchived:              {StatusDraft},
}

// ParseStatus converts a string into a known Status.
func ParseStatus(s string) (Status, error) {
	st := Status(strings.TrimSpace(s))
	if _, ok := transitions[st]; !ok {
		return "", &ValidationError{Field: "status", Message: fmt.Sprintf("unknown status %q", s)}
	}
	return st, nil
}

// AllowedTransitions returns the states reachable from s.
func AllowedTransitions(s Status) []Status {
	return slices.Clone(transitions[s])
}

// Prerequisites carries facts from outside the aggregate that some
// transitions depend on.
type Prerequisites struct {
	// VerifiedPlatformConnections is the number of external platform
	// accounts whose identity and publishing permission were confirmed by
	// the platform itself. Simulated or unverified connections never count.
	VerifiedPlatformConnections int
}

// StatusEvent records one lifecycle transition for auditing.
type StatusEvent struct {
	From   Status
	To     Status
	Reason string
}

// TransitionTo moves the channel to a new state if the move is allowed and
// its prerequisites are satisfied.
func (c *Channel) TransitionTo(to Status, reason string, pre Prerequisites) (StatusEvent, error) {
	reason = strings.TrimSpace(reason)
	if len(reason) > MaxReasonLen {
		return StatusEvent{}, &ValidationError{Field: "reason", Message: "must be at most 500 characters"}
	}
	if c.Status == to {
		return StatusEvent{}, &TransitionError{From: c.Status, To: to, Reason: "channel is already in this state"}
	}
	if !slices.Contains(transitions[c.Status], to) {
		return StatusEvent{}, &TransitionError{From: c.Status, To: to, Reason: "transition is not allowed"}
	}
	if err := c.checkPrerequisites(to, pre); err != nil {
		return StatusEvent{}, err
	}
	if to == StatusError && reason == "" {
		return StatusEvent{}, &ValidationError{Field: "reason", Message: "is required when moving to error"}
	}
	ev := StatusEvent{From: c.Status, To: to, Reason: reason}
	c.Status = to
	c.StatusReason = reason
	return ev, nil
}

func (c *Channel) checkPrerequisites(to Status, pre Prerequisites) error {
	fail := func(msg string) error { return &TransitionError{From: c.Status, To: to, Reason: msg} }
	switch to {
	case StatusBranding:
		if c.Niche == "" {
			return fail("a niche must be selected first")
		}
		if c.Strategy == nil {
			return fail("a channel strategy must be generated or written first")
		}
	case StatusAwaitingPlatformSetup:
		if c.Name == "" {
			return fail("a channel name must be selected first")
		}
		if c.Identity == nil {
			return fail("a channel identity must be generated first")
		}
	case StatusReady:
		if pre.VerifiedPlatformConnections < 1 {
			return fail("at least one platform connection must be verified by the platform")
		}
		if c.Blueprint == nil {
			return fail("a content blueprint must be generated first")
		}
	case StatusActive:
		if pre.VerifiedPlatformConnections < 1 {
			return fail("at least one verified platform connection is required")
		}
	}
	return nil
}
