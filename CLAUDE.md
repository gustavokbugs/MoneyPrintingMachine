# Project Instructions — AI Content Factory

## 1. Product context

Build a reliable, modular AI Content Factory for researching niches and trends, launching branded channels, producing original short- and long-form videos, publishing through supported official platform integrations, and measuring channel/video performance.

The product must support multiple independent channels and platform accounts. Channel creation must distinguish between:
1. Creating an internal channel workspace and brand.
2. Connecting an existing external platform account.
3. Creating/configuring an external channel or profile.
4. Publishing content and retrieving analytics.

Never claim an external account or channel was created unless the platform confirms it. Use official APIs and supported OAuth flows. When a platform does not expose a supported creation API, provide a guided manual setup flow. Never bypass CAPTCHA, platform safeguards, rate limits, or account-creation restrictions.

## 2. Language

- All product UI, generated channel content defaults, developer documentation, code comments, logs, API descriptions, prompts, and examples must be in English.
- Default content locale: American English (`en-US`).
- Keep localization architecture possible; do not hard-code user-facing strings throughout components.
- Do not introduce Portuguese strings unless explicitly requested for a specific localization feature.

## 3. Engineering principles

- Prefer the simplest architecture that safely supports the requirement.
- Use a modular monolith initially. Do not introduce microservices or Kubernetes without a demonstrated need.
- Optimize for maintainability, correctness, observability, and efficient resource use.
- Avoid speculative abstractions, unnecessary dependencies, duplicated logic, and premature optimization.
- Keep business logic independent from HTTP handlers, UI components, external APIs, and infrastructure.
- Make boundaries explicit: domain, application/use cases, infrastructure/adapters, and delivery interfaces.
- Favor deterministic, testable functions and explicit error handling.
- Do not silently swallow errors or return fake success responses.
- Never fabricate integrations, API capabilities, metrics, analytics, trend data, or test results.

## 4. Technology defaults

Use these as defaults unless the existing repository establishes a justified alternative:

- Backend: Go.
- Frontend: React, TypeScript, Vite, and the repository's established styling system.
- Database: PostgreSQL.
- Database access: SQLC or a similarly type-safe, explicit approach.
- Media processing: FFmpeg.
- Local model/runtime integrations: evaluate Ollama, llama.cpp, and suitable local inference tools.
- Containers: Docker and Docker Compose for local development.
- Background work: durable jobs with retries, bounded concurrency, idempotency, and observable states.

Before adding a library or service, inspect the existing dependency graph and verify maintenance status, license, security posture, platform compatibility, and resource requirements.

## 5. Free/open-source requirement

Prefer free and open-source software, local execution, and official free API tiers.

Before adopting any dependency, model, repository, dataset, or API:
- Verify its current license and whether commercial use is allowed.
- Check model weights, code, datasets, and assets separately; their licenses may differ.
- Identify attribution, redistribution, usage, and output restrictions.
- Check whether the free tier has quotas, approval requirements, or operational limits.
- Do not assume that "open source", "free download", or "free tier" means unrestricted commercial use.
- Record material restrictions in project documentation.

Do not introduce paid APIs, subscriptions, usage-based services, or hidden cloud dependencies without explicit approval.

## 6. Content integrity and platform compliance

The product must prioritize original, useful, monetizable content—not mass-produced low-value output.

Implement or preserve:
- Originality and similarity checks where practical.
- Source tracking and citations for factual scripts.
- Fact-checking workflows for claims, dates, statistics, and sensitive topics.
- Rights checks for music, footage, images, fonts, and generated assets.
- Human review and approval gates for risky or uncertain content.
- Clear content provenance and generation metadata.
- Platform-specific publishing validation.

Do not build features intended to evade copyright detection, platform policies, monetization review, spam controls, or AI-content disclosure requirements. Do not scrape or automate platforms contrary to their terms.

## 7. Architecture and implementation workflow

Before changing code:
1. Inspect the repository structure and existing conventions.
2. Read relevant documentation and nearby implementations.
3. Identify the smallest complete vertical slice.
4. State assumptions only when they materially affect implementation.
5. Check whether an existing abstraction or dependency already solves the problem.

While implementing:
- Make the smallest coherent change.
- Keep modules cohesive and APIs explicit.
- Validate inputs at system boundaries.
- Use context cancellation and timeouts for I/O.
- Add bounded retries with exponential backoff and jitter only where retrying is safe.
- Make background jobs idempotent.
- Use transactions for multi-step database invariants.
- Add migrations for schema changes; never rely on undocumented manual database edits.
- Keep secrets out of source code, logs, fixtures, screenshots, and commits.
- Do not change unrelated files to satisfy personal style preferences.

After implementing:
1. Format code.
2. Run targeted tests.
3. Run relevant lint, type-check, and build commands.
4. Run broader checks when practical.
5. Inspect `git diff` for correctness, scope, secrets, generated files, and accidental changes.
6. Report exactly what was run and any failures or skipped checks.

Do not claim tests passed unless they were actually executed successfully.

## 8. Go standards

- Follow `gofmt`; run `go vet` where applicable.
- Keep exported APIs documented.
- Wrap errors with useful context (`%w`) and handle them deliberately.
- Avoid panic for normal error paths.
- Pass `context.Context` through operations that perform I/O or may block.
- Avoid package-level mutable state.
- Prefer small interfaces defined by consumers.
- Do not create interfaces solely for mocking without a design need.
- Validate data at boundaries and keep domain invariants in domain/application layers.
- Use table-driven tests where they improve clarity.
- Run `go test ./...` when the scope permits.
- Use race detection for concurrency-sensitive code when practical.

## 9. TypeScript and React standards

