package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"contentfactory/internal/channel"
	"contentfactory/internal/config"
	"contentfactory/internal/connections"
	"contentfactory/internal/launchpad"
	"contentfactory/internal/postgres"
	"contentfactory/internal/secrets"
	"contentfactory/internal/trends"
	"contentfactory/internal/youtube"
)

// youtubeDeps groups the YouTube integration components.
type youtubeDeps struct {
	data        *youtube.DataClient
	budget      *youtube.DailyBudget
	trends      *trends.Service
	connections *connections.Service
}

// buildYouTube wires the YouTube integration. Missing credentials leave
// the related features unavailable rather than failing startup; an invalid
// encryption key is a startup error.
func buildYouTube(cfg config.Config, pool *pgxpool.Pool, channels *channel.Service, log *slog.Logger) (youtubeDeps, error) {
	httpClient := &http.Client{Timeout: 20 * time.Second}
	budget, err := youtube.NewDailyBudget(postgres.NewQuotaStore(pool), cfg.YouTubeDailyQuota)
	if err != nil {
		return youtubeDeps{}, err
	}
	data := youtube.NewDataClient(httpClient, "", cfg.YouTubeAPIKey, budget)

	var box *secrets.Box
	if cfg.TokenEncryptionKey != "" {
		box, err = secrets.NewBox(cfg.TokenEncryptionKey)
		if err != nil {
			return youtubeDeps{}, fmt.Errorf("TOKEN_ENCRYPTION_KEY: %w", err)
		}
	}
	oauth := youtube.NewOAuth(httpClient, youtube.OAuthConfig{
		ClientID: cfg.GoogleClientID, ClientSecret: cfg.GoogleClientSecret, RedirectURL: cfg.GoogleRedirectURL,
	})
	conns := connections.NewService(postgres.NewConnectionStore(pool), oauth, data, channels, box)

	log.Info("youtube integration",
		"data_api", data.HasAPIKey(), "oauth", conns.Configured(), "daily_quota", cfg.YouTubeDailyQuota)
	if oauth.Configured() && box == nil {
		log.Warn("GOOGLE_CLIENT_ID is set but TOKEN_ENCRYPTION_KEY is missing; YouTube connections are disabled")
	}
	return youtubeDeps{
		data:        data,
		budget:      budget,
		trends:      trends.NewService(data, postgres.NewTrendSnapshotRepository(pool)),
		connections: conns,
	}, nil
}

// nameChecker adapts the Launchpad engine to the HTTP NameChecker port.
type nameChecker struct {
	engine   *launchpad.Engine
	channels *channel.Service
}

func (n nameChecker) HandleChecksAvailable() bool { return n.engine.HandleChecksAvailable() }

func (n nameChecker) CheckNames(ctx context.Context, ownerID, channelID string) (*channel.Channel, error) {
	return n.engine.CheckNames(ctx, n.channels, ownerID, channelID)
}
