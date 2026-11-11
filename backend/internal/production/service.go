package production

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"contentfactory/internal/channel"
	"contentfactory/internal/jobs"
	"contentfactory/internal/llm"
)

// Job kinds.
const (
	KindScript  = "production.script"
	KindProduce = "production.produce"
)

// Store persists videos. Every method is owner-scoped.
type Store interface {
	Create(ctx context.Context, v *Video) error
	Get(ctx context.Context, ownerID, id string) (*Video, error)
	ListByChannel(ctx context.Context, ownerID, channelID string) ([]Video, error)
	// Save writes the video if its version matches, then bumps the version.
	Save(ctx context.Context, v *Video) error
}

// Channels loads channels with ownership checks.
type Channels interface {
	Get(ctx context.Context, ownerID, id string) (*channel.Channel, error)
}

// Generator produces structured output from the local model.
type Generator interface {
	GenerateJSON(ctx context.Context, req llm.Request, out any) error
	Model() string
}

// Service implements the production use cases.
type Service struct {
	store    Store
	channels Channels
	gen      Generator
	tools    Tools
	runner   Runner
	mediaDir string
	now      func() time.Time
}

// NewService builds a Service. mediaDir holds rendered files and the TTS cache.
func NewService(store Store, channels Channels, gen Generator, tools Tools, runner Runner, mediaDir string) *Service {
	return &Service{store: store, channels: channels, gen: gen, tools: tools, runner: runner, mediaDir: mediaDir, now: time.Now}
}

// ToolsMissing lists missing media tools (empty when ready).
func (s *Service) ToolsMissing() []string { return s.tools.Missing() }

// CreateFromIdea creates a planned video from a blueprint idea.
func (s *Service) CreateFromIdea(ctx context.Context, ownerID, channelID string, ideaIndex int) (*Video, error) {
	ch, err := s.channels.Get(ctx, ownerID, channelID)
	if err != nil {
		return nil, err
	}
	if ch.Blueprint == nil || ideaIndex < 0 || ideaIndex >= len(ch.Blueprint.Ideas) {
		return nil, &channel.ValidationError{Field: "ideaIndex", Message: "does not match an idea in the content blueprint"}
	}
	idea := ch.Blueprint.Ideas[ideaIndex]
	aspect := "16:9"
	for _, f := range ch.Blueprint.Formats {
		if f.Name == idea.Format {
			aspect = f.AspectRatio
		}
	}
	target := min(max(idea.SuggestedDurationSecs, 15), 3600)
	v := &Video{
		OwnerID: ownerID, ChannelID: ch.ID, IdeaIndex: ideaIndex, Title: idea.WorkingTitle, Format: idea.Format,
		AspectRatio: aspect, TargetSeconds: target, Status: StatusPlanned, VoiceID: 6,
	}
	if err := s.store.Create(ctx, v); err != nil {
		return nil, err
	}
	return v, nil
}

// Get returns a video.
func (s *Service) Get(ctx context.Context, ownerID, id string) (*Video, error) {
	return s.store.Get(ctx, ownerID, id)
}

// List returns a channel's videos.
func (s *Service) List(ctx context.Context, ownerID, channelID string) ([]Video, error) {
	if _, err := s.channels.Get(ctx, ownerID, channelID); err != nil {
		return nil, err
	}
	return s.store.ListByChannel(ctx, ownerID, channelID)
}

// mutate loads, applies fn, and saves with bounded conflict retries.
func (s *Service) mutate(ctx context.Context, ownerID, id string, fn func(*Video) error) (*Video, error) {
	var last error
	for range 3 {
		v, err := s.store.Get(ctx, ownerID, id)
		if err != nil {
			return nil, err
		}
		if err := fn(v); err != nil {
			return nil, err
		}
		err = s.store.Save(ctx, v)
		if err == nil {
			return v, nil
		}
		if !errors.Is(err, ErrConflict) {
			return nil, err
		}
		last = err
	}
	return nil, last
}

