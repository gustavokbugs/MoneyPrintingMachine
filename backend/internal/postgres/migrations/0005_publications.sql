-- One upload attempt per video and platform. A failed attempt may be retried;
-- a successful one is never repeated automatically.

CREATE TABLE publications (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id    text NOT NULL,
    video_id    uuid NOT NULL REFERENCES videos (id) ON DELETE CASCADE,
    platform    text NOT NULL CHECK (platform IN ('youtube', 'tiktok', 'instagram')),
    status      text NOT NULL CHECK (status IN ('uploading', 'published', 'failed')),
    privacy     text NOT NULL CHECK (privacy IN ('private', 'unlisted', 'public')),
    external_id text NOT NULL DEFAULT '',
    url         text NOT NULL DEFAULT '',
    error       text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (video_id, platform)
);
