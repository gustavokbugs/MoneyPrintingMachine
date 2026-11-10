package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	content := "# comment\nDATABASE_URL=\"postgres://from-file\"\nOLLAMA_MODEL=qwen3:8b\nWORKER_CONCURRENCY=2\n"
	if err := os.WriteFile(env, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_URL", "")
	os.Unsetenv("DATABASE_URL")
	t.Setenv("OLLAMA_MODEL", "from-env")
	os.Unsetenv("WORKER_CONCURRENCY")
	t.Cleanup(func() { os.Unsetenv("WORKER_CONCURRENCY") })

	cfg, err := Load(env)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "postgres://from-file" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.OllamaModel != "from-env" {
		t.Errorf("environment should take precedence, got %q", cfg.OllamaModel)
	}
	if cfg.WorkerConcurrency != 2 || cfg.HTTPAddr != "127.0.0.1:8080" || cfg.LLMTimeout != 10*time.Minute {
		t.Errorf("unexpected config: %+v", cfg)
	}
}

func TestLoadValidation(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("WORKER_CONCURRENCY", "99")
	if _, err := Load(""); err == nil {
		t.Error("expected error for out-of-range concurrency")
	}
	t.Setenv("WORKER_CONCURRENCY", "1")
	t.Setenv("LLM_TIMEOUT", "soon")
	if _, err := Load(""); err == nil {
		t.Error("expected error for invalid duration")
	}
	t.Setenv("LLM_TIMEOUT", "")
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(filepath.Join(t.TempDir(), "missing.env")); err == nil {
		t.Error("expected error for missing DATABASE_URL")
	}
}

func TestLoadYouTubeDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("HTTP_ADDR", "127.0.0.1:9090")
	t.Setenv("GOOGLE_REDIRECT_URL", "")
	t.Setenv("YOUTUBE_DAILY_QUOTA", "")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GoogleRedirectURL != "http://127.0.0.1:9090/api/oauth/youtube/callback" || cfg.YouTubeDailyQuota != 10000 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	t.Setenv("YOUTUBE_DAILY_QUOTA", "0")
	if _, err := Load(""); err == nil {
		t.Fatal("expected error for zero quota")
	}
}