- Enable and respect strict TypeScript settings.
- Avoid `any`; use `unknown` and narrow it when the type is not known.
- Do not silence type errors with casts or `@ts-ignore` unless there is a documented, unavoidable reason.
- Keep components focused and compose reusable UI from small units.
- Keep server state, local UI state, and form state conceptually separate.
- Handle loading, empty, error, and success states explicitly.
- Use accessible semantic HTML, labels, keyboard interaction, and visible focus.
- Avoid effects for derived state; derive values during render or with memoization only when useful.
- Avoid premature global state.
- Keep API contracts typed and validate untrusted responses at boundaries.
- Do not put business rules exclusively in UI components.

## 10. Database and API standards

- Use explicit migrations and reversible changes where feasible.
- Add appropriate indexes based on query patterns, not guesswork.
- Avoid N+1 queries and unbounded list endpoints.
- Use pagination for potentially large collections.
- Define stable API error shapes and meaningful HTTP status codes.
- Validate and authorize every operation server-side.
- Enforce tenant/channel ownership at the data-access or application boundary.
- Use idempotency keys for externally visible operations such as publishing where supported.
- Never expose provider tokens or secrets to the frontend.

## 11. Security and privacy

- Treat all external content, model output, uploads, and provider responses as untrusted input.
- Use least-privilege OAuth scopes.
- Encrypt sensitive tokens at rest using an appropriate secret-management strategy.
- Redact secrets and personal data from logs.
- Protect against SSRF, path traversal, injection, unsafe file handling, and arbitrary command execution.
- Never execute model-generated shell commands, SQL, or code without explicit validation and controlled execution.
- Do not log full OAuth tokens, API keys, passwords, or authorization headers.
- Include authorization tests for channel and account boundaries.

## 12. Media pipeline and resource management

- Treat rendering and inference as resource-intensive background work.
- Bound CPU, GPU, RAM, disk, and worker concurrency.
- Make jobs resumable where practical and persist meaningful job states.
- Cache reusable intermediate artifacts with stable content-based keys.
- Avoid loading entire large media files into memory when streaming is possible.
- Validate media duration, dimensions, codecs, and file size.
- Clean up temporary files safely and predictably.
- Provide graceful cancellation and failure recovery.
- Do not assume the development machine can run large video-generation models; inspect VRAM/RAM requirements and provide fallback paths.

## 13. Testing and quality gates

For every meaningful behavior change:
- Add or update unit tests.
- Add integration tests for database, queue, filesystem, and provider boundaries when relevant.
- Test failure paths, invalid inputs, retries, and authorization—not only happy paths.
- Use deterministic fixtures; do not depend on live external APIs in ordinary unit tests.
- Mock external boundaries, not the internal implementation being tested.
- Keep tests independent and repeatable.
- Avoid brittle snapshots and arbitrary sleeps.
- Add regression tests for fixed bugs.

A change is not complete merely because it compiles. It must meet the relevant behavior, test, security, and documentation requirements.

## 14. Git and commit discipline

The user values granular history and traceability.

- Prefer many small, atomic commits over one large catch-all commit.
- Each commit must represent one independently understandable and reviewable change.
- Split unrelated work into separate commits.
- A commit should contain only the files needed for its one logical purpose.
- Good examples: add one migration; implement one repository method; add tests for one use case; add one UI state; fix one isolated bug.
- Do not split a single inseparable change into meaningless fragments merely to inflate commit count.
- Use imperative, specific commit subjects, ideally under 72 characters.
- Recommended format: `type(scope): imperative summary`.
- Suggested types: `feat`, `fix`, `refactor`, `test`, `docs`, `build`, `ci`, `chore`, `perf`.
- Stage explicit paths; avoid indiscriminate `git add .` when unrelated files may exist.
- Inspect `git status` and staged diff before each commit.
- Never include secrets, local environment files, generated media, model weights, caches, or build output.
- Do not add AI co-author attribution or trailers. Specifically, never add `Co-authored-by: Claude`, `Co-authored-by: ChatGPT`, `Co-authored-by: OpenAI`, or equivalent AI attribution.
- Do not alter `user.name` or `user.email`; preserve the repository owner's configured Git identity.
- Do not amend, rebase, reset, force-push, or push commits unless the user explicitly requests it.
- Do not create commits unless the user asks for commits or the active task explicitly authorizes committing.
- If asked to commit, create the smallest sensible atomic commit(s), then report each hash and subject.

## 15. Token and context efficiency

- Read only the files needed for the current task; use targeted searches instead of dumping the entire repository.
- Do not repeatedly reread unchanged files.
- Prefer concise plans and concise progress updates.
- Avoid restating project instructions in every response.
- Reuse established patterns instead of inventing alternatives.
- Do not generate lengthy documentation unless it provides lasting operational value or is requested.
- Batch independent read-only inspections where safe, but keep code changes and commits logically isolated.
- Before a large task, identify milestones and complete them in focused increments.
- Preserve important decisions in concise project documentation rather than relying on conversation history.
- Never skip essential tests or safety checks merely to save tokens.

## 16. Documentation

Update documentation when behavior, setup, configuration, architecture, or operational procedures change.

Keep docs accurate and concise. Include:
- Prerequisites.
- Setup and run commands.
- Environment variables (names and purpose, never real values).
- Migration and worker instructions.
- Troubleshooting for non-obvious failures.
- External API limitations and required permissions.
- License and attribution notes for adopted models/assets.

## 17. Agent response format

At the end of a task, report:
- What changed.
- Key design decisions.
- Tests/checks actually run and their results.
- Any limitations, risks, or follow-up work.
- Whether a database migration or configuration change is required.

Do not provide a long narrative of every trivial action. Be factual and concise.
