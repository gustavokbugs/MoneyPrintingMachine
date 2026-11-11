package production

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// ErrToolsMissing means FFmpeg, the TTS binary, or the voice model is not
// installed. Run scripts/setup-media-tools.ps1.
var ErrToolsMissing = errors.New("media tools are not installed; run scripts/setup-media-tools.ps1")

// Tools locates the external media tools.
type Tools struct {
	FFmpeg   string
	FFprobe  string
	TTS      string // sherpa-onnx-offline-tts executable
	ModelDir string // Kokoro model directory
	FontsDir string // directory with Inter TTF files
}

// ToolsFromRoot returns the default layout created by the setup script
// under root (the repository's .local directory).
func ToolsFromRoot(root string) Tools {
	exe := func(p string) string { return filepath.Join(root, p) }
	return Tools{
		FFmpeg:   exe(filepath.Join("ffmpeg", "bin", "ffmpeg.exe")),
		FFprobe:  exe(filepath.Join("ffmpeg", "bin", "ffprobe.exe")),
		TTS:      exe(filepath.Join("sherpa-onnx", "bin", "sherpa-onnx-offline-tts.exe")),
		ModelDir: exe(filepath.Join("models", "kokoro-int8-en-v0_19")),
		FontsDir: exe("fonts"),
	}
}

// Missing lists tools that are not present on disk.
func (t Tools) Missing() []string {
	var out []string
	for name, p := range map[string]string{"ffmpeg": t.FFmpeg, "ffprobe": t.FFprobe, "tts": t.TTS, "voice model": filepath.Join(t.ModelDir, "voices.bin"), "lexicon": filepath.Join(t.ModelDir, "lexicon-en-us.txt"), "fonts": filepath.Join(t.FontsDir, "Inter-Bold.ttf")} {
		if _, err := os.Stat(p); err != nil {
			out = append(out, name)
		}
	}
	return out
}

// Voices are the Kokoro v0.19 English speakers (speaker ID → name).
var Voices = []struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}{
	{0, "Default (American female)"}, {1, "Bella (American female)"}, {2, "Nicole (American female)"},
	{3, "Sarah (American female)"}, {4, "Sky (American female)"}, {5, "Adam (American male)"},
	{6, "Michael (American male)"}, {7, "Emma (British female)"}, {8, "Isabella (British female)"},
	{9, "George (British male)"}, {10, "Lewis (British male)"},
}

// ValidVoice reports whether id is a known speaker.
func ValidVoice(id int) bool { return id >= 0 && id < len(Voices) }

// Runner executes an external program with an argument list (no shell).
type Runner interface {
	Run(ctx context.Context, dir, name string, args ...string) ([]byte, error)
}

// ExecRunner runs real processes. Output is bounded to keep errors small.
type ExecRunner struct{}

// Run executes name with args in dir.
func (ExecRunner) Run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &limitedWriter{w: &out, n: 64 << 10}
	cmd.Stderr = cmd.Stdout
	err := cmd.Run()
	if err != nil {
		return out.Bytes(), fmt.Errorf("%s failed: %w: %s", filepath.Base(name), err, lastLines(out.String(), 5))
	}
	return out.Bytes(), nil
}

type limitedWriter struct {
	w *bytes.Buffer
	n int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if room := l.n - l.w.Len(); room > 0 {
		if len(p) > room {
			l.w.Write(p[:room])
		} else {
			l.w.Write(p)
		}
	}
	return len(p), nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}

// cleanForSpeech removes control characters and collapses whitespace so
// the text is safe to pass as a single argument.
func cleanForSpeech(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

// modelFile picks the Kokoro ONNX file in the model directory.
func modelFile(dir string) (string, error) {
	for _, name := range []string{"model.int8.onnx", "model.onnx"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return name, nil
		}
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*.onnx"))
	if len(matches) == 0 {
		return "", ErrToolsMissing
	}
	return filepath.Base(matches[0]), nil
}

// TTSArgs builds the sherpa-onnx command line for one utterance.
func TTSArgs(tools Tools, model string, voice int, text, out string) []string {
	return []string{
		"--kokoro-model=" + filepath.Join(tools.ModelDir, model),
		"--kokoro-voices=" + filepath.Join(tools.ModelDir, "voices.bin"),
		"--kokoro-tokens=" + filepath.Join(tools.ModelDir, "tokens.txt"),
		"--kokoro-lexicon=" + filepath.Join(tools.ModelDir, "lexicon-en-us.txt"),
		"--num-threads=4",
		"--sid=" + strconv.Itoa(voice),
		"--output-filename=" + out,
		text,
	}
}

// Narrator synthesizes speech with a content-addressed cache, so unchanged
// segments are never synthesized twice.
type Narrator struct {
	Tools    Tools
	Runner   Runner
	CacheDir string
}

// CacheKey identifies an utterance by model, voice, and text.
func CacheKey(model string, voice int, text string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%s", model, voice, text)))
	return hex.EncodeToString(sum[:16])
}

// Synthesize returns the path of a WAV file for text.
func (n Narrator) Synthesize(ctx context.Context, voice int, text string) (string, error) {
	text = cleanForSpeech(text)
	if text == "" {
		return "", errors.New("nothing to narrate")
	}
	model, err := modelFile(n.Tools.ModelDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(n.CacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create tts cache: %w", err)
	}
	out := filepath.Join(n.CacheDir, CacheKey(model, voice, text)+".wav")
	if st, err := os.Stat(out); err == nil && st.Size() > 44 {
		return out, nil
	}
	tmp := out + ".tmp.wav"
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if _, err := n.Runner.Run(ctx, n.CacheDir, n.Tools.TTS, TTSArgs(n.Tools, model, voice, text, tmp)...); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, out); err != nil {
		return "", fmt.Errorf("store narration: %w", err)
	}
	return out, nil
}

// ProbeDuration returns a media file's duration in seconds.
func ProbeDuration(ctx context.Context, r Runner, ffprobe, path string) (float64, error) {
	out, err := r.Run(ctx, filepath.Dir(path), ffprobe, "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", path)
	if err != nil {
		return 0, err
	}
	d, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("could not read duration of %s", filepath.Base(path))
	}
	return d, nil
}
