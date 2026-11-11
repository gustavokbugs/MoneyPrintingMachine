// Package config loads application configuration from environment
// variables, optionally seeded from a local .env file.
package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds runtime settings.
type Config struct {
	DatabaseURL       string
	HTTPAddr          string
	OllamaURL         string
	OllamaModel       string
	LLMTimeout        time.Duration
	LLMContextTokens  int
	WorkerConcurrency int
	LocalOwnerID      string
	WebDist           string
	WebBaseURL        string

	// YouTube Data API key for public data (trends, handle checks).
	YouTubeAPIKey     string
	PexelsAPIKey      string
	YouTubeDailyQuota int
	// Google OAuth client for channel connections.
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string
	// TokenEncryptionKey is a base64 32-byte key for stored OAuth tokens.
	TokenEncryptionKey string
}

// Load reads configuration. Values already present in the environment take
// precedence over values in envFile. A missing envFile is not an error.
func Load(envFile string) (Config, error) {
	if envFile != "" {
		if err := loadEnvFile(envFile); err != nil {
			return Config{}, err
		}
	}
	cfg := Config{
		DatabaseURL:  os.Getenv("DATABASE_URL"),
		HTTPAddr:     getenv("HTTP_ADDR", "127.0.0.1:8080"),
		OllamaURL:    getenv("OLLAMA_URL", "http://127.0.0.1:11434"),
		OllamaModel:  getenv("OLLAMA_MODEL", "qwen3:4b"),
		LocalOwnerID: getenv("LOCAL_OWNER_ID", "local"),
		WebDist:      os.Getenv("WEB_DIST"),
		WebBaseURL:   getenv("WEB_BASE_URL", "http://127.0.0.1:5173"),

		YouTubeAPIKey:      strings.TrimSpace(os.Getenv("YOUTUBE_API_KEY")),
		PexelsAPIKey:       strings.TrimSpace(os.Getenv("PEXELS_API_KEY")),
		GoogleClientID:     strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID")),
		GoogleClientSecret: strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_SECRET")),
		TokenEncryptionKey: strings.TrimSpace(os.Getenv("TOKEN_ENCRYPTION_KEY")),
	}
	cfg.GoogleRedirectURL = getenv("GOOGLE_REDIRECT_URL", "http://"+cfg.HTTPAddr+"/api/oauth/youtube/callback")
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required (see .env.example)")
	}
	var err error
	if cfg.LLMTimeout, err = durationEnv("LLM_TIMEOUT", 10*time.Minute); err != nil {
		return Config{}, err
	}
	if cfg.LLMContextTokens, err = intEnv("LLM_CONTEXT_TOKENS", 8192, 2048, 131072); err != nil {
		return Config{}, err
	}
	if cfg.WorkerConcurrency, err = intEnv("WORKER_CONCURRENCY", 1, 1, 8); err != nil {
		return Config{}, err
	}
	if cfg.YouTubeDailyQuota, err = intEnv("YOUTUBE_DAILY_QUOTA", 10000, 1, 1_000_000); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func getenv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func durationEnv(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration such as 10m", key)
	}
	return d, nil
}

func intEnv(key string, def, min, max int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < min || n > max {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", key, min, max)
	}
	return n, nil
}

// loadEnvFile sets KEY=VALUE pairs that are not already set. Lines starting
// with # and blank lines are ignored; surrounding quotes are removed.
func loadEnvFile(path string) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	line := 0
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		key, value, ok := strings.Cut(text, "=")
		if !ok {
			return fmt.Errorf("%s:%d: expected KEY=VALUE", path, line)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		}
		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("set %s: %w", key, err)
			}
		}
	}
	return sc.Err()
}
