package production

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListAndPickBackgrounds(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"b.MP4", "a.webm", "notes.txt", "c.gif"} {
		_ = os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644)
	}
	_ = os.Mkdir(filepath.Join(dir, "sub.mp4"), 0o755)
	clips := listBackgrounds(dir)
	if len(clips) != 2 || filepath.Base(clips[0]) != "a.webm" || filepath.Base(clips[1]) != "b.MP4" {
		t.Fatalf("clips = %v", clips)
	}
	if got := listBackgrounds(filepath.Join(dir, "missing")); got != nil {
		t.Errorf("missing dir = %v", got)
	}

	src, off := pickBackground(clips, []float64{300, 10}, 4, 30)
	if filepath.Base(src) != "a.webm" || off < 1 || off+30 > 300 {
		t.Errorf("long clip: %s at %v", src, off)
	}
	if src2, off2 := pickBackground(clips, []float64{300, 10}, 4, 30); src2 != src || off2 != off {
		t.Error("picks must be repeatable for a seed")
	}
	if src, off := pickBackground(clips, []float64{300, 10}, 5, 30); filepath.Base(src) != "b.MP4" || off != 0 {
		t.Errorf("short clip must start at 0 and loop: %s %v", src, off)
	}
	if src, _ := pickBackground(nil, nil, 1, 30); src != "" {
		t.Error("no clips, no background")
	}
}

func TestBackgroundArgsCropAndMute(t *testing.T) {
	args := strings.Join(BackgroundArgs("in.mov", 12.5, 30.5, 1080, 1920, "out.mp4"), " ")
	for _, want := range []string{"-stream_loop -1 -ss 12.500 -i in.mov", "-t 30.500", "-an",
		"scale=1080:1920:force_original_aspect_ratio=increase,crop=1080:1920", "fps=30", "-g 15", "out.mp4"} {
		if !strings.Contains(args, want) {
			t.Errorf("missing %q in %s", want, args)
		}
	}
}

func TestWriteProjectWithBackgroundClip(t *testing.T) {
	root := t.TempDir()
	gsap := filepath.Join(root, "gsap.min.js")
	_ = os.WriteFile(gsap, []byte("/*gsap*/"), 0o644)
	fonts := filepath.Join(root, "fontsource")
	for _, f := range projectFonts {
		p := filepath.Join(fonts, f.pkg, "files", f.file)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte("font"), 0o644)
	}
	bg := filepath.Join(root, "background.mp4")
	_ = os.WriteFile(bg, []byte("mp4"), 0o644)
	segs := []Timed{{Duration: 3, Text: "You prank your friends."}}
	in := ProjectInput{Width: 1080, Height: 1920, Style: StyleGameplay, Scenes: PlanScenes(segs, nil, "Q"), Captions: WordCaptions(segs), Background: bg}
	dir := filepath.Join(root, "project")
	if err := WriteProject(dir, in, gsap, fonts); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	html := string(b)
	if !strings.Contains(html, `<video id="bgv" class="clip" src="assets/bg.mp4" muted`) || strings.Contains(html, `<canvas id="game"`) {
		t.Error("the clip must replace the drawn game")
	}
	if _, err := os.Stat(filepath.Join(dir, "assets", "bg.mp4")); err != nil {
		t.Error("background not copied into the project")
	}
}

func TestCutStarts(t *testing.T) {
	got := cutStarts(638, 60, 10)
	if len(got) != 10 || got[0] != 2 || got[9]+60 > 636+1e-9 {
		t.Fatalf("starts = %v", got)
	}
	for i := 1; i < len(got); i++ {
		if got[i]-got[i-1] < 60 {
			t.Fatalf("clips overlap: %v", got)
		}
	}
	if got := cutStarts(150, 60, 10); len(got) != 2 {
		t.Errorf("2.5 minutes fits 2 clips, got %v", got)
	}
	if got := cutStarts(63, 60, 10); len(got) != 0 {
		t.Errorf("too short = %v", got)
	}
}

func TestCutNamesNeverClash(t *testing.T) {
	a := cutName("x/vidssave.com Minecraft Parkour Gameplay No Copyright 4K 720P.mp4", 1)
	b := cutName("x/vidssave.com Minecraft Parkour Gameplay No Copyright 4K 720P (1).mp4", 1)
	if a == b || !strings.HasSuffix(a, "-01.mp4") || strings.ContainsAny(a, " ()") {
		t.Errorf("names %q and %q", a, b)
	}
	if cutName("x/!!!.mp4", 3) == "" {
		t.Error("empty base name")
	}
}

func TestCutBackgrounds(t *testing.T) {
	media := t.TempDir()
	src := filepath.Join(media, BackgroundsDir, SourcesDir)
	_ = os.MkdirAll(src, 0o755)
	_ = os.WriteFile(filepath.Join(src, "parkour.mp4"), []byte("x"), 0o644)
	r := cutRunner{}
	made, err := CutBackgrounds(context.Background(), Tools{FFmpeg: "ffmpeg", FFprobe: "ffprobe"}, r, media, 10, 60, 1080, 1920)
	if err != nil || len(made) != 3 {
		t.Fatalf("made %v, %v (190 s fits 3 clips)", made, err)
	}
	if !filepath.IsAbs(made[0]) {
		t.Errorf("clip paths must be absolute, got %s", made[0])
	}
	clips := listBackgrounds(filepath.Join(media, BackgroundsDir))
	if len(clips) != 3 {
		t.Fatalf("clips ready for renders = %v", clips)
	}
	// Re-running keeps finished clips.
	again, err := CutBackgrounds(context.Background(), Tools{FFmpeg: "ffmpeg", FFprobe: "ffprobe"}, r, media, 10, 60, 1080, 1920)
	if err != nil || len(again) != 0 {
		t.Errorf("re-run made %v, %v", again, err)
	}
}

// cutRunner reports a 190 s source to ffprobe and writes ffmpeg outputs.
type cutRunner struct{}

func (cutRunner) Run(_ context.Context, _, name string, args ...string) ([]byte, error) {
	if name == "ffprobe" {
		return []byte("190\n"), nil
	}
	return nil, os.WriteFile(args[len(args)-1], []byte("clip"), 0o644)
}
