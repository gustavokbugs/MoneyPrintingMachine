package youtube

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type countingQuota struct {
	used  int
	limit int
}

func (q *countingQuota) Reserve(_ context.Context, units int) error {
	if q.used+units > q.limit {
		return ErrQuotaBudget
	}
	q.used += units
	return nil
}

func TestSearchAndVideos(t *testing.T) {
	var searchQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			searchQuery = r.URL.Query()
			_, _ = w.Write([]byte(`{"items":[{"id":{"videoId":"v1"}},{"id":{"videoId":"v2"}},{"id":{}}]}`))
		case "/videos":
			if r.URL.Query().Get("id") != "v1,v2" {
				t.Errorf("unexpected ids %q", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`{"items":[
				{"id":"v1","snippet":{"channelId":"c1","publishedAt":"2026-11-01T00:00:00Z"},"statistics":{"viewCount":"1000","likeCount":"50","commentCount":"5"}},
				{"id":"v2","snippet":{"channelId":"c2","publishedAt":"2026-11-05T00:00:00Z"},"statistics":{"viewCount":"200"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	quota := &countingQuota{limit: 1000}
	c := NewDataClient(srv.Client(), srv.URL, "test-key", quota)

	ids, err := c.SearchVideoIDs(context.Background(), SearchParams{
		Query: "space history", RegionCode: "US", Language: "en",
		PublishedAfter: time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Fatalf("ids = %v", ids)
	}
	for k, want := range map[string]string{"type": "video", "order": "viewCount", "regionCode": "US", "relevanceLanguage": "en", "key": "test-key", "maxResults": "50"} {
		if got := searchQuery.Get(k); got != want {
			t.Errorf("search %s = %q, want %q", k, got, want)
		}
	}
	videos, err := c.Videos(context.Background(), ids)
	if err != nil {
		t.Fatal(err)
	}
	if videos[0].ViewCount != 1000 || videos[0].LikeCount != 50 || videos[1].LikeCount != 0 || videos[1].ChannelID != "c2" {
		t.Fatalf("unexpected videos: %+v", videos)
	}
	if quota.used != CostSearch+CostVideos {
		t.Fatalf("quota used = %d", quota.used)
	}
}

func TestQuotaBudgetBlocksCall(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { called = true }))
	defer srv.Close()
	c := NewDataClient(srv.Client(), srv.URL, "k", &countingQuota{limit: 50})
	_, err := c.SearchVideoIDs(context.Background(), SearchParams{Query: "x"})
	if !errors.Is(err, ErrQuotaBudget) {
		t.Fatalf("expected budget error, got %v", err)
	}
	if called {
		t.Fatal("request must not be sent when the budget is exhausted")
	}
}

func TestErrorClassification(t *testing.T) {
	tests := []struct {
		status int
		body   string
		want   error
	}{
		{403, `{"error":{"message":"quota","errors":[{"reason":"quotaExceeded"}]}}`, ErrQuotaExceeded},
		{400, `{"error":{"message":"bad key","errors":[{"reason":"keyInvalid"}]}}`, ErrUnauthorized},
		{401, `{"error":{"message":"expired"}}`, ErrUnauthorized},
		{503, `oops`, ErrUnavailable},
	}
	for _, tt := range tests {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tt.status)
			_, _ = w.Write([]byte(tt.body))
		}))
		c := NewDataClient(srv.Client(), srv.URL, "secret-key", nil)
		_, err := c.Channels(context.Background(), []string{"c1"})
		srv.Close()
		if !errors.Is(err, tt.want) {
			t.Errorf("status %d: got %v, want %v", tt.status, err, tt.want)
		}
		if err != nil && strings.Contains(err.Error(), "secret-key") {
			t.Errorf("error leaks API key: %v", err)
		}
	}
}

func TestTransportErrorDoesNotLeakKey(t *testing.T) {
	c := NewDataClient(&http.Client{Timeout: time.Second}, "http://127.0.0.1:1", "secret-key", nil)
	_, err := c.Channels(context.Background(), []string{"c1"})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected unavailable, got %v", err)
	}
	if strings.Contains(err.Error(), "secret-key") {
		t.Fatalf("error leaks API key: %v", err)
	}
}

func TestChannelByHandleAndMine(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case q.Get("forHandle") == "@taken":
			_, _ = w.Write([]byte(`{"items":[{"id":"UC1","snippet":{"title":"Taken","customUrl":"@taken"},"statistics":{"subscriberCount":"10"}}]}`))
		case q.Get("forHandle") != "":
			_, _ = w.Write([]byte(`{}`))
		case q.Get("mine") == "true":
			if r.Header.Get("Authorization") != "Bearer at" || q.Get("key") != "" {
				t.Errorf("mine must use bearer auth and no API key")
			}
			_, _ = w.Write([]byte(`{"items":[{"id":"UCme","snippet":{"title":"Mine"},"statistics":{"hiddenSubscriberCount":true}}]}`))
		}
	}))
	defer srv.Close()
	c := NewDataClient(srv.Client(), srv.URL, "k", nil)
	ch, err := c.ChannelByHandle(context.Background(), "taken")
	if err != nil || ch == nil || ch.ID != "UC1" {
		t.Fatalf("taken: %+v %v", ch, err)
	}
	ch, err = c.ChannelByHandle(context.Background(), "free")
	if err != nil || ch != nil {
		t.Fatalf("free: %+v %v", ch, err)
	}
	mine, err := c.MyChannels(context.Background(), "at")
	if err != nil || len(mine) != 1 || !mine[0].HiddenSubscriber {
		t.Fatalf("mine: %+v %v", mine, err)
	}
	if _, err := NewDataClient(srv.Client(), srv.URL, "", nil).ChannelByHandle(context.Background(), "x"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected not configured without key, got %v", err)
	}
}
