// Package llm provides a client for locally hosted language models served
// by Ollama (https://github.com/ollama/ollama, MIT license). No remote or
// paid model APIs are used.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrUnavailable means the local model server could not be reached or is
// not ready. Callers may retry later.
var ErrUnavailable = errors.New("local model server unavailable")

// ErrInvalidOutput means the model returned output that does not match the
// requested JSON schema. Retrying with a new sample may succeed.
var ErrInvalidOutput = errors.New("model returned invalid output")

// Message is one chat message.
type Message struct {
	Role    string `json:"role"` // "system", "user", or "assistant".
	Content string `json:"content"`
}

// Request describes a structured generation request.
type Request struct {
	Messages []Message
	// Schema is a JSON Schema the response must conform to.
	Schema      json.RawMessage
	Temperature float64
	// MaxTokens bounds the generated tokens; 0 uses the client default.
	MaxTokens int
}

// Client calls a local Ollama server.
type Client struct {
	baseURL    *url.URL
	model      string
	httpClient *http.Client
	contextLen int
}

// DefaultContextTokens is the context window requested from the model.
const DefaultContextTokens = 8192

// NewClient builds a Client. baseURL must be an http(s) URL; it is expected
// to point to a local or private-network Ollama instance.
func NewClient(baseURL, model string, timeout time.Duration) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("invalid Ollama URL %q", baseURL)
	}
	if strings.TrimSpace(model) == "" {
		return nil, errors.New("model name is required")
	}
	return &Client{
		baseURL:    u,
		model:      model,
		httpClient: &http.Client{Timeout: timeout},
		contextLen: DefaultContextTokens,
	}, nil
}

// WithContextTokens sets the context window. Smaller windows use less GPU
// memory but can truncate long outputs.
func (c *Client) WithContextTokens(n int) *Client {
	if n >= 2048 {
		c.contextLen = n
	}
	return c
}

// Model returns the configured model name.
func (c *Client) Model() string { return c.model }

type chatRequest struct {
	Model    string          `json:"model"`
	Messages []Message       `json:"messages"`
	Stream   bool            `json:"stream"`
	Format   json.RawMessage `json:"format,omitempty"`
	Think    bool            `json:"think"`
	Options  map[string]any  `json:"options,omitempty"`
}

type chatResponse struct {
	Message Message `json:"message"`
	Done    bool    `json:"done"`
	Error   string  `json:"error"`
}

// maxResponseBytes bounds how much of a response body is read.
const maxResponseBytes = 4 << 20

// GenerateJSON runs a chat completion constrained to req.Schema and decodes
// the result into out. Unknown fields in the model output are rejected.
func (c *Client) GenerateJSON(ctx context.Context, req Request, out any) error {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 2048
	}
	body, err := json.Marshal(chatRequest{
		Model:    c.model,
		Messages: req.Messages,
		Stream:   false,
		Format:   req.Schema,
		Think:    false,
		Options: map[string]any{
			"temperature": req.Temperature,
			"num_ctx":     c.contextLen,
			"num_predict": maxTokens,
		},
	})
	if err != nil {
		return fmt.Errorf("encode chat request: %w", err)
	}
	endpoint := c.baseURL.JoinPath("/api/chat")
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build chat request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("%w: read response: %v", ErrUnavailable, err)
	}
	var cr chatResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return fmt.Errorf("%w: decode Ollama response (HTTP %d)", ErrUnavailable, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		msg := cr.Error
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		if resp.StatusCode == http.StatusNotFound {
			return fmt.Errorf("%w: model %q is not installed (run `ollama pull %s`): %s", ErrUnavailable, c.model, c.model, msg)
		}
		return fmt.Errorf("%w: HTTP %d: %s", ErrUnavailable, resp.StatusCode, msg)
	}
	return DecodeStrict(cr.Message.Content, out)
}

// DecodeStrict decodes model output into out, rejecting trailing data and
// unknown fields.
func DecodeStrict(content string, out any) error {
	dec := json.NewDecoder(strings.NewReader(strings.TrimSpace(content)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidOutput, err)
	}
	if dec.More() {
		return fmt.Errorf("%w: trailing data after JSON value", ErrInvalidOutput)
	}
	return nil
}

// Ping checks that the server is reachable and the model is installed.
func (c *Client) Ping(ctx context.Context) error {
	endpoint := c.baseURL.JoinPath("/api/show")
	body, _ := json.Marshal(map[string]string{"model": c.model})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build ping request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%w: model %q is not installed", ErrUnavailable, c.model)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: HTTP %d", ErrUnavailable, resp.StatusCode)
	}
	return nil
}
