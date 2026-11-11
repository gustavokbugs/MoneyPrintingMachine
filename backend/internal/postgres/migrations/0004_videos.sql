-- Production pipeline: one row per video, created from a blueprint idea.

CREATE TABLE videos (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id       text NOT NULL,
    channel_id     uuid NOT NULL REFERENCES channels (id) ON DELETE CASCADE,
    idea_index     integer NOT NULL CHECK (idea_index >= 0),
    title          text NOT NULL,
    format         text NOT NULL DEFAULT '',
    aspect_ratio   text NOT NULL CHECK (aspect_ratio IN ('16:9', '9:16')),
    target_seconds integer NOT NULL CHECK (target_seconds BETWEEN 15 AND 3600),
    status         text NOT NULL CHECK (status IN (
                       'planned', 'scripting', 'script_ready', 'approved', 'producing', 'produced', 'failed')),
    status_reason  text NOT NULL DEFAULT '',
    voice_id       integer NOT NULL DEFAULT 6,
    script         jsonb,
    output         jsonb,
    version        integer NOT NULL DEFAULT 1,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (channel_id, idea_index)
);

CREATE INDEX videos_owner_channel_idx ON videos (owner_id, channel_id, created_at DESC);
