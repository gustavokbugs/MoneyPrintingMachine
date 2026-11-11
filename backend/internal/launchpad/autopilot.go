package launchpad

import (
	"context"
	"errors"
	"fmt"

	"contentfactory/internal/channel"
)

// KindAutopilot fills every missing part of a channel in one job: niche,
// strategy, identity, name, and content blueprint. Steps already present
// are kept, so the job is safe to retry and never overwrites human edits.
// Connecting a platform account stays a human step (OAuth consent).
const KindAutopilot = "launchpad.autopilot"

// AutopilotResult reports what the job generated.
type AutopilotResult struct {
	ChannelID string   `json:"channelId"`
	Generated []string `json:"generated"`
	// Notes explain choices made without a human (for example, which
	// name was picked and whether its handle was checked).
	Notes []string `json:"notes,omitempty"`
}

// Autopilot runs the missing setup steps for a channel in order.
func (e *Engine) Autopilot(ctx context.Context, store ChannelStore, ownerID, channelID string) (*AutopilotResult, error) {
	res := &AutopilotResult{ChannelID: channelID, Generated: []string{}}
	ch, err := store.Get(ctx, ownerID, channelID)
	if err != nil {
		return nil, err
	}
	if ch.Status == channel.StatusArchived {
		return nil, &channel.ValidationError{Field: "status", Message: "archived channels cannot be set up"}
	}

	if ch.Niche == "" {
		report, err := e.DiscoverNiches(ctx, DiscoverInput{Language: ch.Language, GeoTarget: ch.GeoTarget, OwnerID: ownerID})
		if err != nil {
			return nil, fmt.Errorf("discover niches: %w", err)
		}
		best, ok := bestNiche(report.Niches)
		if !ok {
			return nil, fmt.Errorf("%w: niche discovery returned no niches", ErrPrecondition)
		}
		if ch, err = store.UpdateSettings(ctx, ownerID, channelID, channel.SettingsUpdate{Niche: &best.Name}); err != nil {
			return nil, err
		}
		res.Generated = append(res.Generated, "niche")
		res.Notes = append(res.Notes, fmt.Sprintf("Picked the niche %q (heuristic score %d of %d candidates).", best.Name, best.HeuristicScore, len(report.Niches)))
	}

	if ch.Strategy == nil {
		s, err := e.GenerateStrategy(ctx, ch)
		if err != nil {
			return nil, fmt.Errorf("generate strategy: %w", err)
		}
		if ch, err = store.SetStrategy(ctx, ownerID, channelID, *s); err != nil {
			return nil, err
		}
		res.Generated = append(res.Generated, "strategy")
	}

	if ch.Identity == nil {
		idn, err := e.GenerateIdentity(ctx, ch)
		if err != nil {
			return nil, fmt.Errorf("generate identity: %w", err)
		}
		if ch, err = store.SetIdentity(ctx, ownerID, channelID, *idn); err != nil {
			return nil, err
		}
		res.Generated = append(res.Generated, "identity")
	}

	if ch.Name == "" {
		if e.HandleChecksAvailable() {
			// Best effort: unchecked names are still usable.
			if checked, err := e.CheckNames(ctx, store, ownerID, channelID); checked != nil {
				ch = checked
			} else if err != nil && !errors.Is(err, ErrHandleChecksUnavailable) {
				res.Notes = append(res.Notes, "Name handles could not be checked: "+err.Error())
			}
		}
		name, note, ok := pickName(ch.Identity.NameSuggestions)
		if !ok {
			return nil, fmt.Errorf("%w: the identity has no usable name suggestion", ErrPrecondition)
		}
		if ch, err = store.UpdateSettings(ctx, ownerID, channelID, channel.SettingsUpdate{Name: &name}); err != nil {
			return nil, err
		}
		res.Generated = append(res.Generated, "name")
		res.Notes = append(res.Notes, note)
	}

	if ch.Blueprint == nil {
		bp, err := e.GenerateBlueprint(ctx, ch)
		if err != nil {
			return nil, fmt.Errorf("generate blueprint: %w", err)
		}
		if _, err = store.SetBlueprint(ctx, ownerID, channelID, *bp); err != nil {
			return nil, err
		}
		res.Generated = append(res.Generated, "blueprint")
	}
	return res, nil
}

// bestNiche returns the niche with the highest heuristic score (the first
// one on ties, keeping the model's order).
func bestNiche(niches []Niche) (Niche, bool) {
	best, ok := Niche{}, false
	for _, n := range niches {
		if n.Name == "" {
			continue
		}
		if !ok || n.HeuristicScore > best.HeuristicScore {
			best, ok = n, true
		}
	}
	return best, ok
}

// pickName prefers the first suggestion whose YouTube handle is free, then
// the first one not known to be taken or invalid.
func pickName(names []channel.NameSuggestion) (string, string, bool) {
	for _, n := range names {
		if n.Availability == channel.AvailabilityNoChannel {
			return n.Name, fmt.Sprintf("Named the channel %q; no YouTube channel uses @%s yet (trademarks are not checked).", n.Name, n.Handle), true
		}
	}
	for _, n := range names {
		if n.Name != "" && n.Availability != channel.AvailabilityHandleTaken && n.Availability != channel.AvailabilityInvalidHandle {
			return n.Name, fmt.Sprintf("Named the channel %q; its YouTube handle was not checked.", n.Name), true
		}
	}
	return "", "", false
}
