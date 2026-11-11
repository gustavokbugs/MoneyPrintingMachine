package production

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"contentfactory/internal/channel"
	"contentfactory/internal/jobs"
	"contentfactory/internal/llm"
)

type memVideos struct{ items map[string]Video }

func (m *memVideos) Create(_ context.Context, v *Video) error {
	for _, x := range m.items {
		if x.ChannelID == v.ChannelID && x.IdeaIndex == v.IdeaIndex {
			return ErrExists
		}
	}
	v.ID, v.Version = "vid-1", 1
	m.items[v.ID] = *v
	return nil
}
func (m *memVideos) Get(_ context.Context, owner, id string) (*Video, error) {
	v, ok := m.items[id]
	if !ok || v.OwnerID != owner {
		return nil, ErrNotFound
	}
	if v.Script != nil {
		sc := *v.Script
		sc.Claims = append([]Claim(nil), sc.Claims...)
		v.Script = &sc
	}
	return &v, nil
}
func (m *memVideos) ListByChannel(context.Context, string, string) ([]Video, error) { return nil, nil }
func (m *memVideos) Save(_ context.Context, v *Video) error {
	if m.items[v.ID].Version != v.Version {
		return ErrConflict
	}
	v.Version++
	m.items[v.ID] = *v
	return nil
}

type fixedChannels struct{ ch *channel.Channel }

func (f fixedChannels) Get(_ context.Context, owner, id string) (*channel.Channel, error) {
	if owner != f.ch.OwnerID || id != f.ch.ID {
		return nil, channel.ErrNotFound
	}
	return f.ch, nil
}

type scriptedGen struct {
	responses []string
	calls     int
	requests  []llm.Request
}

func (g *scriptedGen) Model() string { return "test-model" }
func (g *scriptedGen) GenerateJSON(_ context.Context, req llm.Request, out any) error {
	g.requests = append(g.requests, req)
	g.calls++
	if g.calls > len(g.responses) {
		return llm.ErrUnavailable
	}
	return llm.DecodeStrict(g.responses[g.calls-1], out)
}

func testChannel() *channel.Channel {
	return &channel.Channel{
		ID: "ch1", OwnerID: "alice", Name: "Quiet Orbit", Language: "en-US",
		Strategy: &channel.Strategy{TargetAudience: "Curious adults"},
		Identity: &channel.Identity{Palette: channel.Palette{Primary: "#1D3557", Background: "#F1FAEE", Accent: "#E63946"}},
		Blueprint: &channel.Blueprint{
			Formats: []channel.VideoFormat{{Name: "Short", AspectRatio: "9:16"}},
			Ideas:   []channel.ContentIdea{{WorkingTitle: "Why the Moon Drifts", Topic: "Lunar recession", Format: "Short", SuggestedDurationSecs: 60}},
		},
	}
}

const goodScript = `{"hook":"The Moon is leaving us.","sections":[{"heading":"Drift","narration":"Every year the Moon moves a little farther away.","onScreenText":"A slow escape"},{"heading":"Why","narration":"Tides transfer energy from Earth's spin to the Moon's orbit.","onScreenText":"Tidal push"}],"outro":"Subscribe for more.","claims":[{"text":"The Moon recedes about 3.8 cm per year.","sourceHint":"NASA lunar laser ranging"},{"text":"The Moon recedes about 3.8 cm per year.","sourceHint":"dup"}]}`

func newTestService(t *testing.T, gen Generator, runner Runner, tools Tools) (*Service, *memVideos) {
	t.Helper()
	store := &memVideos{items: map[string]Video{}}
	return NewService(store, fixedChannels{testChannel()}, gen, tools, runner, t.TempDir()), store
}

func jobFor(id string, attempts int) jobs.Job {
	in, _ := json.Marshal(map[string]string{"videoId": id})
	return jobs.Job{OwnerID: "alice", Input: in, Attempts: attempts, MaxAttempts: 3}
}

func TestCreateFromIdeaAndScriptJob(t *testing.T) {
	gen := &scriptedGen{responses: []string{`{"bad":true}`, goodScript}}
	svc, _ := newTestService(t, gen, nil, Tools{})
	ctx := context.Background()
	v, err := svc.CreateFromIdea(ctx, "alice", "ch1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if v.AspectRatio != "9:16" || v.TargetSeconds != 60 || v.Status != StatusPlanned || v.Title != "Why the Moon Drifts" {
		t.Fatalf("unexpected video: %+v", v)
	}
	if _, err := svc.CreateFromIdea(ctx, "alice", "ch1", 0); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate idea: %v", err)
	}
	if _, err := svc.CreateFromIdea(ctx, "alice", "ch1", 5); err == nil {
		t.Fatal("unknown idea index must fail")
	}
	if _, err := svc.CreateFromIdea(ctx, "bob", "ch1", 0); !errors.Is(err, channel.ErrNotFound) {
		t.Fatalf("other owner: %v", err)
	}

	if _, err := svc.Handlers()[KindScript](ctx, jobFor(v.ID, 1)); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Get(ctx, "alice", v.ID)
	if got.Status != StatusScriptReady || got.Script == nil || len(got.Script.Claims) != 1 || got.Script.Provenance.Model != "test-model" {
		t.Fatalf("script not stored correctly: %+v", got)
	}
	// Call 1 is invalid and re-sampled; call 2 is valid but far shorter
	// than the target, so it is retried once more (call 3, no answer) and
	// then kept as the best script.
	if gen.calls != 3 {
		t.Fatalf("calls=%d, want 3", gen.calls)
	}
}

