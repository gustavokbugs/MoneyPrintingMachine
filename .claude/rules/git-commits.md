---
paths:
  - "**/*"
---

# Git Commit Rules

- Prefer frequent, small, atomic commits for granular tracking.
- Each commit must represent one independently understandable logical change.
- Split unrelated changes; do not split inseparable work into meaningless fragments.
- Stage explicit paths and inspect the staged diff.
- Use imperative subjects, ideally under 72 characters.
- Preferred format: `type(scope): imperative summary`.
- Suggested types: `feat`, `fix`, `refactor`, `test`, `docs`, `build`, `ci`, `chore`, `perf`.
- Never add AI co-author trailers or attribution, including `Co-authored-by: Claude`, `Co-authored-by: ChatGPT`, or `Co-authored-by: OpenAI`.
- Preserve the configured Git author identity; do not change `user.name` or `user.email`.
- Never commit secrets, `.env`, model weights, generated media, caches, or build artifacts.
- Do not amend, rebase, reset, force-push, or push without explicit authorization.
- Do not create commits unless requested or explicitly authorized by the current task.
- Before committing, inspect `git status` and the staged diff.
- When authorized to commit, create the smallest sensible atomic commits and report hashes and subjects.
