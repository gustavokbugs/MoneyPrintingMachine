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
| Trend data: YouTube Data API (demand, competition, momentum snapshots) | Implemented (needs `YOUTUBE_API_KEY`) |
| Name handle checks against YouTube | Implemented (needs `YOUTUBE_API_KEY`) |
| YouTube account connection (OAuth 2.0 + PKCE, encrypted tokens, verification) | Implemented (needs a Google OAuth client) |
| Scheduled trend refresh (watchlist re-measured daily) | Implemented |
| News mentions from RSS feeds | Implemented (add feeds on the Trends page) |
| Trend data: Reddit | Not yet |
| TikTok and Instagram connections | Not yet; guided manual setup only |
| Script, narration, visuals, rendering, publishing, analytics | Not yet |

Channels reach **Ready** only after a platform connection is verified by the platform (today: a YouTube channel authorized with upload permission). Nothing is reported as connected unless it really is.

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

5. Optional, for video production: install the media tools and the scene renderer.

   ```sh
   powershell -ExecutionPolicy Bypass -File scripts/setup-media-tools.ps1
   cd renderer && npm install
   ```

   Without `renderer/`, videos render on a plain background.

## YouTube setup (optional)

Both parts are free and use a Google Cloud project you own.

**Trend data and handle checks (API key)**

1. In [Google Cloud Console](https://console.cloud.google.com/), create a project and enable **YouTube Data API v3**.
2. Create an **API key** (APIs & Services → Credentials). Restrict it to the YouTube Data API.
3. Set `YOUTUBE_API_KEY` in `backend/.env`.

Each niche measurement costs about 102 quota units (one search plus two lookups) and is cached for 6 hours; a discovery run measures up to 10 niches. Handle checks cost 1 unit per name. The app stops calling the API when `YOUTUBE_DAILY_QUOTA` is reached; usage resets at midnight Pacific time.

**Channel connection (OAuth)**

1. In the same project, configure the **OAuth consent screen** (External, Testing is fine) and add your Google account as a test user.
2. Create an **OAuth client ID** of type **Web application** with the authorized redirect URI `http://127.0.0.1:8080/api/oauth/youtube/callback`.
3. Set `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, and `TOKEN_ENCRYPTION_KEY` (generate it with `cd backend && go run ./cmd/keygen`).
4. Restart the backend and use **Platform Setup → Connect YouTube account**.

Scopes requested: `youtube.readonly`, `youtube.upload`, `yt-analytics.readonly`. While the app is in Testing mode, Google shows an "unverified app" warning and expires refresh tokens after 7 days; reconnect or submit the app for verification. Uploads from unaudited API projects may be restricted to private; see [docs/platform-capabilities.md](docs/platform-capabilities.md).

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
| `WEB_BASE_URL` | Browser return address after OAuth | `http://127.0.0.1:5173` |
| `YOUTUBE_API_KEY` | YouTube Data API key (trends, handle checks) | unset |
| `YOUTUBE_DAILY_QUOTA` | Local daily quota budget in units | `10000` |
| `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` | Google OAuth client for connections | unset |
| `GOOGLE_REDIRECT_URL` | OAuth redirect URI | `http://<HTTP_ADDR>/api/oauth/youtube/callback` |
| `TOKEN_ENCRYPTION_KEY` | Base64 32-byte key for stored tokens | unset |
| `TEST_DATABASE_URL` | Enables PostgreSQL integration tests | unset |

The API has no login yet. Keep `HTTP_ADDR` on localhost.

## Checks

```sh
cd backend && gofmt -l . && go vet ./... && go test ./...
cd web && npm run typecheck && npm test && npm run build
```

Set `TEST_DATABASE_URL` to run the PostgreSQL integration tests; they create and drop an isolated schema.

## Documentation

- [Project status and open gaps](docs/STATUS.md): start here
- [Architecture](docs/architecture.md)
- [Channel lifecycle](docs/channel-lifecycle.md)
- [Platform capabilities and limits](docs/platform-capabilities.md)
- [UI design system](design-system/content-factory/MASTER.md): tokens, components, and accessibility rules for the web app, derived from the vendored [ui-ux-pro-max](.claude/skills/ui-ux-pro-max/SOURCE.md) skill. Read it before changing UI.

## Troubleshooting

- **"Some services are unavailable: local model"**: start Ollama and run `ollama pull qwen3:4b` (or the model in `OLLAMA_MODEL`). Queued jobs retry automatically.
- **Generation is slow**: on a 4 GB GPU each step can take a minute or more, and a 30-idea blueprint makes four to six model calls. Keep `WORKER_CONCURRENCY=1` on a single small GPU.
- **Job failed with "invalid output"**: the model produced output that failed validation three times in a row. Regenerate; a larger model (`qwen3:8b`) is more reliable if your hardware allows it.
