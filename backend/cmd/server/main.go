// Command server runs the AI Content Factory API and background workers.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	// Embed the time zone database so scheduling works on systems without
	// one (for example Windows or minimal containers).
	_ "time/tzdata"

	"contentfactory/internal/channel"
	"contentfactory/internal/config"
	"contentfactory/internal/httpapi"
	"contentfactory/internal/jobs"
	"contentfactory/internal/launchpad"
	"contentfactory/internal/llm"
	"contentfactory/internal/postgres"
	"contentfactory/internal/refresh"
	"contentfactory/internal/rss"
	"contentfactory/internal/trends"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(log); err != nil {
		log.Error("server stopped with error", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load(".env")
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	startCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	pool, err := postgres.Connect(startCtx, cfg.DatabaseURL, 10)
	cancel()
	if err != nil {
		return err
	}
	defer pool.Close()

	applied, err := postgres.Migrate(ctx, pool)
	if err != nil {
		return err
	}
	if len(applied) > 0 {
		log.Info("applied migrations", "versions", strings.Join(applied, ","))
	}

	model, err := llm.NewClient(cfg.OllamaURL, cfg.OllamaModel, cfg.LLMTimeout)
	if err == nil {
		model.WithContextTokens(cfg.LLMContextTokens)
	}
	if err != nil {
		return err
	}
	pingCtx, pingCancel := context.WithTimeout(ctx, 3*time.Second)
	if err := model.Ping(pingCtx); err != nil {
		log.Warn("local model is not ready; generation jobs will retry until it is",
			"model", cfg.OllamaModel, "error", err)
	}
	pingCancel()

	connections := postgres.NewPlatformConnectionRepository(pool)
	channels := channel.NewService(postgres.NewChannelRepository(pool), connections)
	jobStore := postgres.NewJobStore(pool)
	engine := launchpad.NewEngine(model)

	yt, err := buildYouTube(cfg, pool, channels, log)
	if err != nil {
		return err
	}
	feeds := rss.NewService(postgres.NewFeedRepository(pool), rss.NewFetcher(rss.NewClient(30*time.Second, false)))
	watchlist := postgres.NewWatchlistRepository(pool)
	engine.WithMeasurer(yt.trends).WithHandleLookup(yt.data).WithNews(feeds)

	handlers := engine.Handlers(channels)
	handlers[refresh.KindTrendRefresh] = refresh.Handler(trends.NewRefresher(watchlist, yt.trends), feeds, jobStore)

	runner := jobs.NewRunner(jobStore, handlers, jobs.RunnerConfig{
		Concurrency: cfg.WorkerConcurrency,
		Lease:       cfg.LLMTimeout*3 + time.Minute, // Blueprints make several model calls.
	}, log.With("component", "jobs"))
	runnerDone := make(chan struct{})
	go func() {
		defer close(runnerDone)
		runner.Run(ctx)
	}()

	// Enqueue a trend refresh every 15 minutes; the job itself decides
	// which watches and feeds are due.
	go refresh.Schedule(ctx, jobs.NewEnqueuer(jobStore), cfg.LocalOwnerID, 15*time.Minute, log.With("component", "scheduler"))

	api := httpapi.NewHandler(httpapi.Deps{
		Channels:    channels,
		Jobs:        jobs.NewEnqueuer(jobStore),
		SavedNiches: postgres.NewSavedNicheRepository(pool),
		Connections: connections,
		Health: []httpapi.HealthCheck{
			{Name: "database", Check: func(ctx context.Context) error { return pool.Ping(ctx) }},
			{Name: "local_model", Check: model.Ping},
		},
		YouTube:    yt.connections,
		Trends:     yt.trends,
		Names:      nameChecker{engine: engine, channels: channels},
		Quota:      yt.budget,
		Watchlist:  watchlist,
		Feeds:      feeds,
		Trajectory: yt.trends,
		WebBaseURL: cfg.WebBaseURL,
		Model:      cfg.OllamaModel,
		Log:        log.With("component", "http"),
		OwnerID:    cfg.LocalOwnerID,
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           withStatic(api, cfg.WebDist),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.HTTPAddr, "model", cfg.OllamaModel)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil {
			stop()
			<-runnerDone
			return err
		}
	}
	log.Info("shutting down")
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelShutdown()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("http shutdown", "error", err)
	}
	<-runnerDone
	return nil
}

// withStatic serves the built web app from dir (when set) for non-API
// paths, falling back to index.html for client-side routes.
func withStatic(api http.Handler, dir string) http.Handler {
	if dir == "" {
		return api
	}
	files := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			api.ServeHTTP(w, r)
			return
		}
		clean := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			http.NotFound(w, r)
			return
		}
		if _, err := os.Stat(filepath.Join(dir, clean)); err != nil || clean == "." {
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
			return
		}
		files.ServeHTTP(w, r)
	})
}
