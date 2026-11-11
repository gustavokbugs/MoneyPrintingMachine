package stock

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const commonsSample = `{"query":{"pages":{
 "11":{"pageid":11,"index":2,"title":"File:Moon B.jpg","imageinfo":[{"thumburl":"https://upload.wikimedia.org/b.jpg","thumbwidth":1920,"thumbheight":1280,"width":4000,"height":3000,"mime":"image/jpeg","descriptionurl":"https://commons.wikimedia.org/wiki/File:Moon_B.jpg",
   "extmetadata":{"License":{"value":"cc-by-sa-4.0"},"LicenseShortName":{"value":"CC BY-SA 4.0"},"Artist":{"value":"<a href=\"x\">Jane &amp; Co</a>"},"ObjectName":{"value":"Moon B"}}}]},
 "10":{"pageid":10,"index":1,"title":"File:Moon A.jpg","imageinfo":[{"thumburl":"https://upload.wikimedia.org/a.jpg","thumbwidth":1920,"thumbheight":1280,"width":3000,"height":2000,"mime":"image/jpeg","descriptionurl":"https://commons.wikimedia.org/wiki/File:Moon_A.jpg",
   "extmetadata":{"License":{"value":"pd"},"LicenseShortName":{"value":"Public domain"},"Artist":{"value":"NASA"}}}]},
 "12":{"pageid":12,"index":3,"title":"File:NC.jpg","imageinfo":[{"thumburl":"https://upload.wikimedia.org/c.jpg","width":3000,"height":2000,"mime":"image/jpeg","extmetadata":{"License":{"value":"cc-by-nc-2.0"}}}]},
 "13":{"pageid":13,"index":4,"title":"File:Tiny.jpg","imageinfo":[{"thumburl":"https://upload.wikimedia.org/d.jpg","width":300,"height":200,"mime":"image/jpeg","extmetadata":{"License":{"value":"cc0"}}}]},
 "14":{"pageid":14,"index":5,"title":"File:Elsewhere.jpg","imageinfo":[{"thumburl":"https://evil.example.com/e.jpg","width":3000,"height":2000,"mime":"image/jpeg","extmetadata":{"License":{"value":"cc0"}}}]}
}}}`

func TestCommonsSearchFiltersLicensesAndKeepsCredits(t *testing.T) {
	var ua, query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua, query = r.Header.Get("User-Agent"), r.URL.Query().Get("gsrsearch")
		_, _ = w.Write([]byte(commonsSample))
	}))
	defer srv.Close()
	c := NewCommons(5 * time.Second)
	c.base = srv.URL
	clips, err := c.Search(context.Background(), "moon", "portrait", 1080, 1920, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ua, "ContentFactory/") || query != "moon filetype:bitmap" {
		t.Errorf("ua=%q query=%q", ua, query)
	}
	if len(clips) != 2 || clips[0].ID != 10 || clips[1].ID != 11 {
		t.Fatalf("clips = %+v, want the two reusable, large, allowed-host images in relevance order", clips)
	}
	b := clips[1]
	if b.Author != "Jane & Co" || b.License != "CC BY-SA 4.0" || b.Title != "Moon B" || !b.Image || b.Provider != ProviderCommons {
		t.Errorf("credit fields = %+v", b)
	}
}

func TestCommercialLicense(t *testing.T) {
	for code, want := range map[string]bool{"pd": true, "cc0": true, "cc-by-4.0": true, "cc-by-sa-3.0": true, "pd-usgov-nasa": true,
		"cc-by-nc-2.0": false, "cc-by-nd-4.0": false, "gfdl": false, "": false} {
		if got := commercialLicense(code); got != want {
			t.Errorf("%q: got %v", code, got)
		}
	}
}

func TestSourcesFallBackToCommons(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(commonsSample)) }))
	defer srv.Close()
	commons := NewCommons(5 * time.Second)
	commons.base = srv.URL
	s := Sources{Pexels: NewClient("", time.Second), Commons: commons}
	clips, err := s.Search(context.Background(), "moon", "portrait", 1080, 1920, 5)
	if err != nil || len(clips) != 2 {
		t.Fatalf("fallback: %v %+v", err, clips)
	}
	if _, err := (Sources{Pexels: NewClient("", time.Second)}).Search(context.Background(), "x", "portrait", 1, 1, 1); !errors.Is(err, ErrNoKey) {
		t.Errorf("no sources: %v", err)
	}
	if (Sources{}).Available() {
		t.Error("empty sources reported available")
	}
}

func TestPreferRelevantTitles(t *testing.T) {
	clips := []Clip{{ID: 1, Title: "Birdhouses on Sidney Spit"}, {ID: 2, Title: "Waves at Ocean Beach"}, {ID: 3, Title: "Big wave"}}
	got := preferRelevant(clips, "waves beach")
	if len(got) != 2 || got[0].ID != 2 || got[1].ID != 3 {
		t.Errorf("got %+v", got)
	}
	if len(preferRelevant(clips, "zz")) != 0 {
		t.Error("photos without a shared word must be dropped")
	}
	// Dates and filler words never match; more shared words rank first.
	dated := []Clip{{ID: 1, Title: "Ataturk-September 20, 1928"}, {ID: 2, Title: "Mold on a petri dish"}, {ID: 3, Title: "Penicillin mold on a petri dish, 1928"}}
	got = preferRelevant(dated, "penicillin mold september 1928")
	if len(got) != 2 || got[0].ID != 3 || got[1].ID != 2 {
		t.Errorf("dated: got %+v", got)
	}
	if len(preferRelevant(dated, "september 1928")) != 0 {
		t.Error("a date-only query must not match")
	}
}

func TestCommonsRetriesOnceAfterRateLimit(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(commonsSample))
	}))
	defer srv.Close()
	c := NewCommons(5 * time.Second)
	c.base, c.gap = srv.URL, 0
	if clips, err := c.Search(context.Background(), "moon", "", 1080, 1920, 10); err != nil || len(clips) != 2 || calls != 2 {
		t.Fatalf("clips=%d err=%v calls=%d, want a successful retry", len(clips), err, calls)
	}

	// A second 429, or a Retry-After beyond the cap, gives up with ErrRateLimited.
	always := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer always.Close()
	c.base = always.URL
	if _, err := c.Search(context.Background(), "moon", "", 1080, 1920, 10); !errors.Is(err, ErrRateLimited) || !strings.Contains(err.Error(), "wikimedia") {
		t.Fatalf("err = %v, want a wikimedia rate-limit error", err)
	}
}

func TestCommonsPacesRequests(t *testing.T) {
	c := NewCommons(time.Second)
	c.gap = 40 * time.Millisecond
	start := time.Now()
	for range 3 {
		if err := c.wait(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if d := time.Since(start); d < 80*time.Millisecond {
		t.Errorf("three requests took %v, want at least two gaps", d)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.gap = time.Hour
	if err := c.wait(ctx); err == nil {
		t.Error("wait must honor cancellation")
	}
}
