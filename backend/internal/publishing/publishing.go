// Package publishing uploads produced videos to connected platforms.
// Uploads are explicit user actions: each starts from a confirmed request,
// defaults to private, and a successful upload is never repeated
// automatically, so retries cannot create duplicate public videos.
package publishing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"contentfactory/internal/channel"
	"contentfactory/internal/jobs"
	"contentfactory/internal/production"
	"contentfactory/internal/youtube"
)

// KindYouTube is the job kind for a YouTube upload.
const KindYouTube = "publishing.youtube"

// Publication statuses.
const (
	StatusUploading = "uploading"
	StatusPublished = "published"
	StatusFailed    = "failed"
)

var (
	// ErrAlreadyPublished blocks a second upload of the same video.
	ErrAlreadyPublished = errors.New("this video was already uploaded to YouTube")
	// ErrInProgress blocks concurrent uploads of the same video.
	ErrInProgress = errors.New("an upload of this video is already in progress")
	// ErrNotProduced means there is no rendered file to upload.
	ErrNotProduced = errors.New("produce the video before publishing it")
)

// Publication records one upload of a video to a platform.
type Publication struct {
	ID         string    `json:"id"`
	OwnerID    string    `json:"-"`
	VideoID    string    `json:"videoId"`
	Platform   string    `json:"platform"`
	Status     string    `json:"status"`
	Privacy    string    `json:"privacy"`
	ExternalID string    `json:"externalId"`
	URL        string    `json:"url"`
	Error      string    `json:"error"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// Store persists publications.
type Store interface {
	// Begin creates or reuses the publication for a video/platform and marks
	// it uploading. It returns ErrAlreadyPublished or ErrInProgress when an
	// upload succeeded before or is running.
	Begin(ctx context.Context, ownerID, videoID, platform, privacy string) (*Publication, error)
	Finish(ctx context.Context, id, status, externalID, url, errMsg string) error
	ListForVideo(ctx context.Context, ownerID, videoID string) ([]Publication, error)
}

// Videos reads videos and their files.
type Videos interface {
	Get(ctx context.Context, ownerID, id string) (*production.Video, error)
	MediaPath(ctx context.Context, ownerID, id, name string) (string, error)
}

// Channels reads channels.
type Channels interface {
	Get(ctx context.Context, ownerID, id string) (*channel.Channel, error)
}

// Tokens provides publishing access tokens.
type Tokens interface {
	AccessToken(ctx context.Context, ownerID, channelID string) (string, error)
}

// Uploader uploads a file to YouTube.
type Uploader interface {
	Upload(ctx context.Context, accessToken string, meta youtube.UploadMeta, path string) (string, error)
}

// Service implements publishing.
type Service struct {
	store    Store
	videos   Videos
	channels Channels
	tokens   Tokens
	uploader Uploader
}

// NewService builds a Service.
func NewService(store Store, videos Videos, channels Channels, tokens Tokens, uploader Uploader) *Service {
	return &Service{store: store, videos: videos, channels: channels, tokens: tokens, uploader: uploader}
}

// ValidPrivacy reports whether p is a YouTube privacy status.
func ValidPrivacy(p string) bool { return p == "private" || p == "unlisted" || p == "public" }

// Check validates that a video can be published before a job is queued.
func (s *Service) Check(ctx context.Context, ownerID, videoID, privacy string) (*production.Video, error) {
	if !ValidPrivacy(privacy) {
		return nil, &channel.ValidationError{Field: "privacy", Message: "must be private, unlisted, or public"}
	}
	v, err := s.videos.Get(ctx, ownerID, videoID)
	if err != nil {
		return nil, err
	}
	if v.Status != production.StatusProduced || v.Output == nil {
		return nil, ErrNotProduced
	}
	if _, err := s.tokens.AccessToken(ctx, ownerID, v.ChannelID); err != nil {
		return nil, err
	}
	for _, p := range s.mustList(ctx, ownerID, videoID) {
		if p.Platform == "youtube" && p.Status == StatusPublished {
			return nil, ErrAlreadyPublished
		}
		if p.Platform == "youtube" && p.Status == StatusUploading {
			return nil, ErrInProgress
		}
	}
	return v, nil
}

func (s *Service) mustList(ctx context.Context, ownerID, videoID string) []Publication {
	list, err := s.store.ListForVideo(ctx, ownerID, videoID)
	if err != nil {
		return nil
	}
	return list
}

// List returns a video's publications.
func (s *Service) List(ctx context.Context, ownerID, videoID string) ([]Publication, error) {
	if _, err := s.videos.Get(ctx, ownerID, videoID); err != nil {
		return nil, err
	}
	return s.store.ListForVideo(ctx, ownerID, videoID)
}

// Description builds the video description: channel bio, the hook, the
// sources to check, stock footage credits, and an AI-voice disclosure.
func Description(ch *channel.Channel, v *production.Video) string {
	var parts []string
	if v.Script != nil && v.Script.Hook != "" {
		parts = append(parts, v.Script.Hook)
	}
	if ch.Identity != nil && ch.Identity.ShortBio != "" {
		parts = append(parts, ch.Identity.ShortBio)
	}
	if v.Script != nil {
		var sources []string
		seen := map[string]bool{}
		for _, c := range v.Script.Claims {
			if c.Status == production.ClaimVerified && c.SourceHint != "" && !seen[c.SourceHint] {
				seen[c.SourceHint] = true
				sources = append(sources, "- "+c.SourceHint)
			}
		}
		if len(sources) > 0 {
			parts = append(parts, "Sources:\n"+strings.Join(sources, "\n"))
		}
	}
	if v.Output != nil && len(v.Output.Footage) > 0 {
		// Pexels asks for credit to the platform and its creators.
		credits := []string{"Stock footage from Pexels (https://www.pexels.com):"}
		for _, c := range v.Output.Footage {
			credits = append(credits, "- "+c.Author+" "+c.PageURL)
		}
		parts = append(parts, strings.Join(credits, "\n"))
	}
	if v.Output != nil && v.Output.SyntheticVoice {
		parts = append(parts, "Narration uses a synthetic (AI-generated) voice.")
	}
	return strings.Join(parts, "\n\n")
}

type uploadInput struct {
	VideoID string `json:"videoId"`
	Privacy string `json:"privacy"`
}

// Handlers returns the publishing job handlers.
func (s *Service) Handlers() map[string]jobs.Handler {
	return map[string]jobs.Handler{KindYouTube: s.handleYouTube}
}

func permanent(err error) error {
	var ve *channel.ValidationError
	if errors.As(err, &ve) || errors.Is(err, ErrAlreadyPublished) || errors.Is(err, ErrInProgress) || errors.Is(err, ErrNotProduced) ||
		errors.Is(err, production.ErrNotFound) || errors.Is(err, channel.ErrNotFound) || errors.Is(err, youtube.ErrNotConfigured) {
		return jobs.Permanent(err)
	}
	return err
}

func (s *Service) handleYouTube(ctx context.Context, j jobs.Job) (any, error) {
	var in uploadInput
	if err := json.Unmarshal(j.Input, &in); err != nil || in.VideoID == "" || !ValidPrivacy(in.Privacy) {
		return nil, jobs.Permanent(errors.New("invalid publish job input"))
	}
	v, err := s.videos.Get(ctx, j.OwnerID, in.VideoID)
	if err != nil {
		return nil, permanent(err)
	}
	if v.Status != production.StatusProduced || v.Output == nil {
		return nil, jobs.Permanent(ErrNotProduced)
	}
	ch, err := s.channels.Get(ctx, j.OwnerID, v.ChannelID)
	if err != nil {
		return nil, permanent(err)
	}
	pub, err := s.store.Begin(ctx, j.OwnerID, v.ID, "youtube", in.Privacy)
	if err != nil {
		return nil, permanent(err)
	}
	finishFail := func(cause error) (any, error) {
		_ = s.store.Finish(context.WithoutCancel(ctx), pub.ID, StatusFailed, "", "", truncate(cause.Error(), 500))
		// Uploads are not retried automatically: a lost response after a
		// successful upload would otherwise create a duplicate video.
		return nil, jobs.Permanent(cause)
	}
	token, err := s.tokens.AccessToken(ctx, j.OwnerID, v.ChannelID)
	if err != nil {
		return finishFail(err)
	}
	path, err := s.videos.MediaPath(ctx, j.OwnerID, v.ID, "video.mp4")
	if err != nil {
		return finishFail(err)
	}
	var tags []string
	if ch.Identity != nil {
		tags = ch.Identity.Keywords
	}
	id, err := s.uploader.Upload(ctx, token, youtube.UploadMeta{
		Title: v.Title, Description: Description(ch, v), Tags: tags, CategoryID: "27",
		Language: ch.Language, Privacy: in.Privacy, SyntheticMedia: v.Output.SyntheticVoice,
	}, path)
	if err != nil {
		return finishFail(err)
	}
	url := "https://www.youtube.com/watch?v=" + id
	if err := s.store.Finish(ctx, pub.ID, StatusPublished, id, url, ""); err != nil {
		return nil, fmt.Errorf("record publication %s (video %s is uploaded): %w", pub.ID, id, err)
	}
	return map[string]string{"videoId": v.ID, "youtubeId": id, "url": url}, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
