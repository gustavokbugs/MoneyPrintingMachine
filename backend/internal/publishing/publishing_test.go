package publishing

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"contentfactory/internal/channel"
	"contentfactory/internal/connections"
	"contentfactory/internal/jobs"
	"contentfactory/internal/production"
	"contentfactory/internal/youtube"
)

type memPubs struct{ items []Publication }

func (m *memPubs) Begin(_ context.Context, owner, vid, platform, privacy string) (*Publication, error) {
	for i := range m.items {
		p := &m.items[i]
		if p.VideoID == vid && p.Platform == platform {
			switch p.Status {
			case StatusPublished:
				return nil, ErrAlreadyPublished
			case StatusUploading:
				return nil, ErrInProgress
			}
			p.Status, p.Privacy = StatusUploading, privacy
			return p, nil
		}
	}
	m.items = append(m.items, Publication{ID: "p1", OwnerID: owner, VideoID: vid, Platform: platform, Status: StatusUploading, Privacy: privacy})
	return &m.items[len(m.items)-1], nil
}
func (m *memPubs) Finish(_ context.Context, id, status, ext, url, msg string) error {
	for i := range m.items {
		if m.items[i].ID == id {
			m.items[i].Status, m.items[i].ExternalID, m.items[i].URL, m.items[i].Error = status, ext, url, msg
		}
	}
	return nil
}
func (m *memPubs) ListForVideo(context.Context, string, string) ([]Publication, error) {
	return m.items, nil
}

type fakeVideos struct{ v *production.Video }

func (f fakeVideos) Get(_ context.Context, owner, id string) (*production.Video, error) {
	if owner != "alice" || id != f.v.ID {
		return nil, production.ErrNotFound
	}
	c := *f.v
	return &c, nil
}
func (f fakeVideos) MediaPath(context.Context, string, string, string) (string, error) {
	return "/media/video.mp4", nil
}

type fakeChannels struct{}

func (fakeChannels) Get(context.Context, string, string) (*channel.Channel, error) {
	return &channel.Channel{ID: "ch1", Language: "en-US", Identity: &channel.Identity{ShortBio: "Space, explained.", Keywords: []string{"moon"}}}, nil
}

type fakeTokens struct{ err error }

func (f fakeTokens) AccessToken(context.Context, string, string) (string, error) { return "at", f.err }

type fakeUploader struct {
	meta  youtube.UploadMeta
	err   error
	calls int
}

func (f *fakeUploader) Upload(_ context.Context, tok string, meta youtube.UploadMeta, path string) (string, error) {
	f.calls++
	f.meta = meta
	return "yt123", f.err
}

func producedVideo() *production.Video {
	return &production.Video{ID: "v1", ChannelID: "ch1", Title: "The Moon Is Leaving Us", Status: production.StatusProduced,
		Script: &production.Script{Hook: "The Moon is drifting away.", Claims: []production.Claim{
			{Text: "a", SourceHint: "NASA lunar laser ranging", Status: production.ClaimVerified},
			{Text: "b", SourceHint: "Unverified source", Status: production.ClaimRemoved}}},
		Output: &production.Output{SyntheticVoice: true}}
}

func job(privacy string) jobs.Job {
	in, _ := json.Marshal(map[string]string{"videoId": "v1", "privacy": privacy})
	return jobs.Job{OwnerID: "alice", Input: in}
}

func TestPublishOnceAndDisclose(t *testing.T) {
	pubs := &memPubs{}
	up := &fakeUploader{}
	svc := NewService(pubs, fakeVideos{producedVideo()}, fakeChannels{}, fakeTokens{}, up)
	ctx := context.Background()
	if _, err := svc.Check(ctx, "alice", "v1", "everyone"); err == nil {
		t.Fatal("invalid privacy must fail")
	}
	if _, err := svc.Check(ctx, "alice", "v1", "private"); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Handlers()[KindYouTube](ctx, job("private"))
	if err != nil {
		t.Fatal(err)
	}
	if res.(map[string]string)["url"] != "https://www.youtube.com/watch?v=yt123" || pubs.items[0].Status != StatusPublished {
		t.Fatalf("result %v pubs %+v", res, pubs.items)
	}
	if !up.meta.SyntheticMedia || up.meta.Privacy != "private" || !strings.Contains(up.meta.Description, "synthetic") ||
		!strings.Contains(up.meta.Description, "NASA lunar laser ranging") || strings.Contains(up.meta.Description, "Unverified source") {
		t.Fatalf("metadata: %+v", up.meta)
	}
	if _, err := svc.Check(ctx, "alice", "v1", "public"); !errors.Is(err, ErrAlreadyPublished) {
		t.Fatalf("second publish: %v", err)
	}
	if _, err := svc.Handlers()[KindYouTube](ctx, job("public")); !jobs.IsPermanent(err) || up.calls != 1 {
		t.Fatalf("duplicate upload must be blocked: %v calls=%d", err, up.calls)
	}
}

func TestUploadFailureIsRecordedAndNotRetried(t *testing.T) {
	pubs := &memPubs{}
	up := &fakeUploader{err: youtube.ErrUnavailable}
	svc := NewService(pubs, fakeVideos{producedVideo()}, fakeChannels{}, fakeTokens{}, up)
	_, err := svc.Handlers()[KindYouTube](context.Background(), job("unlisted"))
	if !jobs.IsPermanent(err) || pubs.items[0].Status != StatusFailed || pubs.items[0].Error == "" {
		t.Fatalf("failure: %v %+v", err, pubs.items)
	}
	// A failed attempt may be retried explicitly.
	up.err = nil
	if _, err := svc.Handlers()[KindYouTube](context.Background(), job("unlisted")); err != nil || pubs.items[0].Status != StatusPublished {
		t.Fatalf("explicit retry: %v %+v", err, pubs.items)
	}
}

func TestCheckRequirements(t *testing.T) {
	v := producedVideo()
	v.Status = production.StatusApproved
	svc := NewService(&memPubs{}, fakeVideos{v}, fakeChannels{}, fakeTokens{}, &fakeUploader{})
	if _, err := svc.Check(context.Background(), "alice", "v1", "private"); !errors.Is(err, ErrNotProduced) {
		t.Fatalf("unproduced: %v", err)
	}
	svc = NewService(&memPubs{}, fakeVideos{producedVideo()}, fakeChannels{}, fakeTokens{err: connections.ErrNotPublishable}, &fakeUploader{})
	if _, err := svc.Check(context.Background(), "alice", "v1", "private"); !errors.Is(err, connections.ErrNotPublishable) {
		t.Fatalf("no connection: %v", err)
	}
	if _, err := svc.Check(context.Background(), "bob", "v1", "private"); !errors.Is(err, production.ErrNotFound) {
		t.Fatalf("other owner: %v", err)
	}
}
