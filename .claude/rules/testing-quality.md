---
paths:
  - "**/*"
---

# Testing and Quality Rules

- Add or update tests for every meaningful behavior change.
- Test failure paths, invalid input, authorization, and retries where relevant.
- Use deterministic fixtures; ordinary unit tests must not depend on live external APIs.
- Avoid arbitrary sleeps and brittle snapshots.
- Run formatters, targeted tests, lint/type-check, and builds using repository scripts.
- Inspect the final diff for secrets, generated files, and unrelated changes.
- Do not weaken tests, types, lint rules, or security controls to force a pass.
- Report only checks actually executed and their real outcomes.