func TestScriptJobFailureMarksVideoOnLastAttempt(t *testing.T) {
	svc, _ := newTestService(t, &scriptedGen{}, nil, Tools{})
	ctx := context.Background()
	v, _ := svc.CreateFromIdea(ctx, "alice", "ch1", 0)
	if _, err := svc.Handlers()[KindScript](ctx, jobFor(v.ID, 1)); err == nil || jobs.IsPermanent(err) {
		t.Fatalf("unavailable model must be retryable: %v", err)
	}
	if got, _ := svc.Get(ctx, "alice", v.ID); got.Status != StatusScripting {
		t.Fatalf("intermediate failure keeps scripting status, got %s", got.Status)
	}
	_, _ = svc.Handlers()[KindScript](ctx, jobFor(v.ID, 3))
	if got, _ := svc.Get(ctx, "alice", v.ID); got.Status != StatusFailed || got.StatusReason == "" {
		t.Fatalf("last attempt must mark failed: %+v", got)
	}
}

func TestEditScriptCarriesClaimReviews(t *testing.T) {
	svc, _ := newTestService(t, &scriptedGen{responses: []string{goodScript}}, nil, Tools{})
	ctx := context.Background()
	v, _ := svc.CreateFromIdea(ctx, "alice", "ch1", 0)
	_, _ = svc.Handlers()[KindScript](ctx, jobFor(v.ID, 1))
	if _, err := svc.ReviewClaim(ctx, "alice", v.ID, 0, ClaimVerified, "checked"); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Get(ctx, "alice", v.ID)
	edited := *got.Script
	edited.Claims = append(edited.Claims, Claim{Text: "New claim", Status: ClaimVerified})
	res, err := svc.EditScript(ctx, "alice", v.ID, edited)
	if err != nil {
		t.Fatal(err)
	}
	if res.Script.Claims[0].Status != ClaimVerified || res.Script.Claims[1].Status != ClaimUnverified || !res.Script.Provenance.EditedByUser {
		t.Fatalf("claims after edit: %+v", res.Script.Claims)
	}
	if _, err := svc.Approve(ctx, "alice", v.ID); err == nil {
		t.Fatal("new claim must block approval")
	}
}

// fakeTools creates the files the real tools would, so Produce can run.
func fakeTools(t *testing.T) (Tools, *recordRunner) {
	root := t.TempDir()
	tools := ToolsFromRoot(root)
	for _, p := range []string{tools.FFmpeg, tools.FFprobe, tools.TTS, filepath.Join(tools.ModelDir, "voices.bin"), filepath.Join(tools.ModelDir, "model.int8.onnx"), filepath.Join(tools.ModelDir, "lexicon-en-us.txt"), filepath.Join(tools.FontsDir, "Inter-Bold.ttf")} {
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte("x"), 0o644)
	}
	var words []string
	for _, w := range strings.Fields(strings.ToLower("the moon is leaving us every year moves a little farther away tides transfer energy from earth spin to orbit subscribe for more")) {
		words = append(words, w+" || x")
	}
	_ = os.WriteFile(filepath.Join(tools.ModelDir, "lexicon-en-us.txt"), []byte(strings.Join(words, "\n")), 0o644)
	r := &recordRunner{}
	r.write = func(args []string) {
		for _, a := range args {
			if p, ok := strings.CutPrefix(a, "--output-filename="); ok {
				_ = os.WriteFile(p, make([]byte, 100), 0o644)
			}
		}
	}
	return tools, r
}

