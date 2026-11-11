package production

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaptionChunks(t *testing.T) {
	got := CaptionChunks("The Moon drifts away. It moves about 3.8 centimeters every single year, slowly.", 6)
	want := []string{"The Moon drifts away.", "It moves about 3.8 centimeters every", "single year, slowly."}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("chunks = %q", got)
	}
	if len(CaptionChunks("", 6)) != 0 {
		t.Fatal("empty text yields no chunks")
	}
}

func TestCuesCoverEachSegment(t *testing.T) {
	segs := []Timed{{Duration: 4, Text: "one two three four five six seven eight nine ten"}, {Duration: 2, Text: "short"}}
	cues := Cues(segs, 5)
	if len(cues) != 3 || cues[0].Start != 0 || cues[1].End != 4 || cues[2].Start != 4 || cues[2].End != 6 {
		t.Fatalf("cues = %+v", cues)
	}
}

func TestBuildASSEscapesAndTimes(t *testing.T) {
	segs := []Timed{{Duration: 61.234, Title: `Evil {\b1}title`, Text: `caption {\pos(1,1)} text`}}
	ass := BuildASS(segs, 1920, 1080, Style{Background: "#1D3557", Text: "#F1FAEE", Accent: "#E63946", Font: "Inter"})
	if strings.Contains(ass, `{\b1}`) || strings.Contains(ass, `{\pos`) {
		t.Fatalf("override tags from content must be neutralized:\n%s", ass)
	}
	if !strings.Contains(ass, "0:00:00.00,0:01:01.23,Title") {
		t.Fatalf("title timing missing:\n%s", ass)
	}
	// #F1FAEE -> &H00EEFAF1 (BGR order).
	if !strings.Contains(ass, "&H00EEFAF1") || !strings.Contains(ass, "PlayResX: 1920") {
		t.Fatalf("colors or resolution wrong:\n%s", ass)
	}
}

func TestBuildSRT(t *testing.T) {
	srt := BuildSRT([]Timed{{Duration: 3725.5, Text: "hello world"}})
	if !strings.HasPrefix(srt, "1\n00:00:00,000 --> 01:02:05,500\nhello world\n") {
		t.Fatalf("srt = %q", srt)
	}
}

func TestFFmpegArgs(t *testing.T) {
	args := FFmpegArgs("", AudioMix{Voice: []string{"C:/cache/a.wav", "C:/cache/b.wav"}}, 12.3, 1080, 1920, "#0F172A", "captions.ass", `..\..\fonts`, "video.mp4")
	joined := strings.Join(args, " ")
	for _, want := range []string{"color=c=0x0F172A:s=1080x1920:r=30:d=12.80", "[1:a][2:a]concat=n=2", "fontsdir='../../fonts'", "-c:v libx264", "video.mp4"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %s", want, joined)
		}
	}
	animated := strings.Join(FFmpegArgs("visual.mp4", AudioMix{Voice: []string{"a.wav"}}, 5, 1920, 1080, "#000000", "c.ass", "f", "o.mp4"), " ")
	if !strings.Contains(animated, "-i visual.mp4") || strings.Contains(animated, "lavfi") {
		t.Fatalf("animated visual must replace the solid background: %s", animated)
	}
	if filterPath("a:b,c'd") != `a\:b\,c\'d` {
		t.Fatalf("filterPath = %s", filterPath("a:b,c'd"))
	}
}

type recordRunner struct {
	calls [][]string
	write func(args []string)
}

func (r *recordRunner) Run(_ context.Context, _, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	if r.write != nil {
		r.write(args)
	}
	return []byte("12.5\n"), nil
}

func TestNarratorCachesAndPassesArgs(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "model")
	if err := os.MkdirAll(model, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(model, "model.int8.onnx"), []byte("x"), 0o644)
	r := &recordRunner{write: func(args []string) {
		for _, a := range args {
			if p, ok := strings.CutPrefix(a, "--output-filename="); ok {
				_ = os.WriteFile(p, make([]byte, 100), 0o644)
			}
		}
	}}
	n := Narrator{Tools: Tools{TTS: "tts.exe", ModelDir: model}, Runner: r, CacheDir: filepath.Join(dir, "cache")}
	text := "Hello\x00 world  \n again"
	p1, err := n.Synthesize(context.Background(), 6, text)
	if err != nil {
		t.Fatal(err)
	}
	p2, _ := n.Synthesize(context.Background(), 6, text)
	if p1 != p2 || len(r.calls) != 1 {
		t.Fatalf("expected one synthesis for identical text, got %d calls", len(r.calls))
	}
	call := r.calls[0]
	if call[len(call)-1] != "Hello world again" || !contains(call, "--sid=6") {
		t.Fatalf("unexpected args: %q", call)
	}
	if _, err := n.Synthesize(context.Background(), 6, "  "); err == nil {
		t.Fatal("empty text must fail")
	}
	d, err := ProbeDuration(context.Background(), r, "ffprobe", p1)
	if err != nil || d != 12.5 {
		t.Fatalf("probe = %v %v", d, err)
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func TestToolsFromRootIsAbsolute(t *testing.T) {
	// Tools run with cmd.Dir set elsewhere, so relative paths would break.
	tools := ToolsFromRoot(filepath.Join("..", ".local"))
	for _, p := range []string{tools.FFmpeg, tools.FFprobe, tools.TTS, tools.ModelDir, tools.FontsDir} {
		if !filepath.IsAbs(p) {
			t.Errorf("%s is not absolute", p)
		}
	}
}

func TestToolsMissing(t *testing.T) {
	if len(ToolsFromRoot(t.TempDir()).Missing()) != 6 {
		t.Fatal("all tools should be missing in an empty directory")
	}
}

func TestWithoutTitlesDropsTitleCards(t *testing.T) {
	segs := []Timed{{Duration: 2, Title: "Tidal Pull", Text: "Friction slows Earth."}}
	ass := BuildASS(WithoutTitles(segs), 1080, 1920, Style{})
	if strings.Contains(ass, ",Title,,") {
		t.Fatal("title card still present")
	}
	if !strings.Contains(ass, "Friction slows Earth.") {
		t.Fatal("captions missing")
	}
	if segs[0].Title != "Tidal Pull" {
		t.Fatal("input mutated")
	}
}
