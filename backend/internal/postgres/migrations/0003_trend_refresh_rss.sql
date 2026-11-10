-- Scheduled trend re-measurement (watchlist) and RSS as a trend source.

-- Queries re-measured on a schedule so momentum builds over time. Rows
-- with source 'channel' mirror the niches of non-archived channels; rows
-- with source 'manual' are added by the user.
CREATE TABLE trend_watchlist (
    id               bigserial PRIMARY KEY,
    owner_id         text NOT NULL,
    query_key        text NOT NULL,
    query            text NOT NULL,
    region           text NOT NULL,
    language         text NOT NULL,
    source           text NOT NULL CHECK (source IN ('channel', 'manual')),
    active           boolean NOT NULL DEFAULT true,
    next_due_at      timestamptz NOT NULL DEFAULT now(),
    last_measured_at timestamptz,
    last_error       text NOT NULL DEFAULT '',
    created_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (owner_id, query_key, region, language)
);

-- Claim query: active rows ordered by due time.
CREATE INDEX trend_watchlist_due_idx ON trend_watchlist (next_due_at) WHERE active;

CREATE TABLE rss_feeds (
    id              bigserial PRIMARY KEY,
    owner_id        text NOT NULL,
    url             text NOT NULL,
    title           text NOT NULL DEFAULT '',
    active          boolean NOT NULL DEFAULT true,
    etag            text NOT NULL DEFAULT '',
    last_modified   text NOT NULL DEFAULT '',
    next_fetch_at   timestamptz NOT NULL DEFAULT now(),
    last_fetched_at timestamptz,
    last_status     text NOT NULL DEFAULT 'pending' CHECK (last_status IN ('pending', 'ok', 'not_modified', 'error')),
    last_error      text NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (owner_id, url)
);

CREATE INDEX rss_feeds_due_idx ON rss_feeds (next_fetch_at) WHERE active;

-- Feed entries. Only metadata is stored (title, link, date), never full
-- article text. guid_hash deduplicates entries per feed.
CREATE TABLE rss_items (
    id           bigserial PRIMARY KEY,
    feed_id      bigint NOT NULL REFERENCES rss_feeds (id) ON DELETE CASCADE,
    owner_id     text NOT NULL,
    guid_hash    text NOT NULL,
    title        text NOT NULL,
    link         text NOT NULL DEFAULT '',
    published_at timestamptz NOT NULL,
    fetched_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (feed_id, guid_hash)
);

-- Mention counts scan an owner's recent items.
CREATE INDEX rss_items_owner_published_idx ON rss_items (owner_id, published_at DESC);
