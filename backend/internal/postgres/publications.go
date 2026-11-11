package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"contentfactory/internal/publishing"
)

// PublicationRepository implements publishing.Store.
type PublicationRepository struct {
	pool *pgxpool.Pool
}

// NewPublicationRepository builds a PublicationRepository.
func NewPublicationRepository(pool *pgxpool.Pool) *PublicationRepository {
	return &PublicationRepository{pool: pool}
}

const publicationColumns = `id::text, owner_id, video_id::text, platform, status, privacy, external_id, url, error, created_at, updated_at`

// Begin creates the publication or re-arms a failed one, atomically
// refusing when an upload succeeded or is in progress.
func (r *PublicationRepository) Begin(ctx context.Context, ownerID, videoID, platform, privacy string) (*publishing.Publication, error) {
	var p publishing.Publication
	err := r.pool.QueryRow(ctx, `INSERT INTO publications (owner_id, video_id, platform, status, privacy)
		VALUES ($1, $2, $3, 'uploading', $4)
		ON CONFLICT (video_id, platform) DO UPDATE SET status = 'uploading', privacy = EXCLUDED.privacy, error = '', updated_at = now()
		WHERE publications.status = 'failed' AND publications.owner_id = EXCLUDED.owner_id
		RETURNING `+publicationColumns, ownerID, videoID, platform, privacy).
		Scan(&p.ID, &p.OwnerID, &p.VideoID, &p.Platform, &p.Status, &p.Privacy, &p.ExternalID, &p.URL, &p.Error, &p.CreatedAt, &p.UpdatedAt)
	if isNoRows(err) {
		var status string
		if err := r.pool.QueryRow(ctx, `SELECT status FROM publications WHERE video_id = $1 AND platform = $2`, videoID, platform).Scan(&status); err != nil {
			return nil, fmt.Errorf("read publication: %w", err)
		}
		if status == publishing.StatusPublished {
			return nil, publishing.ErrAlreadyPublished
		}
		return nil, publishing.ErrInProgress
	}
	if err != nil {
		return nil, fmt.Errorf("begin publication: %w", err)
	}
	return &p, nil
}

// Finish records the outcome of an upload.
func (r *PublicationRepository) Finish(ctx context.Context, id, status, externalID, url, errMsg string) error {
	_, err := r.pool.Exec(ctx, `UPDATE publications SET status = $2, external_id = $3, url = $4, error = $5, updated_at = now() WHERE id = $1`,
		id, status, externalID, url, errMsg)
	if err != nil {
		return fmt.Errorf("finish publication: %w", err)
	}
	return nil
}

// ListForVideo returns a video's publications.
func (r *PublicationRepository) ListForVideo(ctx context.Context, ownerID, videoID string) ([]publishing.Publication, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+publicationColumns+` FROM publications WHERE owner_id = $1 AND video_id = $2 ORDER BY created_at`, ownerID, videoID)
	if err != nil {
		if isInvalidUUID(err) {
			return []publishing.Publication{}, nil
		}
		return nil, fmt.Errorf("list publications: %w", err)
	}
	defer rows.Close()
	out := []publishing.Publication{}
	for rows.Next() {
		var p publishing.Publication
		if err := rows.Scan(&p.ID, &p.OwnerID, &p.VideoID, &p.Platform, &p.Status, &p.Privacy, &p.ExternalID, &p.URL, &p.Error, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan publication: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
