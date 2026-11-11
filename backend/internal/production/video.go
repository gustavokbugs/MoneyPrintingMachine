// Package production turns blueprint ideas into finished videos: script
// generation with a fact-check approval gate, local narration, and FFmpeg
// rendering. External tools run as separate processes with argument lists,
// never through a shell.
package production

import (
	"contentfactory/internal/stock"
	"errors"
	"fmt"
	"strings"
	"time"

	"contentfactory/internal/channel"
)

// Status is a video lifecycle state.
type Status string

// Video states.
const (
	StatusPlanned     Status = "planned"
	StatusScripting   Status = "scripting"
	StatusScriptReady Status = "script_ready"
	StatusApproved    Status = "approved"
	StatusProducing   Status = "producing"
	StatusProduced    Status = "produced"
	StatusFailed      Status = "failed"
)

// Claim review states.
const (
	ClaimUnverified = "unverified"
	ClaimVerified   = "verified"
	ClaimRemoved    = "removed"
)

var (
	// ErrNotFound is returned when a video does not exist for the owner.
	ErrNotFound = errors.New("video not found")
	// ErrConflict is returned on a concurrent modification.
	ErrConflict = errors.New("video was modified concurrently")
	// ErrExists is returned when the idea already has a video.
	ErrExists = errors.New("a video for this idea already exists")
)

// StateError reports an action that is not allowed in the current state.
type StateError struct {
	Status Status
	Action string
	Reason string
}

func (e *StateError) Error() string {
	return fmt.Sprintf("cannot %s while the video is %s: %s", e.Action, e.Status, e.Reason)
}

// Section is one narrated part of the script.
type Section struct {
	Heading      string `json:"heading"`
	Narration    string `json:"narration"`
	OnScreenText string `json:"onScreenText"`
}

// Claim is a factual statement that a person must verify or remove before
// the script can be approved.
type Claim struct {
	Text       string `json:"text"`
	SourceHint string `json:"sourceHint"`
	Status     string `json:"status"`
	Note       string `json:"note"`
}

// Script is the full narration plan for a video.
type Script struct {
	Hook             string             `json:"hook"`
	Sections         []Section          `json:"sections"`
	Outro            string             `json:"outro"`
	Claims           []Claim            `json:"claims"`
	EstimatedSeconds int                `json:"estimatedSeconds"`
	Provenance       channel.Provenance `json:"provenance"`
}

// Segments returns the narrated segments in order: hook, sections, outro.
func (s Script) Segments() []Section {
	out := make([]Section, 0, len(s.Sections)+2)
	if strings.TrimSpace(s.Hook) != "" {
		out = append(out, Section{Narration: s.Hook})
	}
	for _, sec := range s.Sections {
		if strings.TrimSpace(sec.Narration) != "" {
			out = append(out, sec)
		}
	}
	if strings.TrimSpace(s.Outro) != "" {
		out = append(out, Section{Narration: s.Outro})
	}
	return out
}

// WordCount counts narrated words.
func (s Script) WordCount() int {
	n := 0
	for _, seg := range s.Segments() {
		n += len(strings.Fields(seg.Narration))
	}
	return n
}

// Validate checks script structure and limits.
func (s Script) Validate() error {
	if len(s.Sections) == 0 || len(s.Sections) > 30 {
		return &channel.ValidationError{Field: "sections", Message: "must contain between 1 and 30 sections"}
	}
	for i, sec := range s.Sections {
		if strings.TrimSpace(sec.Narration) == "" {
			return &channel.ValidationError{Field: fmt.Sprintf("sections[%d].narration", i), Message: "is required"}
		}
		if len(sec.Narration) > 4000 || len(sec.OnScreenText) > 120 || len(sec.Heading) > 200 {
			return &channel.ValidationError{Field: fmt.Sprintf("sections[%d]", i), Message: "narration max 4000, on-screen text max 120, heading max 200 characters"}
		}
	}
	if len(s.Hook) > 1000 || len(s.Outro) > 1000 {
		return &channel.ValidationError{Field: "hook", Message: "hook and outro must be at most 1000 characters"}
	}
	if len(s.Claims) > 50 {
		return &channel.ValidationError{Field: "claims", Message: "must contain at most 50 claims"}
	}
	for i, c := range s.Claims {
		switch c.Status {
		case ClaimUnverified, ClaimVerified, ClaimRemoved:
		default:
			return &channel.ValidationError{Field: fmt.Sprintf("claims[%d].status", i), Message: "must be unverified, verified, or removed"}
		}
		if strings.TrimSpace(c.Text) == "" || len(c.Text) > 1000 {
			return &channel.ValidationError{Field: fmt.Sprintf("claims[%d].text", i), Message: "must be 1-1000 characters"}
		}
	}
	return nil
}

// UnresolvedClaims counts claims still awaiting review.
func (s Script) UnresolvedClaims() int {
	n := 0
	for _, c := range s.Claims {
		if c.Status == ClaimUnverified {
			n++
		}
	}
	return n
}

