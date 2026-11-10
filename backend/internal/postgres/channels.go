package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"contentfactory/internal/channel"
	"contentfactory/internal/platforms"
)

// ChannelRepository implements channel.Repository.
type ChannelRepository struct {
	pool *pgxpool.Pool
}

// NewChannelRepository builds a ChannelRepository.
func NewChannelRepository(pool *pgxpool.Pool) *ChannelRepository {
	return &ChannelRepository{pool: pool}
}

const channelColumns = `id::text, owner_id, name, status, status_reason, niche, language,
	geo_target, strategy, identity, blueprint, version, created_at, updated_at`

func scanChannel(row pgx.Row) (*channel.Channel, error) {
	var (
		c                   channel.Channel
		status              string
		strat, idn, blueprt []byte
	)
	if err := row.Scan(&c.ID, &c.OwnerID, &c.Name, &status, &c.StatusReason, &c.Niche,
		&c.Language, &c.GeoTarget, &strat, &idn, &blueprt, &c.Version, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	c.Status = channel.Status(status)
	if err := decodeOptional(strat, &c.Strategy); err != nil {
		return nil, fmt.Errorf("decode strategy: %w", err)
	}
	if err := decodeOptional(idn, &c.Identity); err != nil {
		return nil, fmt.Errorf("decode identity: %w", err)
	}
	if err := decodeOptional(blueprt, &c.Blueprint); err != nil {
		return nil, fmt.Errorf("decode blueprint: %w", err)
	}
	return &c, nil
}

func decodeOptional[T any](raw []byte, dst **T) error {
	if len(raw) == 0 {
		*dst = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	*dst = &v
	return nil
}

func encodeOptional[T any](v *T) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}

// Create inserts a new channel and fills generated fields.
func (r *ChannelRepository) Create(ctx context.Context, c *channel.Channel) error {
	row := r.pool.QueryRow(ctx, `INSERT INTO channels (owner_id, name, status, niche, language, geo_target)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id::text, version, created_at, updated_at`,
		c.OwnerID, c.Name, string(c.Status), c.Niche, c.Language, c.GeoTarget)
	if err := row.Scan(&c.ID, &c.Version, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return fmt.Errorf("insert channel: %w", err)
	}
	return nil
}

// Get loads a channel owned by ownerID.
func (r *ChannelRepository) Get(ctx context.Context, ownerID, id string) (*channel.Channel, error) {
	c, err := scanChannel(r.pool.QueryRow(ctx,
		`SELECT `+channelColumns+` FROM channels WHERE id = $1 AND owner_id = $2`, id, ownerID))
	if isNoRows(err) || isInvalidUUID(err) {
		return nil, channel.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get channel: %w", err)
	}
	return c, nil
}

type listCursor struct {
	CreatedAt time.Time `json:"t"`
	ID        string    `json:"i"`
}

func encodeCursor(c listCursor) string {
	raw, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeCursor(s string) (listCursor, error) {
	var c listCursor
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return c, &channel.ValidationError{Field: "cursor", Message: "is invalid"}
	}
	if err := json.Unmarshal(raw, &c); err != nil || c.ID == "" {
		return c, &channel.ValidationError{Field: "cursor", Message: "is invalid"}
	}
	return c, nil
}

// List returns channels newest first using keyset pagination.
func (r *ChannelRepository) List(ctx context.Context, ownerID string, page channel.Page) ([]channel.Channel, string, error) {
	args := []any{ownerID, page.Limit + 1}
	where := "owner_id = $1"
	if page.Cursor != "" {
		cur, err := decodeCursor(page.Cursor)
		if err != nil {
			return nil, "", err
		}
		where += " AND (created_at, id) < ($3, $4::uuid)"
		args = append(args, cur.CreatedAt, cur.ID)
	}
	rows, err := r.pool.Query(ctx, `SELECT `+channelColumns+` FROM channels WHERE `+where+
		` ORDER BY created_at DESC, id DESC LIMIT $2`, args...)
	if err != nil {
		if isInvalidUUID(err) {
			return nil, "", &channel.ValidationError{Field: "cursor", Message: "is invalid"}
		}
		return nil, "", fmt.Errorf("list channels: %w", err)
	}
	defer rows.Close()

	var out []channel.Channel
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			return nil, "", fmt.Errorf("scan channel: %w", err)
		}
		out = append(out, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterate channels: %w", err)
	}
	next := ""
	if len(out) > page.Limit {
		out = out[:page.Limit]
		last := out[len(out)-1]
		next = encodeCursor(listCursor{CreatedAt: last.CreatedAt, ID: last.ID})
	}
	return out, next, nil
}

