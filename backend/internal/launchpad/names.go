package launchpad

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"contentfactory/internal/channel"
	"contentfactory/internal/youtube"
)

// HandleLookup finds a YouTube channel by handle.
type HandleLookup interface {
	HasAPIKey() bool
	ChannelByHandle(ctx context.Context, handle string) (*youtube.ChannelInfo, error)
}

// WithHandleLookup enables YouTube handle checks for name suggestions.
func (e *Engine) WithHandleLookup(h HandleLookup) *Engine {
	e.handles = h
	return e
}

// HandleChecksAvailable reports whether name checks can run.
func (e *Engine) HandleChecksAvailable() bool { return e.handles != nil && e.handles.HasAPIKey() }

// ErrHandleChecksUnavailable means no YouTube API key is configured.
var ErrHandleChecksUnavailable = fmt.Errorf("%w: set YOUTUBE_API_KEY to check channel handles", ErrPrecondition)

// HandleFor derives a YouTube handle candidate from a name: letters and
// digits are kept, everything else removed. YouTube handles are 3 to 30
// characters; an empty string means no valid handle can be derived.
func HandleFor(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	h := b.String()
	if len(h) < 3 || len(h) > 30 {
		return ""
	}
	return h
}

// CheckNames looks up the derived handle of every name suggestion on
// YouTube (1 quota unit each) and stores the results.
func (e *Engine) CheckNames(ctx context.Context, store ChannelStore, ownerID, channelID string) (*channel.Channel, error) {
	if !e.HandleChecksAvailable() {
		return nil, ErrHandleChecksUnavailable
	}
	ch, err := store.Get(ctx, ownerID, channelID)
	if err != nil {
		return nil, err
	}
	if ch.Identity == nil {
		return nil, fmt.Errorf("%w: generate a channel identity first", ErrPrecondition)
	}
	idn := *ch.Identity
	names := append([]channel.NameSuggestion(nil), idn.NameSuggestions...)
	var stopErr error
	for i := range names {
		handle := HandleFor(names[i].Name)
		now := e.now().UTC()
		names[i].Handle = handle
		if handle == "" {
			names[i].Availability = channel.AvailabilityInvalidHandle
			names[i].CheckedAt = &now
			continue
		}
		found, err := e.handles.ChannelByHandle(ctx, handle)
		if err != nil {
			if errors.Is(err, youtube.ErrQuotaBudget) || errors.Is(err, youtube.ErrQuotaExceeded) || errors.Is(err, youtube.ErrUnauthorized) {
				stopErr = err
				break
			}
			// Leave this name unchecked; others may still succeed.
			continue
		}
		names[i].CheckedAt = &now
		if found != nil {
			names[i].Availability = channel.AvailabilityHandleTaken
		} else {
			names[i].Availability = channel.AvailabilityNoChannel
		}
	}
	idn.NameSuggestions = names
	updated, err := store.SetIdentity(ctx, ownerID, channelID, idn)
	if err != nil {
		return nil, err
	}
	if stopErr != nil {
		return updated, fmt.Errorf("some names were not checked: %w", stopErr)
	}
	return updated, nil
}
