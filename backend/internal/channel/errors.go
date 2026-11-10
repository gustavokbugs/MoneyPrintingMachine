package channel

import (
	"errors"
	"fmt"
)

// ErrNotFound is returned when a channel does not exist or is not owned by
// the requesting user. The two cases are intentionally indistinguishable.
var ErrNotFound = errors.New("channel not found")

// ErrConflict is returned when a concurrent update changed the channel first.
var ErrConflict = errors.New("channel was modified concurrently")

// ValidationError reports an invalid field value.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid %s: %s", e.Field, e.Message)
}

// TransitionError reports a lifecycle transition that is not allowed.
type TransitionError struct {
	From   Status
	To     Status
	Reason string
}

func (e *TransitionError) Error() string {
	return fmt.Sprintf("cannot change status from %s to %s: %s", e.From, e.To, e.Reason)
}
