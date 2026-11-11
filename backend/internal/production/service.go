package production

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
	"time"

	"contentfactory/internal/channel"
	"contentfactory/internal/jobs"
	"contentfactory/internal/llm"
	"contentfactory/internal/stock"
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
	footage  FootageSource
}

// NewService builds a Service. mediaDir holds rendered files and the TTS cache.
func NewService(store Store, channels Channels, gen Generator, tools Tools, runner Runner, mediaDir string) *Service {
	// External tools run with other working directories, so every path
	// handed to them must be absolute.
	if abs, err := filepath.Abs(mediaDir); err == nil {
		mediaDir = abs
	}
	return &Service{store: store, channels: channels, gen: gen, tools: tools, runner: runner, mediaDir: mediaDir, now: time.Now}
}

// AnimatedScenes reports whether HyperFrames scene rendering is available.
func (s *Service) AnimatedScenes() bool { return s.tools.AnimatedScenes() }

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
	voiceA, voiceB := defaultVoices(ch.Language)
	v := &Video{
		OwnerID: ownerID, ChannelID: ch.ID, IdeaIndex: ideaIndex, Title: idea.WorkingTitle, Format: idea.Format,
		AspectRatio: aspect, TargetSeconds: target, Status: StatusPlanned, VoiceID: voiceA, VisualStyle: StyleAuto,
		DialogueMode: DialogueNarration, VoiceB: voiceB, StoryFormat: FormatExplainer,
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
	tools, err := s.speechTools()
	if err != nil {
		return nil, err
	}
	lang, ok := speechLang(ch.Language)
	if !ok {
		return nil, &UnsupportedLanguageError{Language: ch.Language}
	}
	narrator := Narrator{Tools: tools, Runner: s.runner, CacheDir: filepath.Join(s.mediaDir, "tts-cache"), Pace: paceOf(v.StoryFormat), Lang: lang}
	lex, err := LoadLexicon(tools.LexiconPath())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrToolsMissing, err)
	}
	if tools.ExtraLexicon != "" {
		extra, err := LoadLexicon(tools.ExtraLexicon)
		if err != nil {
			return nil, err
		}
		for w := range extra {
			lex[w] = true
		}
	}
	// Normalize every segment first so all unpronounceable words are
	// reported at once, before any audio is generated.
	segments := v.Script.Segments()
	spoken := make([]string, len(segments))
	var unspeakable []string
	seenOOV := map[string]bool{}
	for i, seg := range segments {
		if lang != "" {
			// espeak-ng reads numbers and symbols in its own language; the
			// English lexicon check does not apply.
			spoken[i] = cleanForSpeech(seg.Narration)
			continue
		}
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
	aligner := Aligner{Exe: s.tools.ASR, ModelDir: s.tools.ASRModel, Runner: s.runner}
	timing := "estimated"
	if aligner.Available() && lang == "" {
		// The alignment model is English-only; other languages keep
		// estimated word times.
		timing = "measured"
	}
	var segs []Timed
	var audios []string
	var kickers []string
	total := 0.0
	for i, seg := range segments {
		wav, err := narrator.Synthesize(ctx, v.voiceFor(seg.Speaker), spoken[i])
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
		tm := Timed{Audio: wav, Duration: d, Title: title, Text: seg.Narration, Speaker: seg.Speaker}
		if timing == "measured" {
			// Alignment only improves captions; an error falls back to estimates.
			if words, err := aligner.WordTimes(ctx, wav, seg.Narration, d); err == nil {
				tm.Words = words
			} else {
				timing = "estimated"
			}
		}
		segs = append(segs, tm)
		kicker := ""
		if seg.Heading != "" && seg.Heading != title {
			kicker = seg.Heading
		}
		kickers = append(kickers, kicker)
		audios = append(audios, wav)
		total += d
	}
	width, height := v.Dimensions()
	style := Style{Background: "#0F172A", Text: "#F8FAFC", Accent: "#60A5FA", Font: "Inter"}
	secondary := "#1E3A8A"
	if idn := ch.Identity; idn != nil {
		style.Background, style.Text, style.Accent = idn.Palette.Primary, idn.Palette.Background, idn.Palette.Accent
		secondary = idn.Palette.Secondary
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

	// Animated scenes: render the picture with HyperFrames, then mix narration
	// and captions over it. Fall back to the solid background if unavailable.
	visual, visuals, note := "", "solid", ""
	resolved := ResolveStyle(v.VisualStyle, ch.Niche)
	scenes := PlanScenes(segs, kickers, ch.Name)
	var credits []stock.Clip
	if resolved == StyleFootage {
		visual, credits, err = s.renderFootage(rctx, dir, segs, ch.Niche, width, height)
		if err != nil {
			// Fall back to animated scenes so the video still renders.
			visual, credits, note = "", nil, "Stock footage was not available, so animated scenes were used: "+truncateErr(err, 300)
			resolved = StyleMinimal
		} else {
			visuals = FootageLabel(credits)
		}
	}
	var pops []ImagePop
	var wordFX []SFXCue
	background := ""
	if resolved == StyleGameplay && s.tools.AnimatedScenes() {
		// The owner's own clip replaces the drawn game when there is one.
		var name string
		if background, name, err = s.prepareBackground(rctx, dir, seedOf(v.ID), total, width, height); err != nil {
			note = strings.TrimSpace(note + " The background clip was skipped: " + truncateErr(err, 200))
			background = ""
		} else if name != "" {
			note = strings.TrimSpace(note + " Background clip: " + name + ".")
		}
	}
	captions := captionsFor(resolved, segs, width, height)
	var build *BuildData
	if resolved == StyleBuild3D && s.tools.AnimatedScenes() {
		build = buildTiming(segs, sceneFor(v))
	}
	if storyStyle(resolved) && s.tools.AnimatedScenes() {
		// Key words: a photo pops up on each fix word, and captions color
		// fixes yellow and problems red. The story renders without them.
		plans := s.planPops(rctx, segs)
		wordFX = PlanWordEffects(segs)
		fixes, problems := captionMarks(segs, plans)
		captions = MarkCaptions(captions, fixes, problems)
		if resolved == StyleGameplay && s.FootageAvailable() {
			// The 3D build shows its pieces instead of photos.
			if pops, err = s.planImagePops(rctx, segs, plans, wordFX, width, height); err != nil {
				note = strings.TrimSpace(note + " Pop-up images were skipped: " + truncateErr(err, 200))
			}
			credits = PopCredits(pops)
		}
	}
	if resolved != StyleFootage && s.tools.AnimatedScenes() {
		visual, err = s.renderScenes(rctx, dir, ProjectInput{
			Width: width, Height: height, Style: resolved, Palette: style, Secondary: secondary,
			ChannelName: ch.Name, Scenes: scenes, Captions: captions, Seed: seedOf(v.ID),
			Pops: pops, HookText: hookText(segs), Background: background, FollowText: followText(ch.Language, ch.Name),
			Build: build, ThreeJS: s.tools.ThreeJS,
		})
		if err != nil {
			visual, note = "", "Animated scenes failed, so a plain background was used: "+truncateErr(err, 300)
		} else {
			visuals = "animated:" + resolved
		}
	} else if resolved != StyleFootage {
		note = strings.TrimSpace(note + " Animated scenes are not installed (run npm install in renderer/).")
	}
	// Animated scenes carry their own titles and word-by-word captions; the
	// plain fallback burns in titles and captions with ASS subtitles.
	assFile := ""
	switch {
	case resolved == StyleFootage:
		// Stock footage has no text of its own: one word at a time on top.
		assFile = "captions.ass"
		if err := os.WriteFile(filepath.Join(dir, assFile), []byte(BuildWordASS(WordCaptions(segs), width, height)), 0o644); err != nil {
			return nil, fmt.Errorf("write subtitles: %w", err)
		}
	case visual == "":
		assFile = "captions.ass"
		if err := os.WriteFile(filepath.Join(dir, assFile), []byte(BuildASS(segs, width, height, style)), 0o644); err != nil {
			return nil, fmt.Errorf("write subtitles: %w", err)
		}
	}
	// Soundtrack: an original music bed and effects synthesized in code.
	// Formats that want immersion drop the music and lean on effects.
	mood := MoodFor(resolved)
	mix := AudioMix{Voice: audios, Music: "music.wav", Effects: "effects.wav"}
	music := "Original score generated in code (" + mood.Description + "); no third-party audio."
	if musicOff(v.StoryFormat) {
		mix.Music, music = "", "No music; sound effects generated in code."
	} else if err := ComposeMusic(mood, total+1, v.ID+":"+mood.Name).WriteWAV(filepath.Join(dir, mix.Music)); err != nil {
		return nil, fmt.Errorf("compose music: %w", err)
	}
	effects := PlanEffects(scenes, resolved)
	if storyStyle(resolved) {
		// Sounds on the words that name them ("burning" sizzles).
		effects = append(effects, wordFX...)
	}
	if visual == "" {
		// Without animated scenes there are no transitions to accent.
		effects = nil
	}
	if err := RenderEffects(effects, total+1, v.ID).WriteWAV(filepath.Join(dir, mix.Effects)); err != nil {
		return nil, fmt.Errorf("render effects: %w", err)
	}
	tmp := "video.tmp.mp4"
	if _, err := s.runner.Run(rctx, dir, s.tools.FFmpeg, FFmpegArgs(visual, mix, total, width, height, style.Background, assFile, fontsRel, tmp)...); err != nil {
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
		Visuals: visuals, VisualsNote: note,
		Music:        music,
		SoundEffects: len(effects), CaptionTiming: timing, Footage: credits,
	}, nil
}

// hookText is the whole hook (both sentences of a setup and punchline)
// for the gameplay story's hook card, capped at 140 characters.
func hookText(segs []Timed) string {
	if len(segs) == 0 {
		return ""
	}
	if r := []rune(strings.TrimSpace(segs[0].Text)); len(r) > 140 {
		return strings.TrimSpace(string(r[:140])) + "…"
	}
	return strings.TrimSpace(segs[0].Text)
}

// captionsFor picks the caption layout: one word at a time for the
// gameplay story, short lines with a highlighted word otherwise.
func captionsFor(style string, segs []Timed, width, height int) []CaptionChunk {
	if storyStyle(style) {
		return WordCaptions(segs)
	}
	return CaptionTrack(segs, captionWords(width, height))
}

// captionWords is the caption line length: short punchy lines for
// vertical video, longer ones for landscape.
func captionWords(width, height int) int {
	if height > width {
		return 3
	}
	return 6
}

// seedOf derives a stable 32-bit seed from an ID.
func seedOf(id string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(id))
	return h.Sum32()
}

