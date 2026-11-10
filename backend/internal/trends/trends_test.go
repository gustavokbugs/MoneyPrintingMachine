package trends

import (
	"context"
	"errors"
	"testing"
	"time"

	"contentfactory/internal/youtube"
)

var now = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

type fakeYT struct {
	key      bool
	calls    int
	videos   []youtube.Video
	channels []youtube.ChannelInfo
	err      error
	params   youtube.SearchParams
}

func (f *fakeYT) HasAPIKey() bool { return f.key }
func (f *fakeYT) SearchVideoIDs(_ context.Context, p youtube.SearchParams) ([]string, error) {
	f.calls++
	f.params = p
	if f.err != nil {
		return nil, f.err
	}
	ids := make([]string, len(f.videos))
	for i, v := range f.videos {
		ids[i] = v.ID
	}
	return ids, nil
}
func (f *fakeYT) Videos(context.Context, []string) ([]youtube.Video, error) { return f.videos, nil }
func (f *fakeYT) Channels(context.Context, []string) ([]youtube.ChannelInfo, error) {
	return f.channels, nil
}

type memStore struct{ snaps []Snapshot }

func (m *memStore) Latest(_ context.Context, _, source, key, region, lang string, before time.Time) (*Snapshot, error) {
	var best *Snapshot
	for i := range m.snaps {
		s := m.snaps[i]
		if s.Source == source && s.QueryKey == key && s.Region == region && s.Language == lang && !s.CollectedAt.After(before) {
			if best == nil || s.CollectedAt.After(best.CollectedAt) {
				best = &m.snaps[i]
			}
		}
	}
	if best == nil {
		return nil, nil
	}
	c := *best
	return &c, nil
}
func (m *memStore) Save(_ context.Context, _ string, s *Snapshot) error {
	s.ID = int64(len(m.snaps) + 1)
	m.snaps = append(m.snaps, *s)
	return nil
}
func (m *memStore) History(context.Context, string, string, string, string, string, int) ([]Snapshot, error) {
	return m.snaps, nil
}

func video(id, channel string, views, likes int64, ageDays int) youtube.Video {
	return youtube.Video{ID: id, ChannelID: channel, ViewCount: views, LikeCount: likes, PublishedAt: now.AddDate(0, 0, -ageDays)}
}

func TestComputeMetrics(t *testing.T) {
	videos := []youtube.Video{
		video("a", "c1", 1000, 100, 10), // 100/day, 10% engagement
		video("b", "c1", 300, 3, 3),     // 100/day, 1%
		video("c", "c2", 5000, 50, 5),   // 1000/day, 1%
		video("d", "c3", 0, 0, 0),       // brand new: age clamped to 1 day
	}
	channels := []youtube.ChannelInfo{
		{ID: "c1", SubscriberCount: 1000},
		{ID: "c2", SubscriberCount: 3000},
		{ID: "c3", HiddenSubscriber: true},
	}
	m := ComputeMetrics(videos, channels, now)
	if m.SampleSize != 4 || m.DistinctChannels != 3 || m.HiddenSubscriberChannels != 1 {
		t.Fatalf("unexpected counts: %+v", m)
	}
	if m.MedianViews != 650 || m.MedianViewsPerDay != 100 {
		t.Fatalf("medians: views=%d perDay=%v", m.MedianViews, m.MedianViewsPerDay)
	}
	if m.MedianEngagementRate != 0.01 {
		t.Fatalf("engagement = %v", m.MedianEngagementRate)
	}
	if m.TopChannelShare != 0.5 {
		t.Fatalf("top share = %v", m.TopChannelShare)
	}
	if m.MedianSubscribers == nil || *m.MedianSubscribers != 2000 {
		t.Fatalf("median subscribers = %v", m.MedianSubscribers)
	}
}

func TestComputeMetricsEmpty(t *testing.T) {
	m := ComputeMetrics(nil, nil, now)
	if m.SampleSize != 0 || m.MedianSubscribers != nil {
		t.Fatalf("unexpected: %+v", m)
	}
	s := ComputeScores(m, nil)
	if s.Demand == nil || *s.Demand != 0 || s.Competition != nil || s.Trend != nil {
		t.Fatalf("unexpected scores: %+v", s)
	}
}