// EditScript replaces the script; approval is required again afterwards.
// Claim review states are carried over for claims whose text is unchanged.
func (s *Service) EditScript(ctx context.Context, ownerID, id string, sc Script) (*Video, error) {
	return s.mutate(ctx, ownerID, id, func(v *Video) error {
		prev := map[string]Claim{}
		if v.Script != nil {
			for _, c := range v.Script.Claims {
				prev[c.Text] = c
			}
			sc.Provenance = v.Script.Provenance
		}
		for i := range sc.Claims {
			sc.Claims[i].Status = ClaimUnverified
			if p, ok := prev[sc.Claims[i].Text]; ok {
				sc.Claims[i].Status, sc.Claims[i].Note = p.Status, p.Note
			}
		}
		sc.Provenance.EditedByUser = true
		return v.SetScript(sc)
	})
}

// ReviewClaim marks one claim verified, removed, or unverified.
func (s *Service) ReviewClaim(ctx context.Context, ownerID, id string, index int, status, note string) (*Video, error) {
	return s.mutate(ctx, ownerID, id, func(v *Video) error { return v.SetClaimStatus(index, status, note) })
}

// Approve passes the fact-check gate.
func (s *Service) Approve(ctx context.Context, ownerID, id string) (*Video, error) {
	return s.mutate(ctx, ownerID, id, func(v *Video) error { return v.Approve() })
}

// SetVoice chooses the narration voice.
func (s *Service) SetVoice(ctx context.Context, ownerID, id string, voice int) (*Video, error) {
	if !ValidVoice(voice) {
		return nil, &channel.ValidationError{Field: "voiceId", Message: "is not a known voice"}
	}
	return s.mutate(ctx, ownerID, id, func(v *Video) error {
		if v.Status == StatusProducing {
			return &StateError{Status: v.Status, Action: "change the voice", Reason: "production is running"}
		}
		v.VoiceID = voice
		return nil
	})
}

// CheckCanScript validates that script generation may start.
func (s *Service) CheckCanScript(ctx context.Context, ownerID, id string) (*Video, error) {
	v, err := s.store.Get(ctx, ownerID, id)
	if err != nil {
		return nil, err
	}
	return v, v.CanStartScript()
}

// CheckCanProduce validates that production may start.
func (s *Service) CheckCanProduce(ctx context.Context, ownerID, id string) (*Video, error) {
	v, err := s.store.Get(ctx, ownerID, id)
	if err != nil {
		return nil, err
	}
	if err := v.CanStartProduction(); err != nil {
		return v, err
	}
	if missing := s.tools.Missing(); len(missing) > 0 {
		return v, fmt.Errorf("%w (missing: %s)", ErrToolsMissing, strings.Join(missing, ", "))
	}
	return v, nil
}

// VideoDir is where a video's rendered files live.
func (s *Service) VideoDir(id string) string { return filepath.Join(s.mediaDir, "videos", id) }

// Handlers returns the production job handlers.
func (s *Service) Handlers() map[string]jobs.Handler {
	return map[string]jobs.Handler{
		KindScript:  s.handleScript,
		KindProduce: s.handleProduce,
	}
}

type videoInput struct {
	VideoID string `json:"videoId"`
}

func decodeInput(j jobs.Job) (string, error) {
	var in videoInput
	if err := json.Unmarshal(j.Input, &in); err != nil || in.VideoID == "" {
		return "", jobs.Permanent(errors.New("job input has no video ID"))
	}
	return in.VideoID, nil
}

// fail records a failure reason on the video, best effort.
func (s *Service) fail(ctx context.Context, owner, id string, cause error) {
	msg := cause.Error()
	if len(msg) > 500 {
		msg = msg[:500]
	}
	_, _ = s.mutate(context.WithoutCancel(ctx), owner, id, func(v *Video) error {
		v.Status, v.StatusReason = StatusFailed, msg
		return nil
	})
}