// speechTools returns the tools with the supplement lexicon attached,
// building it once when the v0.19 fallback lexicon is installed.
func (s *Service) speechTools() (Tools, error) {
	tools := s.tools
	fb := tools.FallbackLexicon()
	if fb == "" {
		return tools, nil
	}
	sup := filepath.Join(s.mediaDir, "tts-cache", "lexicon-supplement-"+filepath.Base(tools.ModelDir)+".txt")
	if _, err := os.Stat(sup); err != nil {
		if _, err := BuildSupplementLexicon(tools.LexiconPath(), fb, filepath.Join(tools.ModelDir, "tokens.txt"), sup); err != nil {
			return tools, fmt.Errorf("build supplement lexicon: %w", err)
		}
	}
	tools.ExtraLexicon = sup
	return tools, nil
}

// renderScenes writes a HyperFrames project and renders it to visual.mp4
// (picture only; narration and captions are added by FFmpeg afterwards).
func (s *Service) renderScenes(ctx context.Context, dir string, in ProjectInput) (string, error) {
	project := filepath.Join(dir, "scenes")
	_ = os.RemoveAll(project)
	if err := WriteProject(project, in, s.tools.GSAP, s.tools.WebFonts); err != nil {
		return "", err
	}
	out := filepath.Join(dir, "visual.mp4")
	_ = os.Remove(out)
	if _, err := s.runner.Run(ctx, project, s.tools.Node, s.tools.HyperFrames, "render", project,
		"--output", out, "--quality", "looks", "--fps", "30", "--quiet"); err != nil {
		return "", fmt.Errorf("render scenes: %w", err)
	}
	if st, err := os.Stat(out); err != nil || st.Size() == 0 {
		return "", errors.New("render scenes: no output file")
	}
	return out, nil
}

func truncateErr(err error, n int) string {
	msg := err.Error()
	if len(msg) > n {
		return msg[:n]
	}
	return msg
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

// SetStyle chooses the visual style of the animated scenes.
func (s *Service) SetStyle(ctx context.Context, ownerID, id, style string) (*Video, error) {
	if !ValidStyle(style) {
		return nil, &channel.ValidationError{Field: "visualStyle", Message: "is not a known style"}
	}
	return s.mutate(ctx, ownerID, id, func(v *Video) error {
		if v.Status == StatusProducing {
			return &StateError{Status: v.Status, Action: "change the style", Reason: "production is running"}
		}
		v.VisualStyle = style
		return nil
	})
}
