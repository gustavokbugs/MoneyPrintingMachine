---
paths:
  - "backend/**/*.go"
  - "internal/**/*.go"
  - "cmd/**/*.go"
  - "**/*_test.go"
---

# Go Backend Rules

- Run `gofmt`; use `go vet` and `go test ./...` when applicable.
- Pass `context.Context` through I/O and blocking operations.
- Wrap errors with useful context and handle them deliberately.
- Do not panic for expected error paths.
- Keep business logic out of HTTP handlers and infrastructure adapters.
- Define small interfaces at consumer boundaries; avoid interfaces created only for mocks.
- Avoid package-level mutable state.
- Validate inputs and preserve domain invariants.
- Use transactions for multi-step database invariants.
- Make jobs idempotent, cancellable, observable, and bounded.
- Keep provider-specific logic behind adapters.
- Add tests for success, failure, invalid input, and concurrency-sensitive behavior.
