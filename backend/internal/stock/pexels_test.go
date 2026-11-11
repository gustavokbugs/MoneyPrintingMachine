package stock

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

const sample = `{"videos":[
 {"id":1,"url":"https://www.pexels.com/video/1/","duration":12,"user":{"name":"Ana","url":"https://www.pexels.com/@ana"},
  "video_files":[
   {"id":10,"file_type":"video/mp4","width":540,"height":960,"link":"https://videos.pexels.com/video-files/1/sd.mp4"},
   {"id":11,"file_type":"video/mp4","width":1080,"height":1920,"link":"https://videos.pexels.com/video-files/1/hd.mp4"},
   {"id":12,"file_type":"video/mp4","width":2160,"height":3840,"link":"https://videos.pexels.com/video-files/1/uhd.mp4"},
   {"id":13,"file_type":"video/mp4","width":1080,"height":1920,"link":"https://evil.example.com/x.mp4"}]},
 {"id":2,"url":"https://www.pexels.com/video/2/","duration":8,"user":{"name":"Bo","url":"https://www.pexels.com/@bo"},
  "video_files":[{"id":20,"file_type":"video/mp4","width":720,"height":1280,"link":"https://videos.pexels.com/video-files/2/hd.mp4"}]},
 {"id":3,"url":"https://www.pexels.com/video/3/","duration":5,"user":{"name":"Cy","url":""},
  "video_files":[{"id":30,"file_type":"video/webm","width":1080,"height":1920,"link":"https://videos.pexels.com/video-files/3/a.webm"}]}
]}`

func testClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := NewClient("key-123", 5*time.Second)
	c.base = srv.URL
	return c
}

func TestSearchPicksSmallestCoveringFile(t *testing.T) {
	var gotAuth, gotQuery string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotQuery = r.Header.Get("Authorization"), r.URL.RawQuery
		_, _ = w.Write([]byte(sample))
	})
	clips, err := c.Search(context.Background(), "moon surface", "portrait", 1080, 1920, 10)
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "key-123" {
		t.Errorf("auth header = %q", gotAuth)
	}
	q, _ := url.ParseQuery(gotQuery)
	if q.Get("query") != "moon surface" || q.Get("orientation") != "portrait" || q.Get("per_page") != "10" {
		t.Errorf("query = %v", q)
	}
	if len(clips) != 2 {
		t.Fatalf("clips = %+v (webm-only video must be skipped)", clips)
	}
	if clips[0].FileID != 11 || clips[0].Author != "Ana" {
		t.Errorf("first clip = %+v, want the 1080x1920 file", clips[0])
	}
	if clips[1].FileID != 20 {
		t.Errorf("second clip = %+v, want its only (smaller) file", clips[1])
	}
}

func TestSearchErrors(t *testing.T) {
	if _, err := NewClient("", time.Second).Search(context.Background(), "x", "portrait", 1, 1, 1); !errors.Is(err, ErrNoKey) {
		t.Errorf("no key: %v", err)
	}
	limited := testClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTooManyRequests) })
	if _, err := limited.Search(context.Background(), "x", "portrait", 1, 1, 1); !errors.Is(err, ErrRateLimited) {
		t.Errorf("429: %v", err)
	}
	denied := testClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) })
	if _, err := denied.Search(context.Background(), "x", "portrait", 1, 1, 1); err == nil {
		t.Error("401: expected an error")
	}
}

func TestAllowedFileHost(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://videos.pexels.com/a.mp4":         true,
		"https://player.vimeo.com/external/1.mp4": true,
		"http://videos.pexels.com/a.mp4":          false,
		"https://pexels.com.evil.net/a.mp4":       false,
		"https://127.0.0.1/a.mp4":                 false,
		"https://notpexels.com/a.mp4":             false,
	} {
		u, _ := url.Parse(raw)
		if got := allowedFileHost(u); got != want {
			t.Errorf("%s: got %v, want %v", raw, got, want)
		}
	}
}

func TestDownloadRejectsUnexpectedHosts(t *testing.T) {
	c := NewClient("k", time.Second)
	_, err := c.Download(context.Background(), Clip{ID: 1, FileID: 2, FileURL: "https://evil.example.com/x.mp4"}, t.TempDir())
	if err == nil {
		t.Fatal("expected a rejection")
	}
}