// Save updates a channel with optimistic concurrency and records an
// optional status event in the same transaction.
func (r *ChannelRepository) Save(ctx context.Context, c *channel.Channel, ev *channel.StatusEvent) error {
	strat, err := encodeOptional(c.Strategy)
	if err != nil {
		return fmt.Errorf("encode strategy: %w", err)
	}
	idn, err := encodeOptional(c.Identity)
	if err != nil {
		return fmt.Errorf("encode identity: %w", err)
	}
	bp, err := encodeOptional(c.Blueprint)
	if err != nil {
		return fmt.Errorf("encode blueprint: %w", err)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin save channel: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var updatedAt time.Time
	err = tx.QueryRow(ctx, `UPDATE channels SET
			name = $1, status = $2, status_reason = $3, niche = $4, language = $5, geo_target = $6,
			strategy = $7, identity = $8, blueprint = $9, version = version + 1, updated_at = now()
		WHERE id = $10 AND owner_id = $11 AND version = $12
		RETURNING updated_at`,
		c.Name, string(c.Status), c.StatusReason, c.Niche, c.Language, c.GeoTarget,
		strat, idn, bp, c.ID, c.OwnerID, c.Version).Scan(&updatedAt)
	if isNoRows(err) {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM channels WHERE id = $1 AND owner_id = $2)`,
			c.ID, c.OwnerID).Scan(&exists); err != nil {
			return fmt.Errorf("check channel existence: %w", err)
		}
		if !exists {
			return channel.ErrNotFound
		}
		return channel.ErrConflict
	}
	if err != nil {
		return fmt.Errorf("update channel: %w", err)
	}
	if ev != nil {
		if _, err := tx.Exec(ctx, `INSERT INTO channel_status_events (channel_id, from_status, to_status, reason)
			VALUES ($1, $2, $3, $4)`, c.ID, string(ev.From), string(ev.To), ev.Reason); err != nil {
			return fmt.Errorf("insert status event: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit save channel: %w", err)
	}
	c.Version++
	c.UpdatedAt = updatedAt
	return nil
}

// PlatformConnectionRepository reads platform connection state.
type PlatformConnectionRepository struct {
	pool *pgxpool.Pool
}

// NewPlatformConnectionRepository builds a PlatformConnectionRepository.
func NewPlatformConnectionRepository(pool *pgxpool.Pool) *PlatformConnectionRepository {
	return &PlatformConnectionRepository{pool: pool}
}

// CountVerified counts connections confirmed by the platform.
func (r *PlatformConnectionRepository) CountVerified(ctx context.Context, ownerID, channelID string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM platform_connections
		WHERE owner_id = $1 AND channel_id = $2 AND status = 'verified'`, ownerID, channelID).Scan(&n)
	if isInvalidUUID(err) {
		return 0, channel.ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("count verified connections: %w", err)
	}
	return n, nil
}

// ListForChannel returns all connections of a channel.
func (r *PlatformConnectionRepository) ListForChannel(ctx context.Context, ownerID, channelID string) ([]platforms.Connection, error) {
	rows, err := r.pool.Query(ctx, `SELECT platform, status, coalesce(external_id, ''), coalesce(external_name, ''),
			can_publish, can_read_analytics, status_reason, updated_at
		FROM platform_connections WHERE owner_id = $1 AND channel_id = $2 ORDER BY platform`, ownerID, channelID)
	if err != nil {
		if isInvalidUUID(err) {
			return nil, channel.ErrNotFound
		}
		return nil, fmt.Errorf("list platform connections: %w", err)
	}
	defer rows.Close()
	out := []platforms.Connection{}
	for rows.Next() {
		var p platforms.Connection
		if err := rows.Scan(&p.Platform, &p.Status, &p.ExternalID, &p.ExternalName,
			&p.CanPublish, &p.CanReadAnalytics, &p.StatusReason, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan platform connection: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// VerifiedPlatforms returns the verified platforms of each channel in one
// query, keyed by channel ID.
func (r *PlatformConnectionRepository) VerifiedPlatforms(ctx context.Context, ownerID string, channelIDs []string) (map[string][]string, error) {
	out := map[string][]string{}
	if len(channelIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT channel_id::text, platform FROM platform_connections
		WHERE owner_id = $1 AND channel_id = ANY($2::uuid[]) AND status = 'verified'
		ORDER BY platform`, ownerID, channelIDs)
	if err != nil {
		return nil, fmt.Errorf("list verified platforms: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, platform string
		if err := rows.Scan(&id, &platform); err != nil {
			return nil, fmt.Errorf("scan verified platform: %w", err)
		}
		out[id] = append(out[id], platform)
	}
	return out, rows.Err()
}
