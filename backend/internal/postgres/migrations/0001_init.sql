-- Initial schema: channels, lifecycle events, platform connections,
-- saved niche ideas, and durable jobs.

CREATE TABLE channels (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id      text NOT NULL,
    name          text NOT NULL DEFAULT '',
    status        text NOT NULL CHECK (status IN (
                      'draft', 'branding', 'awaiting_platform_setup', 'ready',
                      'active', 'paused', 'error', 'archived')),
    status_reason text NOT NULL DEFAULT '',
    niche         text NOT NULL DEFAULT '',
    language      text NOT NULL DEFAULT 'en-US',
    geo_target    text NOT NULL DEFAULT 'US',
    strategy      jsonb,
    identity      jsonb,
    blueprint     jsonb,
    version       integer NOT NULL DEFAULT 1,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

-- Supports the owner's channel list ordered newest first (keyset pagination).
CREATE INDEX channels_owner_created_idx ON channels (owner_id, created_at DESC, id DESC);

CREATE TABLE channel_status_events (
    id          bigserial PRIMARY KEY,
    channel_id  uuid NOT NULL REFERENCES channels (id) ON DELETE CASCADE,
    from_status text NOT NULL,
    to_status   text NOT NULL,
    reason      text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX channel_status_events_channel_idx ON channel_status_events (channel_id, created_at DESC);

-- External platform accounts linked to a channel. A row only reaches
-- 'verified' after the platform's own API confirmed the account identity
-- and the granted permissions. Tokens are stored encrypted (added with the
-- OAuth integration); this table never stores passwords.
CREATE TABLE platform_connections (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id      text NOT NULL,
    channel_id    uuid NOT NULL REFERENCES channels (id) ON DELETE CASCADE,
    platform      text NOT NULL CHECK (platform IN ('youtube', 'tiktok', 'instagram')),
    status        text NOT NULL CHECK (status IN (
                      'pending_authorization', 'authorized', 'verified', 'revoked', 'error')),
    external_id   text,
    external_name text,
    can_publish   boolean NOT NULL DEFAULT false,
    can_read_analytics boolean NOT NULL DEFAULT false,
    status_reason text NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (channel_id, platform)
);

CREATE INDEX platform_connections_owner_channel_idx ON platform_connections (owner_id, channel_id);

CREATE TABLE saved_niches (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id   text NOT NULL,
    name       text NOT NULL,
    notes      text NOT NULL DEFAULT '',
    data       jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX saved_niches_owner_name_idx ON saved_niches (owner_id, lower(name));

CREATE TABLE jobs (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id        text NOT NULL,
    kind            text NOT NULL,
    channel_id      uuid REFERENCES channels (id) ON DELETE CASCADE,
    status          text NOT NULL CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
    input           jsonb NOT NULL DEFAULT '{}'::jsonb,
    result          jsonb,
    error           text NOT NULL DEFAULT '',
    attempts        integer NOT NULL DEFAULT 0,
    max_attempts    integer NOT NULL DEFAULT 3,
    idempotency_key text,
    run_after       timestamptz NOT NULL DEFAULT now(),
    locked_until    timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX jobs_owner_idempotency_idx ON jobs (owner_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
-- Claim query scans queued jobs by run_after and expired leases by locked_until.
CREATE INDEX jobs_queued_idx ON jobs (run_after) WHERE status = 'queued';
CREATE INDEX jobs_running_lease_idx ON jobs (locked_until) WHERE status = 'running';
CREATE INDEX jobs_channel_idx ON jobs (channel_id, created_at DESC) WHERE channel_id IS NOT NULL;
