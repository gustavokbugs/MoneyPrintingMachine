# Channel lifecycle

Implemented in `backend/internal/channel/lifecycle.go`. Every transition is recorded in `channel_status_events`.

## States

| State | Meaning |
| --- | --- |
| Draft | Workspace created; niche and strategy being defined. |
| Branding | Strategy exists; name and identity being chosen. |
| Awaiting Platform Setup | Identity complete; external accounts being connected and verified. |
| Ready | At least one platform connection verified and a content blueprint exists. |
| Active | Production and publishing run on schedule. |
| Paused | Production stopped; can resume. |
| Error | A problem needs attention (a reason is required). |
| Archived | Read-only; can be restored to Draft. |

## Transitions and guards

| From | To | Guard |
| --- | --- | --- |
| Draft | Branding | Niche and strategy exist |
| Draft | Archived | — |
| Branding | Draft, Error, Archived | — |
| Branding | Awaiting Platform Setup | Name selected and identity exists |
| Awaiting Platform Setup | Branding, Error, Archived | — |
| Awaiting Platform Setup | Ready | ≥ 1 verified platform connection and a blueprint |
| Ready | Awaiting Platform Setup, Error, Archived | — |
| Ready | Active | ≥ 1 verified platform connection |
| Active | Paused, Error, Archived | — |
| Paused | Active | ≥ 1 verified platform connection |
| Paused | Error, Archived | — |
| Error | Draft, Branding, Awaiting Platform Setup, Paused, Archived | — |
| Archived | Draft | — |

A "verified" connection is one whose account and permissions were confirmed by the platform API. Simulated or unverified connections never satisfy a guard.

Bulk operations support only `paused` and `active`, up to 100 channels, and report a result per channel.
