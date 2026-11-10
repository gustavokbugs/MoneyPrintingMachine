package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"contentfactory/internal/channel"
	"contentfactory/internal/connections"
)

// ConnectionStore implements connections.Store.
type ConnectionStore struct {
	pool *pgxpool.Pool
}

// NewConnectionStore builds a ConnectionStore.
func NewConnectionStore(pool *pgxpool.Pool) *ConnectionStore { return &ConnectionStore{pool: pool} }

// SaveState stores a pending OAuth authorization and removes expired ones.
func (s *ConnectionStore) SaveState(ctx context.Context, st connections.State) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM oauth_states WHERE expires_at < now()`); err != nil {
		return fmt.Errorf("purge expired oauth states: %w", err)
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO oauth_states (state_hash, owner_id, channel_id, platform, code_verifier_enc, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)`, st.StateHash, st.OwnerID, st.ChannelID, st.Platform, st.VerifierEnc, st.ExpiresAt)
	if isInvalidUUID(err) {
		return channel.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("insert oauth state: %w", err)
	}
	return nil
}

// ConsumeState deletes and returns a state if it exists and has not expired.
func (s *ConnectionStore) ConsumeState(ctx context.Context, stateHash string, now time.Time) (connections.State, error) {
	var st connections.State
	err := s.pool.QueryRow(ctx, `DELETE FROM oauth_states WHERE state_hash = $1
		RETURNING state_hash, owner_id, channel_id::text, platform, code_verifier_enc, expires_at`, stateHash).
		Scan(&st.StateHash, &st.OwnerID, &st.ChannelID, &st.Platform, &st.VerifierEnc, &st.ExpiresAt)
	if isNoRows(err) {
		return connections.State{}, connections.ErrStateInvalid
	}
	if err != nil {
		return connections.State{}, fmt.Errorf("consume oauth state: %w", err)
	}
	if !now.Before(st.ExpiresAt) {
		return connections.State{}, connections.ErrStateInvalid
	}
	return st, nil
}

// Get returns a connection record, or nil when none exists.
func (s *ConnectionStore) Get(ctx context.Context, ownerID, channelID, platform string) (*connections.Record, error) {
	var r connections.Record
	err := s.pool.QueryRow(ctx, `SELECT owner_id, channel_id::text, platform, status, coalesce(external_id, ''),
			coalesce(external_name, ''), can_publish, can_read_analytics, status_reason, access_token_enc,
			refresh_token_enc, token_expires_at, granted_scopes, verified_at, updated_at
		FROM platform_connections WHERE owner_id = $1 AND channel_id = $2 AND platform = $3`, ownerID, channelID, platform).
		Scan(&r.OwnerID, &r.ChannelID, &r.Platform, &r.Status, &r.ExternalID, &r.ExternalName, &r.CanPublish,
			&r.CanReadAnalytics, &r.StatusReason, &r.AccessTokenEnc, &r.RefreshTokenEnc, &r.TokenExpiresAt,
			&r.GrantedScopes, &r.VerifiedAt, &r.UpdatedAt)
	if isNoRows(err) || isInvalidUUID(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get connection: %w", err)
	}
	return &r, nil
}

// Upsert creates or replaces a connection.
func (s *ConnectionStore) Upsert(ctx context.Context, r *connections.Record) error {
	var externalID, externalName any
	if r.ExternalID != "" {
		externalID, externalName = r.ExternalID, r.ExternalName
	}
	scopes := r.GrantedScopes
	if scopes == nil {
		scopes = []string{}
	}
	err := s.pool.QueryRow(ctx, `INSERT INTO platform_connections (owner_id, channel_id, platform, status, external_id,
			external_name, can_publish, can_read_analytics, status_reason, access_token_enc, refresh_token_enc,
			token_expires_at, granted_scopes, verified_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (channel_id, platform) DO UPDATE SET
			status = EXCLUDED.status, external_id = EXCLUDED.external_id, external_name = EXCLUDED.external_name,
			can_publish = EXCLUDED.can_publish, can_read_analytics = EXCLUDED.can_read_analytics,
			status_reason = EXCLUDED.status_reason, access_token_enc = EXCLUDED.access_token_enc,
			refresh_token_enc = EXCLUDED.refresh_token_enc, token_expires_at = EXCLUDED.token_expires_at,
			granted_scopes = EXCLUDED.granted_scopes, verified_at = EXCLUDED.verified_at, updated_at = now()
		WHERE platform_connections.owner_id = EXCLUDED.owner_id
		RETURNING updated_at`,
		r.OwnerID, r.ChannelID, r.Platform, r.Status, externalID, externalName, r.CanPublish, r.CanReadAnalytics,
		r.StatusReason, r.AccessTokenEnc, r.RefreshTokenEnc, r.TokenExpiresAt, scopes, r.VerifiedAt).Scan(&r.UpdatedAt)
	if isUniqueViolation(err) {
		return connections.ErrExternalInUse
	}
	if isNoRows(err) || isInvalidUUID(err) {
		return channel.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("upsert connection: %w", err)
	}
	return nil
}

// Delete removes a connection.
func (s *ConnectionStore) Delete(ctx context.Context, ownerID, channelID, platform string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM platform_connections WHERE owner_id = $1 AND channel_id = $2 AND platform = $3`,
		ownerID, channelID, platform)
	if err != nil && !isInvalidUUID(err) {
		return fmt.Errorf("delete connection: %w", err)
	}
	return nil
}