func TestProduceRequiresApprovalAndRenders(t *testing.T) {
	tools, runner := fakeTools(t)
	svc, _ := newTestService(t, &scriptedGen{responses: []string{goodScript}}, runner, tools)
	ctx := context.Background()
	v, _ := svc.CreateFromIdea(ctx, "alice", "ch1", 0)
	_, _ = svc.Handlers()[KindScript](ctx, jobFor(v.ID, 1))

	if _, err := svc.CheckCanProduce(ctx, "alice", v.ID); err == nil {
		t.Fatal("production must require approval")
	}
	if _, err := svc.Handlers()[KindProduce](ctx, jobFor(v.ID, 1)); !jobs.IsPermanent(err) {
		t.Fatalf("producing an unapproved video must fail permanently: %v", err)
	}
	_, _ = svc.ReviewClaim(ctx, "alice", v.ID, 0, ClaimVerified, "")
	if _, err := svc.Approve(ctx, "alice", v.ID); err != nil {
		t.Fatal(err)
	}

	// The renderer writes the temp output that Produce renames.
	videoDir := svc.VideoDir(v.ID)
	prevWrite := runner.write
	runner.write = func(args []string) {
		prevWrite(args)
		if len(args) > 0 && args[len(args)-1] == "video.tmp.mp4" {
			_ = os.WriteFile(filepath.Join(videoDir, "video.tmp.mp4"), []byte("mp4"), 0o644)
		}
	}
	if _, err := svc.Handlers()[KindProduce](ctx, jobFor(v.ID, 1)); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Get(ctx, "alice", v.ID)
	if got.Status != StatusProduced || got.Output == nil || got.Output.Width != 1080 || !got.Output.SyntheticVoice {
		t.Fatalf("produced video: %+v", got)
	}
	ass, _ := os.ReadFile(filepath.Join(videoDir, "captions.ass"))
	if !strings.Contains(string(ass), "A slow escape") {
		t.Fatal("subtitles missing section titles")
	}
	// 4 segments narrated (hook, 2 sections, outro), each probed, then render and final probe.
	ttsCalls := 0
	for _, c := range runner.calls {
		if c[0] == tools.TTS {
			ttsCalls++
		}
	}
	if ttsCalls != 4 {
		t.Fatalf("tts calls = %d", ttsCalls)
	}
	p, err := svc.MediaPath(ctx, "alice", v.ID, "video.mp4")
	if err != nil || filepath.Base(p) != "video.mp4" {
		t.Fatalf("media path: %v %v", p, err)
	}
	for _, bad := range []string{"../../.env", "captions.ass", "video.tmp.mp4"} {
		if _, err := svc.MediaPath(ctx, "alice", v.ID, bad); !errors.Is(err, ErrNotFound) {
			t.Errorf("%q must not be served", bad)
		}
	}
	if _, err := svc.MediaPath(ctx, "bob", v.ID, "video.mp4"); !errors.Is(err, ErrNotFound) {
		t.Fatal("media must be owner-scoped")
	}
}

func TestProduceStopsOnUnspeakableWords(t *testing.T) {
	tools, runner := fakeTools(t)
	_ = os.WriteFile(filepath.Join(tools.ModelDir, "lexicon-en-us.txt"), []byte("hello || x\n"), 0o644)
	svc, store := newTestService(t, &scriptedGen{}, runner, tools)
	ctx := context.Background()
	v, _ := svc.CreateFromIdea(ctx, "alice", "ch1", 0)
	sv := store.items[v.ID]
	sv.Status = StatusApproved
	sv.Script = &Script{Sections: []Section{{Narration: "hello Zorblax"}}}
	store.items[v.ID] = sv
	_, err := svc.Handlers()[KindProduce](ctx, jobFor(v.ID, 1))
	var ue *UnspeakableError
	if !errors.As(err, &ue) || !jobs.IsPermanent(err) || ue.Words[0] != "Zorblax" {
		t.Fatalf("expected permanent unspeakable error, got %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatal("no audio may be generated when words are unpronounceable")
	}
	if got, _ := svc.Get(ctx, "alice", v.ID); got.Status != StatusFailed || !strings.Contains(got.StatusReason, "Zorblax") {
		t.Fatalf("video must name the word: %+v", got)
	}
}

func TestProduceWithoutToolsIsPermanent(t *testing.T) {
	svc, store := newTestService(t, &scriptedGen{}, nil, ToolsFromRoot(t.TempDir()))
	ctx := context.Background()
	v, _ := svc.CreateFromIdea(ctx, "alice", "ch1", 0)
	sv := store.items[v.ID]
	sv.Status = StatusApproved
	sv.Script = &Script{Sections: []Section{{Narration: "hi"}}}
	store.items[v.ID] = sv
	if _, err := svc.CheckCanProduce(ctx, "alice", v.ID); !errors.Is(err, ErrToolsMissing) {
		t.Fatalf("expected tools missing, got %v", err)
	}
	if _, err := svc.Handlers()[KindProduce](ctx, jobFor(v.ID, 1)); !jobs.IsPermanent(err) {
		t.Fatalf("missing tools must fail permanently: %v", err)
	}
	if got, _ := svc.Get(ctx, "alice", v.ID); got.Status != StatusFailed || !strings.Contains(got.StatusReason, "setup-media-tools") {
		t.Fatalf("video must explain the missing tools: %+v", got)
	}
}

func TestMediaDirIsAbsolute(t *testing.T) {
	svc := NewService(&memVideos{items: map[string]Video{}}, fixedChannels{testChannel()}, &scriptedGen{}, Tools{}, nil, filepath.Join("..", ".local", "media"))
	if !filepath.IsAbs(svc.VideoDir("x")) {
		t.Fatalf("video dir must be absolute: %s", svc.VideoDir("x"))
	}
}

func TestSetVoice(t *testing.T) {
	svc, _ := newTestService(t, &scriptedGen{}, nil, Tools{})
	ctx := context.Background()
	v, _ := svc.CreateFromIdea(ctx, "alice", "ch1", 0)
	if _, err := svc.SetVoice(ctx, "alice", v.ID, 99); err == nil {
		t.Fatal("unknown voice must fail")
	}
	got, err := svc.SetVoice(ctx, "alice", v.ID, 9)
	if err != nil || got.VoiceID != 9 {
		t.Fatalf("set voice: %+v %v", got, err)
	}
}
