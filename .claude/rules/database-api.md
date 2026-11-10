---
paths:
  - "**/*.sql"
  - "**/*migration*"
  - "**/*schema*"
  - "**/*repository*"
  - "**/*handler*"
  - "**/*route*"
---

# Database and API Rules

- Use explicit versioned migrations.
- Add indexes based on query patterns; avoid N+1 and unbounded list endpoints.
- Paginate potentially large collections.
- Define stable error responses and meaningful HTTP status codes.
- Validate inputs and authorize every operation server-side.
- Enforce channel/account ownership boundaries.
- Use transactions for related writes that must remain consistent.
- Use idempotency for external publishing operations where supported.
- Never expose provider tokens or secrets to the frontend.
- Add migration, repository, API, and authorization tests where relevant.
