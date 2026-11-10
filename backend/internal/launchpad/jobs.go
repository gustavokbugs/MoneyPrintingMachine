package launchpad

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"contentfactory/internal/channel"
	"contentfactory/internal/jobs"
)

// ErrPrecondition means a generation step was requested before the data it
// depends on exists (for example, identity before strategy).
var ErrPrecondition = errors.New("precondition not met")

// Job kinds handled by the Launchpad.
const (
	KindDiscoverNiches    = "launchpad.discover_niches"
	KindGenerateStrategy  = "launchpad.generate_strategy"
	KindGenerateIdentity  = "launchpad.generate_identity"
	KindGenerateBlueprint = "launchpad.generate_blueprint"
)

// ChannelStore is the subset of channel use cases the Launchpad jobs need.
type ChannelStore interface {
	Get(ctx context.Context, ownerID, id string) (*channel.Channel, error)
	SetStrategy(ctx context.Context, ownerID, id string, s channel.Strategy) (*channel.Channel, error)
	SetIdentity(ctx context.Context, ownerID, id string, i channel.Identity) (*channel.Channel, error)
	SetBlueprint(ctx context.Context, ownerID, id string, b channel.Blueprint) (*channel.Channel, error)
}

// ChannelJobResult is returned by channel generation jobs.
type ChannelJobResult struct {
	ChannelID string `json:"channelId"`
}

// Handlers returns job handlers for every Launchpad job kind.
func (e *Engine) Handlers(store ChannelStore) map[string]jobs.Handler {
	return map[string]jobs.Handler{
		KindDiscoverNiches: func(ctx context.Context, j jobs.Job) (any, error) {
			var in DiscoverInput
			if err := json.Unmarshal(j.Input, &in); err != nil {
				return nil, jobs.Permanent(fmt.Errorf("decode input: %w", err))
			}
			in.OwnerID = j.OwnerID
			report, err := e.DiscoverNiches(ctx, in)
			return report, classify(err)
		},
		KindGenerateStrategy: e.channelHandler(store, func(ctx context.Context, ch *channel.Channel) error {
			s, err := e.GenerateStrategy(ctx, ch)
			if err != nil {
				return err
			}
			_, err = store.SetStrategy(ctx, ch.OwnerID, ch.ID, *s)
			return err
		}),
		KindGenerateIdentity: e.channelHandler(store, func(ctx context.Context, ch *channel.Channel) error {
			idn, err := e.GenerateIdentity(ctx, ch)
			if err != nil {
				return err
			}
			_, err = store.SetIdentity(ctx, ch.OwnerID, ch.ID, *idn)
			return err
		}),
		KindGenerateBlueprint: e.channelHandler(store, func(ctx context.Context, ch *channel.Channel) error {
			bp, err := e.GenerateBlueprint(ctx, ch)
			if err != nil {
				return err
			}
			_, err = store.SetBlueprint(ctx, ch.OwnerID, ch.ID, *bp)
			return err
		}),
	}
}

func (e *Engine) channelHandler(store ChannelStore, run func(context.Context, *channel.Channel) error) jobs.Handler {
	return func(ctx context.Context, j jobs.Job) (any, error) {
		if j.ChannelID == "" {
			return nil, jobs.Permanent(errors.New("job has no channel"))
		}
		ch, err := store.Get(ctx, j.OwnerID, j.ChannelID)
		if err != nil {
			return nil, classify(err)
		}
		if err := run(ctx, ch); err != nil {
			return nil, classify(err)
		}
		return ChannelJobResult{ChannelID: ch.ID}, nil
	}
}

// classify marks errors that cannot succeed on retry as permanent. Other
// errors (unavailable model, invalid samples, transient database errors)
// remain retryable within the job's attempt budget.
func classify(err error) error {
	if err == nil {
		return nil
	}
	var ve *channel.ValidationError
	var te *channel.TransitionError
	if errors.Is(err, ErrPrecondition) || errors.Is(err, channel.ErrNotFound) ||
		errors.As(err, &ve) || errors.As(err, &te) {
		return jobs.Permanent(err)
	}
	return err
}
