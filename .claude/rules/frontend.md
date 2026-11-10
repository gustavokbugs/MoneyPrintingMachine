---
paths:
  - "frontend/**/*.{ts,tsx,js,jsx}"
  - "web/**/*.{ts,tsx,js,jsx}"
  - "src/**/*.{ts,tsx,js,jsx}"
---

# Frontend Rules

- Respect strict TypeScript; avoid `any`, unsafe casts, and `@ts-ignore`.
- Keep components focused and reusable.
- Separate server state, local UI state, and form state.
- Handle loading, empty, error, and success states.
- Use semantic HTML, accessible labels, keyboard support, and visible focus.
- Avoid effects for derived state.
- Keep API contracts typed and validate untrusted data at boundaries.
- Do not put business rules exclusively in UI components.
- Keep strings in the localization system; default locale is English (`en-US`).
- Test important interactions and permission boundaries.