// Output describes rendered files.
type Output struct {
	VideoFile       string    `json:"videoFile"`
	CaptionsFile    string    `json:"captionsFile"`
	DurationSeconds float64   `json:"durationSeconds"`
	Width           int       `json:"width"`
	Height          int       `json:"height"`
	RenderedAt      time.Time `json:"renderedAt"`
	// SyntheticVoice records that narration is AI-generated, which platforms
	// may require creators to disclose.
	SyntheticVoice bool `json:"syntheticVoice"`
	// Visuals records how the picture was made: "animated:<style>" or
	// "solid" (fallback), plus a note when the fallback was used.
	Visuals     string `json:"visuals"`
	VisualsNote string `json:"visualsNote,omitempty"`
	// Music describes the soundtrack's provenance; SoundEffects counts the
	// effects placed on the timeline.
	Music        string `json:"music,omitempty"`
	SoundEffects int    `json:"soundEffects,omitempty"`
	// CaptionTiming is "measured" when word times come from the audio,
	// "estimated" when they are proportional to word length.
	CaptionTiming string `json:"captionTiming,omitempty"`
	// Footage credits every stock clip used (Pexels asks apps to credit
	// creators; the publish description includes them).
	Footage []stock.Clip `json:"footage,omitempty"`
}

// Video is one production unit.
type Video struct {
	ID            string    `json:"id"`
	OwnerID       string    `json:"-"`
	ChannelID     string    `json:"channelId"`
	IdeaIndex     int       `json:"ideaIndex"`
	Title         string    `json:"title"`
	Format        string    `json:"format"`
	AspectRatio   string    `json:"aspectRatio"`
	TargetSeconds int       `json:"targetSeconds"`
	Status        Status    `json:"status"`
	StatusReason  string    `json:"statusReason"`
	VoiceID       int       `json:"voiceId"`
	VisualStyle   string    `json:"visualStyle"`
	Script        *Script   `json:"script"`
	Output        *Output   `json:"output"`
	Version       int       `json:"version"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// CanStartScript reports whether script generation may start.
func (v *Video) CanStartScript() error {
	switch v.Status {
	case StatusPlanned, StatusScriptReady, StatusFailed:
		return nil
	}
	return &StateError{Status: v.Status, Action: "generate a script", Reason: "finish or cancel the current step first"}
}

// SetScript stores a new or edited script. Any edit requires a new
// approval, so an approved or produced video returns to script_ready.
func (v *Video) SetScript(s Script) error {
	if v.Status == StatusScripting || v.Status == StatusProducing {
		return &StateError{Status: v.Status, Action: "edit the script", Reason: "a job is running"}
	}
	if err := s.Validate(); err != nil {
		return err
	}
	v.Script = &s
	v.Status = StatusScriptReady
	v.StatusReason = ""
	return nil
}

// SetClaimStatus records the review outcome of one claim.
func (v *Video) SetClaimStatus(i int, status, note string) error {
	if v.Script == nil || i < 0 || i >= len(v.Script.Claims) {
		return &channel.ValidationError{Field: "claim", Message: "does not exist"}
	}
	if v.Status == StatusScripting || v.Status == StatusProducing {
		return &StateError{Status: v.Status, Action: "review claims", Reason: "a job is running"}
	}
	switch status {
	case ClaimUnverified, ClaimVerified, ClaimRemoved:
	default:
		return &channel.ValidationError{Field: "status", Message: "must be unverified, verified, or removed"}
	}
	if len(note) > 1000 {
		return &channel.ValidationError{Field: "note", Message: "must be at most 1000 characters"}
	}
	v.Script.Claims[i].Status = status
	v.Script.Claims[i].Note = strings.TrimSpace(note)
	// Changing a claim invalidates an earlier approval.
	if v.Status == StatusApproved || v.Status == StatusProduced {
		v.Status = StatusScriptReady
	}
	return nil
}

// Approve is the fact-check gate: every claim must be verified or removed.
func (v *Video) Approve() error {
	if v.Status != StatusScriptReady {
		return &StateError{Status: v.Status, Action: "approve", Reason: "only a ready script can be approved"}
	}
	if v.Script == nil {
		return &StateError{Status: v.Status, Action: "approve", Reason: "there is no script"}
	}
	if n := v.Script.UnresolvedClaims(); n > 0 {
		return &StateError{Status: v.Status, Action: "approve", Reason: fmt.Sprintf("%d claim(s) still need to be verified or removed", n)}
	}
	v.Status = StatusApproved
	v.StatusReason = ""
	return nil
}

// CanStartProduction reports whether narration and rendering may start.
func (v *Video) CanStartProduction() error {
	if v.Script == nil {
		return &StateError{Status: v.Status, Action: "produce", Reason: "there is no script"}
	}
	switch v.Status {
	case StatusApproved, StatusProduced:
		return nil
	case StatusFailed:
		if v.Script.UnresolvedClaims() == 0 {
			return nil
		}
	}
	return &StateError{Status: v.Status, Action: "produce", Reason: "the script must be approved first"}
}

// Dimensions returns the output frame size for the aspect ratio.
func (v *Video) Dimensions() (int, int) {
	if v.AspectRatio == "9:16" {
		return 1080, 1920
	}
	return 1920, 1080
}
