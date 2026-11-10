package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"contentfactory/internal/launchpad"
)

// SavedNicheRepository stores saved niche ideas.
type SavedNicheRepository struct {
	pool *pgxpool.Pool
}

// NewSavedNicheRepository builds a SavedNicheRepository.
func NewSavedNicheRepository(pool *pgxpool.Pool) *SavedNicheRepository {
	return &SavedNicheRepository{pool: pool}
}

// Create saves a niche idea.
func (r *SavedNicheRepository) Create(ctx context.Context, ownerID, name, notes string, data json.RawMessage) (launchpad.SavedNiche, error) {
	name = strings.TrimSpace(name)
	if len(data) == 0 {
		data = json.RawMessage(`{}`)
	}
	var n launchpad.SavedNiche
	err := r.pool.QueryRow(ctx, `INSERT INTO saved_niches (owner_id, name, notes, data) VALUES ($1, $2, $3, $4)
		RETURNING id::text, name, notes, data, created_at`, ownerID, name, notes, data).
		Scan(&n.ID, &n.Name, &n.Notes, &n.Data, &n.CreatedAt)
	if isUniqueViolation(err) {
		return launchpad.SavedNiche{}, launchpad.ErrSavedNicheExists
	}
	if err != nil {
		return launchpad.SavedNiche{}, fmt.Errorf("insert saved niche: %w", err)
	}
	return n, nil
}

// List returns at most 200 saved niches, newest first.
func (r *SavedNicheRepository) List(ctx context.Context, ownerID string) ([]launchpad.SavedNiche, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text, name, notes, data, created_at FROM saved_niches
		WHERE owner_id = $1 ORDER BY created_at DESC LIMIT 200`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list saved niches: %w", err)
	}
	defer rows.Close()
	out := []launchpad.SavedNiche{}
	for rows.Next() {
		var n launchpad.SavedNiche
		if err := rows.Scan(&n.ID, &n.Name, &n.Notes, &n.Data, &n.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan saved niche: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// Delete removes a saved niche.
func (r *SavedNicheRepository) Delete(ctx context.Context, ownerID, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM saved_niches WHERE id = $1 AND owner_id = $2`, id, ownerID)
	if isInvalidUUID(err) {
		return launchpad.ErrSavedNicheNotFound
	}
	if err != nil {
		return fmt.Errorf("delete saved niche: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return launchpad.ErrSavedNicheNotFound
	}
	return nil
}
