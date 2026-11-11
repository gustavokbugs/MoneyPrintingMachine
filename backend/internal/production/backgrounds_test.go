package production

import (
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
