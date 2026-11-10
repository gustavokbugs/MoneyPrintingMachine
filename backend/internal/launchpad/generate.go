// Package launchpad implements the Channel Launchpad: niche discovery,
// strategy, identity, and content blueprint generation using a local LLM.
//
// Model output is treated as untrusted input: it is constrained by a JSON
// schema, decoded strictly, validated, and normalized before it is stored.
package launchpad

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"contentfactory/internal/channel"
	"contentfactory/internal/llm"
)

// Generator produces structured output from a local model.
type Generator interface {
	GenerateJSON(ctx context.Context, req llm.Request, out any) error
	Model() string
}

// Engine runs Launchpad generation steps.
type Engine struct {
	gen Generator
	now func() time.Time
}

// NewEngine builds an Engine.
func NewEngine(gen Generator) *Engine {
	return &Engine{gen: gen, now: time.Now}
}

func (e *Engine) provenance() channel.Provenance {
	return channel.Provenance{Generator: "ollama", Model: e.gen.Model(), GeneratedAt: e.now().UTC()}
}

// maxSampleAttempts bounds re-sampling when the model output fails
// decoding or validation.
const maxSampleAttempts = 3

// sample calls the model and validates the output, re-sampling a bounded
// number of times when the output is invalid.
func sample[T any](ctx context.Context, gen Generator, req llm.Request, validate func(*T) error) (*T, error) {
	var lastErr error
	for range maxSampleAttempts {
		var out T
		err := gen.GenerateJSON(ctx, req, &out)
		if err == nil && validate != nil {
			if verr := validate(&out); verr != nil {
				err = fmt.Errorf("%w: %v", llm.ErrInvalidOutput, verr)
			}
		}
		if err == nil {
			return &out, nil
		}
		if !errors.Is(err, llm.ErrInvalidOutput) {
			return nil, err
		}
		lastErr = err
	}
	return nil, lastErr
}

// JSON schema helpers. Every object requires all of its properties and
// forbids additional ones, matching the strict decoder.

type schema = map[string]any

func str() schema                 { return schema{"type": "string"} }
func integer(min, max int) schema { return schema{"type": "integer", "minimum": min, "maximum": max} }
func enum(values ...string) schema {
	return schema{"type": "string", "enum": values}
}
func array(items schema, min, max int) schema {
	return schema{"type": "array", "items": items, "minItems": min, "maxItems": max}
}

func object(props map[string]schema) schema {
	required := make([]string, 0, len(props))
	p := make(map[string]any, len(props))
	for k, v := range props {
		required = append(required, k)
		p[k] = v
	}
	return schema{"type": "object", "properties": p, "required": required, "additionalProperties": false}
}

func mustSchema(s schema) json.RawMessage {
	raw, err := json.Marshal(s)
	if err != nil {
		// Schemas are static program data; failure is a programming error.
		panic(fmt.Sprintf("marshal schema: %v", err))
	}
	return raw
}

var languageNames = map[string]string{
	"en-US": "American English",
	"en-GB": "British English",
	"es-ES": "Spanish (Spain)",
	"es-MX": "Mexican Spanish",
	"pt-BR": "Brazilian Portuguese",
	"fr-FR": "French",
	"de-DE": "German",
}

func languageInstruction(tag, geo string) string {
	name := languageNames[tag]
	if name == "" {
		name = "the language identified by the BCP 47 tag " + tag
	}
	return fmt.Sprintf("Write every text value in %s, using natural phrasing and cultural references that fit an audience in country %s.", name, geo)
}

const baseSystemPrompt = `You are a senior editorial strategist for original online video channels.
Principles:
- Favor original, useful, well-researched content over volume or clickbait.
- Never invent statistics, view counts, subscriber counts, search volumes, or other measurements.
- When you are unsure about a fact, say so instead of guessing.
- Avoid content that infringes copyright, impersonates people or brands, or misleads viewers.
- Respond only with JSON that matches the provided schema.`

func messages(system, user string) []llm.Message {
	return []llm.Message{{Role: "system", Content: system}, {Role: "user", Content: user}}
}

// clean trims whitespace and collapses internal runs of whitespace.
func clean(s string) string { return strings.Join(strings.Fields(s), " ") }

func cleanList(items []string, max int) []string {
	out := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, it := range items {
		it = clean(it)
		key := strings.ToLower(it)
		if it == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, it)
		if len(out) == max {
			break
		}
	}
	return out
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
