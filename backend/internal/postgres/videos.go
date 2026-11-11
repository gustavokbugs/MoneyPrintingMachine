package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"contentfactory/internal/production"
)

// VideoRepository implements production.Store.
type VideoRepository struct {
	pool *pgxpool.Pool
}

// NewVideoRepository builds a VideoRepository.
func NewVideoRepository(pool *pgxpool.Pool) *VideoRepository { return &VideoRepository{pool: pool} }

const videoColumns = `id::text, owner_id, channel_id::text, idea_index, title, format, aspect_ratio, target_seconds,
	status, status_reason, voice_id, script, output, version, created_at, updated_at`

func scanVideo(row pgx.Row) (*production.Video, error) {
	var (
		v              production.Video
		status         string
		script, output []byte
	)
	if err := row.Scan(&v.ID, &v.OwnerID, &v.ChannelID, &v.IdeaIndex, &v.Title, &v.Format, &v.AspectRatio, &v.TargetSeconds,
		&status, &v.StatusReason, &v.VoiceID, &script, &output, &v.Version, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, err
	}
	v.Status = production.Status(status)
	if len(script) > 0 {
		v.Script = &production.Script{}
		if err := json.Unmarshal(script, v.Script); err != nil {
			return nil, fmt.Errorf("decode script: %w", err)
		}
	}
	if len(output) > 0 {
		v.Output = &production.Output{}
		if err := json.Unmarshal(output, v.Output); err != nil {
			return nil, fmt.Errorf("decode output: %w", err)
		}
	}
	return &v, nil
}

// Create inserts a video.
func (r *VideoRepository) Create(ctx context.Context, v *production.Video) error {
	err := r.pool.QueryRow(ctx, `INSERT INTO videos (owner_id, channel_id, idea_index, title, format, aspect_ratio, target_seconds, status, voice_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id::text, version, created_at, updated_at`,
		v.OwnerID, v.ChannelID, v.IdeaIndex, v.Title, v.Format, v.AspectRatio, v.TargetSeconds, string(v.Status), v.VoiceID).
		Scan(&v.ID, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	if isUniqueViolation(err) {
		return production.ErrExists
	}
	if err != nil {
		return fmt.Errorf("insert video: %w", err)
	}
	return nil
}

// Get loads a video owned by ownerID.
func (r *VideoRepository) Get(ctx context.Context, ownerID, id string) (*production.Video, error) {
	v, err := scanVideo(r.pool.QueryRow(ctx, `SELECT `+videoColumns+` FROM videos WHERE id = $1 AND owner_id = $2`, id, ownerID))
	if isNoRows(err) || isInvalidUUID(err) {
		return nil, production.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get video: %w", err)
	}
	return v, nil
}

// ListByChannel returns a channel's videos ordered by idea.
func (r *VideoRepository) ListByChannel(ctx context.Context, ownerID, channelID string) ([]production.Video, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+videoColumns+` FROM videos WHERE owner_id = $1 AND channel_id = $2 ORDER BY idea_index LIMIT 200`, ownerID, channelID)
	if err != nil {
		if isInvalidUUID(err) {
			return []production.Video{}, nil
		}
		return nil, fmt.Errorf("list videos: %w", err)
	}
	defer rows.Close()
	out := []production.Video{}
	for rows.Next() {
		v, err := scanVideo(rows)
		if err != nil {
			return nil, fmt.Errorf("scan video: %w", err)
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

// Save updates a video with optimistic concurrency.
func (r *VideoRepository) Save(ctx context.Context, v *production.Video) error {
	var script, output []byte
	var err error
	if v.Script != nil {
		if script, err = json.Marshal(v.Script); err != nil {
			return fmt.Errorf("encode script: %w", err)
		}
	}
	if v.Output != nil {
		if output, err = json.Marshal(v.Output); err != nil {
			return fmt.Errorf("encode output: %w", err)
		}
	}
	tag, err := r.pool.Exec(ctx, `UPDATE videos SET status = $1, status_reason = $2, voice_id = $3, script = $4, output = $5,
			version = version + 1, updated_at = now()
		WHERE id = $6 AND owner_id = $7 AND version = $8`,
		string(v.Status), v.StatusReason, v.VoiceID, script, output, v.ID, v.OwnerID, v.Version)
	if err != nil {
		return fmt.Errorf("update video: %w", err)
	}
	if tag.RowsAffected() == 0 {
		if _, err := r.Get(ctx, v.OwnerID, v.ID); err != nil {
			return err
		}
		return production.ErrConflict
	}
	v.Version++
	return nil
}
