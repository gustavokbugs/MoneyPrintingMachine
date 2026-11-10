-- YouTube integration and trend intelligence: encrypted OAuth tokens,
-- single-use OAuth states, API quota accounting, and trend snapshots.

ALTER TABLE platform_connections
    ADD COLUMN access_token_enc  bytea,
    ADD COLUMN refresh_token_enc bytea,
    ADD COLUMN token_expires_at  timestamptz,
    ADD COLUMN granted_scopes    text[] NOT NULL DEFAULT '{}',
    ADD COLUMN verified_at       timestamptz;

-- One external account maps to at most one internal channel per owner.
CREATE UNIQUE INDEX platform_connections_external_idx
    ON platform_connections (owner_id, platform, external_id)
    WHERE external_id IS NOT NULL;

-- Pending OAuth authorizations. Only a SHA-256 hash of the state value is
-- stored; the PKCE verifier is encrypted. Rows are deleted when consumed.
CREATE TABLE oauth_states (
    state_hash        text PRIMARY KEY,
    owner_id          text NOT NULL,
    channel_id        uuid NOT NULL REFERENCES channels (id) ON DELETE CASCADE,
    platform          text NOT NULL,
    code_verifier_enc bytea NOT NULL,
    expires_at        timestamptz NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX oauth_states_expires_idx ON oauth_states (expires_at);

-- Units consumed per provider per quota day (YouTube resets at midnight
-- Pacific time; the application computes the day).
CREATE TABLE api_quota_usage (
    provider  text NOT NULL,
    usage_day date NOT NULL,
    units     integer NOT NULL DEFAULT 0 CHECK (units >= 0),
    PRIMARY KEY (provider, usage_day)
);

-- Point-in-time measurements from trend sources. Momentum is computed by
-- comparing a new snapshot with earlier ones for the same query.
CREATE TABLE trend_snapshots (
    id           bigserial PRIMARY KEY,
    owner_id     text NOT NULL,
    source       text NOT NULL,
    query_key    text NOT NULL,
    query        text NOT NULL,
    region       text NOT NULL,
    language     text NOT NULL,
    metrics      jsonb NOT NULL,
    collected_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX trend_snapshots_lookup_idx
    ON trend_snapshots (owner_id, source, query_key, region, language, collected_at DESC);
