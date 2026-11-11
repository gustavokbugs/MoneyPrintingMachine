package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// CostUpload is the quota reserved per upload. Google changed the
// videos.insert cost during 2025-2026; 1600 units is the documented
// historical cost and is reserved conservatively.
const CostUpload = 1600

// DefaultUploadBaseURL is the resumable upload endpoint root.
const DefaultUploadBaseURL = "https://www.googleapis.com/upload/youtube/v3"

// UploadMeta is the video metadata sent with an upload.
type UploadMeta struct {
	Title           string
	Description     string
	Tags            []string
	CategoryID      string
	Language        string
	Privacy         string // "private", "unlisted", or "public"
	SyntheticMedia  bool   // discloses altered or synthetic content
	MadeForKids     bool
	PublishAtRFC339 string // optional scheduled publish time (requires private)
}

// Uploader performs resumable uploads with an OAuth access token.
type Uploader struct {
	http    *http.Client
	baseURL string
	quota   Quota
}

// NewUploader builds an Uploader. httpClient should have no overall
// timeout (uploads can be long); cancellation uses the context.
func NewUploader(httpClient *http.Client, baseURL string, quota Quota) *Uploader {
	if baseURL == "" {
		baseURL = DefaultUploadBaseURL
	}
	return &Uploader{http: httpClient, baseURL: strings.TrimRight(baseURL, "/"), quota: quota}
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// sanitizeTitle enforces YouTube's title rules: at most 100 characters and
// no angle brackets.
func sanitizeTitle(s string) string {
	s = strings.NewReplacer("<", "", ">", "").Replace(strings.Join(strings.Fields(s), " "))
	return truncateRunes(s, 100)
}

// Upload sends a video file and returns the new video ID.
func (u *Uploader) Upload(ctx context.Context, accessToken string, meta UploadMeta, path string) (string, error) {
	switch meta.Privacy {
	case "private", "unlisted", "public":
	default:
		return "", fmt.Errorf("invalid privacy status %q", meta.Privacy)
	}
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open video: %w", err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.Size() == 0 {
		return "", fmt.Errorf("video file is empty or unreadable")
	}
	if u.quota != nil {
		if err := u.quota.Reserve(ctx, CostUpload); err != nil {
			return "", err
		}
	}

	status := map[string]any{
		"privacyStatus":           meta.Privacy,
		"selfDeclaredMadeForKids": meta.MadeForKids,
		"containsSyntheticMedia":  meta.SyntheticMedia,
	}
	if meta.PublishAtRFC339 != "" {
		status["privacyStatus"] = "private"
		status["publishAt"] = meta.PublishAtRFC339
	}
	tags := []string{}
	total := 0
	for _, t := range meta.Tags {
		t = strings.TrimSpace(strings.NewReplacer("<", "", ">", "", ",", " ").Replace(t))
		if t == "" || total+len(t) > 450 {
			continue
		}
		total += len(t) + 1
		tags = append(tags, t)
	}
	body, _ := json.Marshal(map[string]any{
		"snippet": map[string]any{
			"title":                sanitizeTitle(meta.Title),
			"description":          truncateRunes(strings.NewReplacer("<", "", ">", "").Replace(meta.Description), 5000),
			"tags":                 tags,
			"categoryId":           meta.CategoryID,
			"defaultLanguage":      meta.Language,
			"defaultAudioLanguage": meta.Language,
		},
		"status": status,
	})
	q := url.Values{"uploadType": {"resumable"}, "part": {"snippet,status"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.baseURL+"/videos?"+q.Encode(), bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build upload request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("X-Upload-Content-Type", "video/mp4")
	req.Header.Set("X-Upload-Content-Length", strconv.FormatInt(st.Size(), 10))
	resp, err := u.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: start upload: %v", ErrUnavailable, err)
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", classifyError(resp.StatusCode, b)
	}
	session := resp.Header.Get("Location")
	if session == "" {
		return "", fmt.Errorf("%w: upload session URL missing", ErrUnavailable)
	}

	put, err := http.NewRequestWithContext(ctx, http.MethodPut, session, f)
	if err != nil {
		return "", fmt.Errorf("build upload body: %w", err)
	}
	put.ContentLength = st.Size()
	put.Header.Set("Content-Type", "video/mp4")
	resp, err = u.http.Do(put)
	if err != nil {
		return "", fmt.Errorf("%w: send video: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	b, _ = io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", classifyError(resp.StatusCode, b)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(b, &out); err != nil || out.ID == "" {
		return "", fmt.Errorf("%w: upload response without video ID", ErrUnavailable)
	}
	return out.ID, nil
}