func permanentIfDomain(err error) error {
	var se *StateError
	var ve *channel.ValidationError
	var ue *UnspeakableError
	if errors.As(err, &ue) || errors.As(err, &se) || errors.As(err, &ve) || errors.Is(err, ErrNotFound) || errors.Is(err, channel.ErrNotFound) || errors.Is(err, ErrToolsMissing) {
		return jobs.Permanent(err)
	}
	return err
}

func (s *Service) handleScript(ctx context.Context, j jobs.Job) (any, error) {
	id, err := decodeInput(j)
	if err != nil {
		return nil, err
	}
	v, err := s.mutate(ctx, j.OwnerID, id, func(v *Video) error {
		if err := v.CanStartScript(); err != nil && v.Status != StatusScripting {
			return err
		}
		v.Status, v.StatusReason = StatusScripting, ""
		return nil
	})
	if err != nil {
		return nil, permanentIfDomain(err)
	}
	ch, err := s.channels.Get(ctx, j.OwnerID, v.ChannelID)
	if err != nil {
		return nil, permanentIfDomain(err)
	}
	sc, err := s.GenerateScript(ctx, ch, v)
	if err != nil {
		// Leave retryable failures in "scripting" so the retry can continue;
		// mark the video failed only when no retry will happen.
		if j.Attempts >= j.MaxAttempts || jobs.IsPermanent(permanentIfDomain(err)) {
			s.fail(ctx, j.OwnerID, id, err)
		}
		return nil, permanentIfDomain(err)
	}
	if _, err := s.mutate(ctx, j.OwnerID, id, func(v *Video) error {
		v.Status = StatusPlanned // SetScript moves it to script_ready.
		return v.SetScript(*sc)
	}); err != nil {
		return nil, permanentIfDomain(err)
	}
	return map[string]string{"videoId": id}, nil
}

func (s *Service) handleProduce(ctx context.Context, j jobs.Job) (any, error) {
	id, err := decodeInput(j)
	if err != nil {
		return nil, err
	}
	v, err := s.mutate(ctx, j.OwnerID, id, func(v *Video) error {
		if v.Status != StatusProducing {
			if err := v.CanStartProduction(); err != nil {
				return err
			}
		}
		v.Status, v.StatusReason = StatusProducing, ""
		return nil
	})
	if err != nil {
		return nil, permanentIfDomain(err)
	}
	ch, err := s.channels.Get(ctx, j.OwnerID, v.ChannelID)
	if err != nil {
		return nil, permanentIfDomain(err)
	}
	out, err := s.Produce(ctx, ch, v)
	if err != nil {
		if j.Attempts >= j.MaxAttempts || jobs.IsPermanent(permanentIfDomain(err)) {
			s.fail(ctx, j.OwnerID, id, err)
		}
		return nil, permanentIfDomain(err)
	}
	if _, err := s.mutate(ctx, j.OwnerID, id, func(v *Video) error {
		v.Output = out
		v.Status, v.StatusReason = StatusProduced, ""
		return nil
	}); err != nil {
		return nil, err
	}
	return map[string]any{"videoId": id, "durationSeconds": out.DurationSeconds}, nil
}