func TestComputeScores(t *testing.T) {
	subs := int64(ReferenceSubscribers)
	m := Metrics{SampleSize: 10, MedianViewsPerDay: ReferenceViewsPerDay, MedianSubscribers: &subs}
	s := ComputeScores(m, nil)
	if *s.Demand != 100 || *s.Competition != 100 {
		t.Fatalf("reference points should score 100: %+v", s)
	}
	if s.MomentumPercent != nil || s.Trend != nil {
		t.Fatal("momentum requires a previous snapshot")
	}
	prev := Metrics{SampleSize: 10, MedianViewsPerDay: ReferenceViewsPerDay / 2}
	s = ComputeScores(m, &prev)
	if *s.MomentumPercent != 100 || *s.Trend != 100 {
		t.Fatalf("doubling should be +100%% and trend 100: %+v", s)
	}
	prev.MedianViewsPerDay = ReferenceViewsPerDay * 4
	s = ComputeScores(m, &prev)
	if *s.MomentumPercent != -75 || *s.Trend != 12.5 {
		t.Fatalf("drop: %+v", s)
	}
}

func TestMeasureCachesAndComputesMomentum(t *testing.T) {
	yt := &fakeYT{key: true, videos: []youtube.Video{video("a", "c1", 1000, 10, 10)}, channels: []youtube.ChannelInfo{{ID: "c1", SubscriberCount: 500}}}
	store := &memStore{}
	svc := NewService(yt, store)
	clock := now
	svc.now = func() time.Time { return clock }
	req := Request{OwnerID: "alice", Query: "  Space   History ", Region: "us", Language: "en-US"}

	r1, err := svc.Measure(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if r1.Cached || r1.Previous != nil || r1.Scores.Trend != nil || len(r1.Notes) == 0 {
		t.Fatalf("first measurement: %+v", r1)
	}
	if yt.params.Query != "Space History" || yt.params.RegionCode != "US" || yt.params.Language != "en" ||
		!yt.params.PublishedAfter.Equal(now.AddDate(0, 0, -WindowDays)) {
		t.Fatalf("unexpected search params: %+v", yt.params)
	}

	clock = now.Add(time.Hour)
	r2, err := svc.Measure(context.Background(), req)
	if err != nil || !r2.Cached || yt.calls != 1 {
		t.Fatalf("expected cached result without API call: cached=%v calls=%d err=%v", r2.Cached, yt.calls, err)
	}

	clock = now.Add(24 * time.Hour)
	// Published 10 days before the original clock, so 11 days old now: 200/day vs 100/day.
	yt.videos = []youtube.Video{video("a", "c1", 2200, 10, 10)}
	r3, err := svc.Measure(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if r3.Cached || r3.Previous == nil || r3.Scores.MomentumPercent == nil || *r3.Scores.MomentumPercent != 100 {
		t.Fatalf("expected +100%% momentum: %+v", r3.Scores)
	}
	if len(store.snaps) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(store.snaps))
	}
}

func TestMeasureErrors(t *testing.T) {
	svc := NewService(&fakeYT{key: false}, &memStore{})
	if _, err := svc.Measure(context.Background(), Request{Query: "x"}); !errors.Is(err, youtube.ErrNotConfigured) {
		t.Fatalf("expected not configured, got %v", err)
	}
	svc = NewService(&fakeYT{key: true}, &memStore{})
	if _, err := svc.Measure(context.Background(), Request{Query: "   "}); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("expected invalid query, got %v", err)
	}
	svc = NewService(&fakeYT{key: true, err: youtube.ErrQuotaBudget}, &memStore{})
	if _, err := svc.Measure(context.Background(), Request{Query: "x"}); !errors.Is(err, youtube.ErrQuotaBudget) {
		t.Fatalf("expected quota error to propagate, got %v", err)
	}
}
