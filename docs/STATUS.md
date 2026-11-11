# Project Status and Open Gaps

Master document for what works, what is blocked, and what needs the owner.
Update it whenever a gap is closed or a new one appears.

Last updated: 2026-11-10

## Legend

| Mark | Meaning |
| --- | --- |
| Done | Implemented and tested |
| Blocked | Implemented, waiting on something outside the code |
| Owner | Requires the project owner (account, credentials, legal acceptance, or a decision) |
| Planned | Not built yet |

## 1. What works today

| Area | Status | Notes |
| --- | --- | --- |
| Channel workspaces, lifecycle, status history, bulk pause/resume | Done | `internal/channel` |
| Durable background jobs (retries, leases, idempotency) | Done | `internal/jobs`, PostgreSQL-backed |
| PostgreSQL schema and migrations | Done | Applied automatically on server start |
| Channel Launchpad wizard (niche → strategy → branding → platforms → blueprint → review) | Done | Generation steps need the local model (section 2) |
| Avatar and banner generation (SVG, PNG export) | Done | Deterministic, no GPU |
| Web UI redesign (design system, themes, accessibility) | Done | `design-system/content-factory/MASTER.md` |
| YouTube trend measurement and handle checks | Blocked | Code done; needs an API key (gap G2) |
| YouTube channel connection (OAuth) | Blocked | Code done; needs an OAuth client (gap G3) |
| Local AI generation (niches, strategy, identity, blueprint) | Done | Ollama 0.40.16 with `qwen3:4b` installed on the dev machine |
| Scheduled trend refresh (watchlist) | Done | Runs every 15 minutes as a durable job; YouTube measurements need G2 |
| RSS news feeds as a trend source | Done | Add feeds on the Trends page; SSRF-protected fetcher |
| Video production (script, fact-check gate, narration, render) | Done | Kokoro voice, FFmpeg, captions and SRT; UI on each video page |
| Animated scenes (HyperFrames + GSAP) | Done | Styles: minimal, documentary, bold, tech, or auto from the niche; falls back to a plain background |
| YouTube publishing (resumable upload) | Blocked | Code and UI done; needs G3/G4. Private by default, marked as synthetic media |

## 2. Open gaps

### G1. Local language model — Done

- Ollama 0.40.16 installed from the signed installer (publisher verified: Ollama Inc.) into `%LOCALAPPDATA%ProgramsOllama`, and `qwen3:4b` pulled. The health check reports `local_model: ok`.
- Ollama starts with Windows through its tray app. If generation stops working after a reboot, open **Ollama** from the Start menu.

### G9. Install media tools for video production — Done

- Installed with `scripts/setup-media-tools.ps1` into `.local/` (gitignored), verifying every file: FFmpeg 8.1, sherpa-onnx v2.0.1 with the Kokoro int8 English voice and its lexicon, and the Inter font.
- Animated scenes also need `npm install` in `renderer/` (HyperFrames downloads a headless Chrome build to `~/.cache/hyperframes` on first render).

### G10. Production pipeline UI — Done

- Each video page covers script generation and editing, claim review, approval, voice and visual style, rendering, playback, downloads, and YouTube publishing.

### G2. YouTube Data API key — Owner

- **Why the owner:** creating it requires signing in to the owner's Google account and accepting Google Cloud's terms. Agents must not sign in, accept terms, or handle the key in web forms.
- **Steps:**
  1. Open <https://console.cloud.google.com/>, create a project (for example "content-factory").
  2. APIs & Services → Library → enable **YouTube Data API v3**.
  3. APIs & Services → Credentials → **Create credentials → API key**. Restrict it to YouTube Data API v3.
  4. Put it in `backend/.env` as `YOUTUBE_API_KEY=...` (the file is gitignored).
  5. Restart the backend.
- **Unlocks:** measured demand/competition/trend scores in niche discovery, and name handle checks.
- **Cost:** free within the default quota of 10,000 units/day. The app caps itself with `YOUTUBE_DAILY_QUOTA`.

### G3. Google OAuth client for channel connections — Owner

- **Why the owner:** the same account sign-in and terms acceptance as G2. The consent screen publishes an app name under the owner's identity.
- **Steps:**
  1. APIs & Services → **OAuth consent screen**: User type *External*, publishing status *Testing*; add your own Google account under **Test users**.
  2. Credentials → **Create credentials → OAuth client ID**, type **Web application**.
  3. Authorized redirect URI: `http://127.0.0.1:8081/api/oauth/youtube/callback` (port 8081 because port 8080 is used by another service on this machine).
  4. Put `GOOGLE_CLIENT_ID` and `GOOGLE_CLIENT_SECRET` in `backend/.env`. `TOKEN_ENCRYPTION_KEY` is already set locally.
  5. Restart the backend.
- **Known limits while in Testing mode:** Google shows an "unverified app" warning, and refresh tokens expire after 7 days (reconnect, or submit the app for verification). Uploads from unaudited API projects may be restricted to private; see `docs/platform-capabilities.md`.

### G4. Create the YouTube channel and approve access — Owner

- **Why the owner:** YouTube has no API for creating channels, and granting the app access to an account is the account holder's decision.
- **Steps:** create the channel at <https://www.youtube.com/create_channel> using the generated name, then use **Platform Setup → Connect YouTube account** and click **Allow** on Google's consent screen.
- **Unlocks:** the channel can move to **Ready**, then **Active**.

### G5. TikTok and Instagram integrations — Planned (needs owner later)

- Need developer apps on TikTok for Developers and Meta for Developers, plus TikTok's Content Posting API audit and Meta app review before public posting. Owner action is needed to register the apps and submit the reviews.

### G6. Merge the open pull request — Owner decision

- <https://github.com/gustavokbugs/MoneyPrintingMachine/pull/1> holds the YouTube integration and the UI redesign. `master` contains only the initial milestone until it is merged.

### G7. Public repository considerations — Owner decision

- The repository is public. Commit metadata shows the author email configured in git. To hide it in future commits, enable "Keep my email address private" on GitHub and use the provided `noreply` address in `git config user.email`. Existing commits keep their email unless history is rewritten, which is not done without explicit approval.

### G8. Job history retention — Done

- Each refresh run deletes finished jobs older than 30 days; queued and running jobs are never removed.

## 3. Planned milestones

1. **Trend refresh and RSS** — done on branch `feat/trend-refresh-rss`.
2. **Production pipeline** — done: scripts with a claim list, fact-check approval gate, Kokoro narration, HyperFrames animated scenes, FFmpeg render with captions and SRT.
3. **Publishing** — YouTube upload done (one upload per video, no automatic retry); needs G3/G4 to run. TikTok and Instagram later (G5).
4. **Analytics:** YouTube Analytics ingestion and the performance dashboard; depends on G3/G4.

## 4. Change log

| Date | Change |
| --- | --- |
| 2026-11-10 | Media tools installed (G9); production and publishing UI built (G10); HyperFrames animated scenes with four styles. |
| 2026-11-10 | Production pipeline backend added; PRs #1 and #2 merged; work continues on master. |
| 2026-11-10 | Trend refresh and RSS milestone built; Ollama and qwen3:4b installed (G1 closed). |
| 2026-11-10 | Document created. Repository published at <https://github.com/gustavokbugs/MoneyPrintingMachine>. Ollama installer verified and run. |