// Produce narrates every segment and renders the final MP4 and SRT.
func (s *Service) Produce(ctx context.Context, ch *channel.Channel, v *Video) (*Output, error) {
	if missing := s.tools.Missing(); len(missing) > 0 {
		return nil, fmt.Errorf("%w (missing: %s)", ErrToolsMissing, strings.Join(missing, ", "))
	}
	if v.Script == nil {
		return nil, &StateError{Status: v.Status, Action: "produce", Reason: "there is no script"}
	}
	dir := s.VideoDir(v.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create video directory: %w", err)
	}
	narrator := Narrator{Tools: s.tools, Runner: s.runner, CacheDir: filepath.Join(s.mediaDir, "tts-cache")}
	lex, err := LoadLexicon(filepath.Join(s.tools.ModelDir, "lexicon-en-us.txt"))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrToolsMissing, err)
	}
	// Normalize every segment first so all unpronounceable words are
	// reported at once, before any audio is generated.
	segments := v.Script.Segments()
	spoken := make([]string, len(segments))
	var unspeakable []string
	seenOOV := map[string]bool{}
	for i, seg := range segments {
		text, oov := NormalizeForSpeech(seg.Narration, lex)
		spoken[i] = text
		for _, w := range oov {
			if !seenOOV[w] {
				seenOOV[w] = true
				unspeakable = append(unspeakable, w)
			}
		}
	}
	if len(unspeakable) > 0 {
		return nil, &UnspeakableError{Words: unspeakable}
	}
	var segs []Timed
	var audios []string
	total := 0.0
	for i, seg := range segments {
		wav, err := narrator.Synthesize(ctx, v.VoiceID, spoken[i])
		if err != nil {
			return nil, fmt.Errorf("narrate segment %d: %w", i+1, err)
		}
		d, err := ProbeDuration(ctx, s.runner, s.tools.FFprobe, wav)
		if err != nil {
			return nil, fmt.Errorf("measure segment %d: %w", i+1, err)
		}
		title := seg.OnScreenText
		if title == "" {
			title = seg.Heading
		}
		segs = append(segs, Timed{Audio: wav, Duration: d, Title: title, Text: seg.Narration})
		audios = append(audios, wav)
		total += d
	}
	width, height := v.Dimensions()
	style := Style{Background: "#0F172A", Text: "#F8FAFC", Accent: "#60A5FA", Font: "Inter"}
	if idn := ch.Identity; idn != nil {
		style.Background, style.Text, style.Accent = idn.Palette.Primary, idn.Palette.Background, idn.Palette.Accent
	}
	if err := os.WriteFile(filepath.Join(dir, "captions.ass"), []byte(BuildASS(segs, width, height, style)), 0o644); err != nil {
		return nil, fmt.Errorf("write subtitles: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "captions.srt"), []byte(BuildSRT(segs)), 0o644); err != nil {
		return nil, fmt.Errorf("write captions: %w", err)
	}
	fontsRel, err := filepath.Rel(dir, s.tools.FontsDir)
	if err != nil {
		return nil, fmt.Errorf("locate fonts: %w", err)
	}
	rctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	tmp := "video.tmp.mp4"
	if _, err := s.runner.Run(rctx, dir, s.tools.FFmpeg, FFmpegArgs(audios, total, width, height, style.Background, "captions.ass", fontsRel, tmp)...); err != nil {
		_ = os.Remove(filepath.Join(dir, tmp))
		return nil, fmt.Errorf("render video: %w", err)
	}
	if err := os.Rename(filepath.Join(dir, tmp), filepath.Join(dir, "video.mp4")); err != nil {
		return nil, fmt.Errorf("store video: %w", err)
	}
	dur, err := ProbeDuration(ctx, s.runner, s.tools.FFprobe, filepath.Join(dir, "video.mp4"))
	if err != nil {
		return nil, fmt.Errorf("verify render: %w", err)
	}
	return &Output{
		VideoFile: "video.mp4", CaptionsFile: "captions.srt", DurationSeconds: dur,
		Width: width, Height: height, RenderedAt: s.now().UTC(), SyntheticVoice: true,
	}, nil
}

// MediaPath resolves a downloadable file of a video. Only known file names
// are served, so the request cannot reach other paths.
func (s *Service) MediaPath(ctx context.Context, ownerID, id, name string) (string, error) {
	if name != "video.mp4" && name != "captions.srt" {
		return "", ErrNotFound
	}
	v, err := s.store.Get(ctx, ownerID, id)
	if err != nil {
		return "", err
	}
	if v.Output == nil {
		return "", ErrNotFound
	}
	p := filepath.Join(s.VideoDir(v.ID), name)
	if _, err := os.Stat(p); err != nil {
		return "", ErrNotFound
	}
	return p, nil
}
