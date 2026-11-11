# Architecture

## Shape

A modular monolith: one Go process serves the JSON API and runs background workers; PostgreSQL stores all state; a React app is the UI.

```
web (React, Vite) ──HTTP──▶ backend/cmd/server
                              ├─ internal/httpapi    delivery: routing, validation, error mapping
                              ├─ internal/channel    domain: channel aggregate, lifecycle, use cases
                              ├─ internal/launchpad  application: niche/strategy/identity/blueprint generation
                              ├─ internal/branding   deterministic SVG avatar and banner rendering
                              ├─ internal/platforms  platform capability catalog, connection read model
                              ├─ internal/trends     measured demand/competition/momentum from YouTube snapshots
                              ├─ internal/connections YouTube OAuth connect, callback, verify, disconnect
                              ├─ internal/youtube    adapter: YouTube Data API, Google OAuth, daily quota budget
                              ├─ internal/secrets    AES-256-GCM encryption for stored tokens
                              ├─ internal/rss        SSRF-safe feed fetching, RSS/Atom parsing, mention counts
                              ├─ internal/refresh    15-minute scheduler that enqueues the trend refresh job
                              ├─ internal/jobs       durable jobs: leases, retries, bounded concurrency
                              ├─ internal/llm        adapter: local Ollama client
                              └─ internal/postgres   adapter: repositories, job store, migrations
```

Dependencies point inward: `channel` knows nothing about HTTP, SQL, or models; adapters implement small interfaces declared by their consumers.

## Key decisions

- **Local AI only.** All generation goes through `internal/llm` to a local Ollama server. Model output is constrained by a JSON Schema (`format`), decoded strictly (unknown fields rejected), validated in the domain, and re-sampled at most three times when invalid.
- **Default model: `qwen3:4b`.** It is Apache 2.0 licensed and its 4-bit quantization (~2.6 GB) fits in the 4 GB VRAM of the development GPU. `qwen3:8b` gives better output when more memory is available. Thinking mode is disabled (`think: false`) to keep latency down. Verify the license on the model card before switching models.
- **No invented measurements.** Niche recommendations carry qualitative heuristic ratings from the model and a deterministic heuristic score derived from them. Measured fields stay `null` until a real data source is connected, and each source reports its status.
- **Deterministic brand assets.** Avatars and banners are generated as SVG from the identity palette and a hash of the name. They need no GPU, are reproducible, and are cached by a content key (ETag). The browser exports PNG at native size. Generative image models are a later option.
- **Openly licensed fonts.** Typography recommendations are restricted to an allowlist of SIL Open Font License families (`internal/launchpad/identity.go`).
- **Durable jobs.** Slow work runs as jobs in PostgreSQL, claimed with `FOR UPDATE SKIP LOCKED` and a lease, so a crashed worker's job is picked up again. Errors are retryable by default with exponential backoff and jitter; validation errors, unmet preconditions, and missing records fail permanently. Enqueue requests accept an `Idempotency-Key`.
- **Optimistic concurrency.** Channels carry a `version`; saves fail with a conflict when another write happened first, and use cases retry a bounded number of times.
- **Measured trend scores.** Demand = 100 · log10(1 + median views/day) / log10(1 + 50,000) over the 50 most viewed videos of the last 30 days; competition uses the same log scale on median channel subscribers (reference 10,000,000); momentum compares median views/day with a snapshot at least 12 hours older. Snapshots are cached for 6 hours to save quota. Google Trends has no generally available API, so search interest is never shown.
- **Trend refresh.** A ticker only enqueues a `trends.refresh` job per 15-minute slot (idempotency key per slot); the job syncs channel niches into the watchlist, claims due queries with `FOR UPDATE SKIP LOCKED`, measures at most five per run, defers to the Pacific-midnight quota reset when the budget is exhausted, and polls due RSS feeds.
- **RSS safety.** Feed requests use a dialer that refuses loopback, private, link-local, CGNAT, and reserved addresses (checked on the resolved IP, so DNS tricks and redirects cannot reach internal services), ignore proxy settings, cap bodies at 2 MB, and store only titles, links, and dates for 60 days.
- **Quota discipline.** Every Data API call reserves its documented cost in `api_quota_usage` before it is sent, against a local daily budget (Pacific-time days).
- **OAuth safety.** Authorization uses PKCE and a single-use state stored only as a SHA-256 hash with a 10-minute expiry. Tokens are encrypted with AES-256-GCM bound to channel and token kind, never logged, and never sent to the browser. A connection is "verified" only when Google returns the authorized channel and the upload scope was granted. One YouTube channel can be linked to only one internal channel.
- **Single local operator.** There is no authentication yet. Every request acts as `LOCAL_OWNER_ID`, and all data access is still owner-scoped so multi-user support can be added at the edge.

## Dependencies and licenses

| Component | License |
| --- | --- |
| Go standard library, pgx v5 | BSD-3-Clause, MIT |
| React, React Router, TanStack Query, Vite, Vitest | MIT |
| Ollama | MIT |
| Qwen3 4B weights | Apache 2.0 (verify on the model card) |
| Recommended fonts; Inter, Fraunces, JetBrains Mono, Space Grotesk (@fontsource 5.3.0) | SIL OFL 1.1 |
| FFmpeg 8.1 (BtbN GPL build) | GPL-3.0; used as a separate program, not linked |
| sherpa-onnx v2.0.1 | Apache 2.0 |
| Kokoro 82M v1.0 voice model (fp32, sherpa-onnx export) | Apache 2.0 (verify on the model card) |
| Parakeet TDT 110M word-timing model (NVIDIA, sherpa-onnx int8 export) | CC-BY-4.0 (credit NVIDIA; used only to time captions, nothing is redistributed) |
| Music and sound effects | Generated in code (`internal/production/audio.go`); original, no third-party audio |
| HyperFrames 0.8.209 | Apache 2.0; telemetry disabled through environment variables |
| GSAP 3.15.0 | GSAP Standard No-Charge License (free, including commercial use; may not be used in a tool that competes with Webflow) |

## Roadmap

1. Done: YouTube Data API trend snapshots (demand, competition, momentum) and handle checks.
2. Done: YouTube OAuth connection with PKCE, encrypted tokens, and platform verification.
3. Done: scheduled watchlist re-measurement (every 15 minutes as a durable job) and RSS feeds as a news-mention source.
4. Done: production pipeline: scripts with claims and a fact-check gate, local narration (Kokoro v1.0 TTS), animated scenes with transitions and word-by-word captions (HyperFrames HTML + GSAP rendered by headless Chrome, with a plain-background fallback), an original synthesized soundtrack (music bed ducked under the voice, effects on scene changes), and FFmpeg mastering to -14 LUFS.
5. Done for YouTube: publishing with one upload per video and AI disclosure. Next: TikTok and Instagram connections.
6. Analytics ingestion and the performance dashboard.
