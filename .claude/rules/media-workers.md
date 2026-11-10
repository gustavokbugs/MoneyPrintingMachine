---
paths:
  - "**/*worker*"
  - "**/*job*"
  - "**/*render*"
  - "**/*media*"
  - "**/*ffmpeg*"
  - "**/*queue*"
---

# Media and Worker Rules

- Bound CPU, GPU, RAM, disk, and worker concurrency.
- Verify model hardware requirements and license before adoption.
- Persist job states; make jobs resumable and idempotent where practical.
- Support cancellation, timeouts, bounded retries, and actionable failure reasons.
- Cache reusable artifacts with stable content-based keys.
- Stream large files when possible.
- Validate media metadata and file sizes.
- Clean temporary files safely without deleting user assets.
- Never interpolate untrusted input into shell commands or FFmpeg command strings.
- Test partial failures, recovery, and concurrent execution.
