# Platform capabilities

Last reviewed: 2026-11-10. Public sources disagree on several figures; items marked **verify** must be confirmed against the official documentation before building on them. The in-app catalog lives in `backend/internal/platforms/capabilities.go`.

## Summary

| Step | YouTube | TikTok | Instagram |
| --- | --- | --- | --- |
| Create account/channel via API | No | No | No |
| Connect account (OAuth) | Yes | Yes (Login Kit) | Yes (Instagram Login) |
| Detect and verify account via API | Yes | Yes | Yes |
| Edit profile text via API | Yes, with the broader `youtube` scope (not requested by this app yet) | No | No |
| Upload avatar/banner via API | Banner: yes; avatar: no (verify) | No | No |
| Publish via API | Yes | Yes (audit needed for public posts) | Yes (professional account + app review) |
| Analytics via API | Yes (YouTube Analytics API) | Limited | Yes (Insights) |

Where a step has no API, the app shows a guided manual step and only moves forward after detecting the result through the API.

## YouTube

- Scopes: `youtube.upload`, `youtube.readonly`, `yt-analytics.readonly` (least privilege; add `youtube` only when profile editing is implemented).
- Implemented in this app: OAuth connection with PKCE, channel detection via `channels.list?mine=true`, scope verification, token refresh and revocation, trend measurements (`search.list` 100 units + `videos.list` 1 + `channels.list` 1), and handle checks (`channels.list?forHandle=` 1 unit).
- Data API default quota: 10,000 units/day per project.
- `videos.insert` cost: historically about 1,600 units; recent sources report it was cut to about 100 units or moved to a separate bucket of about 100 calls/day (**verify** in the Google quota calculator).
- Uploads from unaudited projects created after July 2020 have been locked to private; one recent report says this was relaxed (**verify**).

## TikTok

- Scopes: `user.info.basic`, `video.upload`, `video.publish`, `video.list`.
- Unaudited clients post only as `SELF_ONLY`, only to private accounts, for up to 5 users per 24 hours. The Content Posting API audit lifts this; `video.publish` approval alone does not.
- About 15 Direct Post posts per creator per day, shared across clients (approximate).

## Instagram

- Scopes (Instagram Login): `instagram_business_basic`, `instagram_business_content_publish`.
- Requires a professional (Business or Creator) account and Meta app review.
- API publishing cap per account per rolling 24 hours: Meta pages list 50 or 100 (**verify** with `GET /<IG_ID>/content_publishing_limit`). Carousels count as one post.

## Sources

- [TikTok Content Posting API: Direct Post reference](https://developers.tiktok.com/doc/content-posting-api-reference-direct-post)
- [TikTok content sharing guidelines](https://developers.tiktok.com/docs/en/content-sharing-guidelines)
- [Instagram content publishing](https://developers.facebook.com/docs/instagram-platform/content-publishing/)
- [Instagram content publishing limit](https://developers.facebook.com/docs/instagram-platform/instagram-graph-api/reference/ig-user/content_publishing_limit)
- [YouTube API quota changes (third-party summary)](https://postproxy.dev/blog/youtube-upload-api-guide/)
- [Is the YouTube API free in 2026? (third-party)](https://www.getphyllo.com/post/is-the-youtube-api-free-in-2026-quota-limits-costs-when-to-pay)
