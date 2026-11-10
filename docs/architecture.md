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
- **Single local operator.** There is no authentication yet. Every request acts as `LOCAL_OWNER_ID`, and all data access is still owner-scoped so multi-user support can be added at the edge.

## Dependencies and licenses

| Component | License |
| --- | --- |
| Go standard library, pgx v5 | BSD-3-Clause, MIT |
| React, React Router, TanStack Query, Vite, Vitest | MIT |
| Ollama | MIT |
| Qwen3 4B weights | Apache 2.0 (verify on the model card) |
| Recommended fonts | SIL OFL 1.1 |

## Roadmap

1. Trend intelligence: YouTube Data API (API key) and RSS collectors with stored snapshots; feed measured niche scores.
2. YouTube OAuth: connect accounts, detect channels, verify scopes, store encrypted tokens.
3. Production pipeline: research with sources, scripts with citations and fact-check gates, local narration (Kokoro TTS, Apache 2.0, CPU-friendly), visuals (motion graphics, optional Stable Diffusion), FFmpeg rendering.
4. Publishing with idempotency and platform-specific validation; TikTok and Instagram connections.
5. Analytics ingestion and the performance dashboard.
