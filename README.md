# Content Factory

A local-first platform for launching and running original video channels: niche discovery, channel strategy, brand identity, content planning, and (in upcoming milestones) script, narration, visuals, rendering, publishing, and analytics.

Every AI feature runs on a local model through [Ollama](https://github.com/ollama/ollama). No paid or remote AI APIs are used.

## Status

| Area | State |
| --- | --- |
| Channel workspaces and lifecycle (Draft → Archived) | Implemented |
| Channel Launchpad: niche discovery, strategy, identity, blueprint | Implemented (local LLM) |
| Brand assets: avatar and banner (SVG, PNG export) | Implemented (deterministic, no GPU) |
| Durable background jobs with retries | Implemented |
| Dashboard with bulk pause/resume | Implemented |
| Trend data sources (YouTube Data API, RSS, Reddit) | Not yet; measured scores show as "Not measured" |
| Platform OAuth (YouTube, TikTok, Instagram) | Not yet; guided manual setup only |
| Script, narration, visuals, rendering, publishing, analytics | Not yet |

Channels cannot reach **Ready** until a platform connection is verified by the platform, so the wizard currently stops at **Awaiting Platform Setup**. This is intentional: nothing is reported as connected unless it really is.

## Prerequisites

- Go 1.24+
- Node.js 22+
- PostgreSQL 15+ (uses `gen_random_uuid()`)
- [Ollama](https://ollama.com/download) with the default model:

  ```sh
  ollama pull qwen3:4b
  ```

## Setup

1. Create a database:

   ```sh
   psql -U postgres -c "CREATE DATABASE content_factory;"
   ```

2. Configure the backend: copy `.env.example` to `backend/.env` and set `DATABASE_URL`.

3. Run the backend (migrations apply automatically on start):

   ```sh
   cd backend
   go run ./cmd/server
   ```

4. Run the web app in another terminal:

   ```sh
   cd web
   npm install
   npm run dev
   ```

   Open http://127.0.0.1:5173.

## Configuration

| Variable | Purpose | Default |
| --- | --- | --- |
| `DATABASE_URL` | PostgreSQL connection string | required |
| `HTTP_ADDR` | API listen address | `127.0.0.1:8080` |
| `OLLAMA_URL` | Local Ollama server | `http://127.0.0.1:11434` |
| `OLLAMA_MODEL` | Model for all generation | `qwen3:4b` |
| `LLM_TIMEOUT` | Limit per model call | `10m` |
| `WORKER_CONCURRENCY` | Parallel background jobs | `1` |
| `LOCAL_OWNER_ID` | Single local operator ID | `local` |
| `WEB_DIST` | Serve built web app from the Go server | unset |
| `TEST_DATABASE_URL` | Enables PostgreSQL integration tests | unset |

The API has no login yet. Keep `HTTP_ADDR` on localhost.

## Checks

```sh
cd backend && gofmt -l . && go vet ./... && go test ./...
cd web && npm run typecheck && npm test && npm run build
```

Set `TEST_DATABASE_URL` to run the PostgreSQL integration tests; they create and drop an isolated schema.

## Documentation

- [Architecture](docs/architecture.md)
- [Channel lifecycle](docs/channel-lifecycle.md)
- [Platform capabilities and limits](docs/platform-capabilities.md)

## Troubleshooting

- **"Some services are unavailable: local model"**: start Ollama and run `ollama pull qwen3:4b` (or the model in `OLLAMA_MODEL`). Queued jobs retry automatically.
- **Generation is slow**: on a 4 GB GPU each step can take a minute or more, and a 30-idea blueprint makes four to six model calls. Keep `WORKER_CONCURRENCY=1` on a single small GPU.
- **Job failed with "invalid output"**: the model produced output that failed validation three times in a row. Regenerate; a larger model (`qwen3:8b`) is more reliable if your hardware allows it.
