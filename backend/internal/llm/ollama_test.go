package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type sample struct {
	Title string `json:"title"`
}

func TestGenerateJSON(t *testing.T) {
	var got chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(chatResponse{Message: Message{Role: "assistant", Content: `{"title":"Hello"}`}, Done: true})
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, "qwen3:4b", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var out sample
	err = c.GenerateJSON(context.Background(), Request{
		Messages: []Message{{Role: "user", Content: "hi"}},
		Schema:   json.RawMessage(`{"type":"object"}`),
	}, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Title != "Hello" {
		t.Fatalf("unexpected output: %+v", out)
	}
	if got.Model != "qwen3:4b" || got.Stream || got.Think || string(got.Format) != `{"type":"object"}` {
		t.Fatalf("unexpected request: %+v", got)
	}
}

func TestGenerateJSONErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr error
	}{
		{"invalid json content", 200, `{"message":{"role":"assistant","content":"not json"}}`, ErrInvalidOutput},
		{"unknown field", 200, `{"message":{"role":"assistant","content":"{\"title\":\"a\",\"extra\":1}"}}`, ErrInvalidOutput},
		{"model missing", 404, `{"error":"model not found"}`, ErrUnavailable},
		{"server error", 500, `{"error":"boom"}`, ErrUnavailable},
		{"garbage body", 502, `<html>`, ErrUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()
			c, _ := NewClient(srv.URL, "m", 5*time.Second)
			var out sample
			err := c.GenerateJSON(context.Background(), Request{}, &out)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestGenerateJSONUnreachable(t *testing.T) {
	c, _ := NewClient("http://127.0.0.1:1", "m", time.Second)
	var out sample
	if err := c.GenerateJSON(context.Background(), Request{}, &out); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected unavailable, got %v", err)
	}
}

func TestNewClientValidation(t *testing.T) {
	for _, u := range []string{"", "ftp://x", "localhost:11434", "http://"} {
		if _, err := NewClient(u, "m", time.Second); err == nil {
			t.Errorf("expected error for %q", u)
		}
	}
	if _, err := NewClient("http://localhost:11434", " ", time.Second); err == nil {
		t.Error("expected error for empty model")
	}
}
