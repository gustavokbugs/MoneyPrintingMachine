package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResumableUpload(t *testing.T) {
	var meta map[string]map[string]any
	var gotBody string
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/videos":
			if r.URL.Query().Get("uploadType") != "resumable" || r.Header.Get("Authorization") != "Bearer at" || r.Header.Get("X-Upload-Content-Length") != "4" {
				t.Errorf("bad init request: %v %v", r.URL, r.Header)
			}
			_ = json.NewDecoder(r.Body).Decode(&meta)
			w.Header().Set("Location", srv.URL+"/session/1")
		case r.Method == http.MethodPut && r.URL.Path == "/session/1":
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"yt123"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	file := filepath.Join(t.TempDir(), "v.mp4")
	_ = os.WriteFile(file, []byte("data"), 0o644)
	quota := &countingQuota{limit: 10000}
	u := NewUploader(srv.Client(), srv.URL, quota)
	id, err := u.Upload(context.Background(), "at", UploadMeta{
		Title: "  The <Moon> Is Leaving Us  " + strings.Repeat("x", 120), Description: "Desc", Tags: []string{"moon", "a,b"},
		CategoryID: "27", Language: "en-US", Privacy: "private", SyntheticMedia: true,
	}, file)
	if err != nil || id != "yt123" || gotBody != "data" {
		t.Fatalf("upload: id=%q body=%q err=%v", id, gotBody, err)
	}
	title := meta["snippet"]["title"].(string)
	if strings.ContainsAny(title, "<>") || len([]rune(title)) != 100 {
		t.Fatalf("title not sanitized: %q", title)
	}
	if meta["status"]["privacyStatus"] != "private" || meta["status"]["containsSyntheticMedia"] != true {
		t.Fatalf("status: %v", meta["status"])
	}
	if quota.used != CostUpload {
		t.Fatalf("quota used = %d", quota.used)
	}
}

func TestUploadValidation(t *testing.T) {
	u := NewUploader(http.DefaultClient, "http://127.0.0.1:1", &countingQuota{limit: 100})
	if _, err := u.Upload(context.Background(), "at", UploadMeta{Privacy: "everyone"}, "x"); err == nil {
		t.Fatal("invalid privacy must fail")
	}
	file := filepath.Join(t.TempDir(), "v.mp4")
	_ = os.WriteFile(file, []byte("data"), 0o644)
	if _, err := u.Upload(context.Background(), "at", UploadMeta{Privacy: "private"}, file); !errors.Is(err, ErrQuotaBudget) {
		t.Fatalf("expected quota budget error, got %v", err)
	}
}
