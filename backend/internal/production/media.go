package production

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	// ExtraLexicon is a generated supplement lexicon (see lexicon.go).
	ExtraLexicon string
	FontsDir     string // directory with Inter TTF files
	// Word timing (optional): sherpa-onnx-offline with a Parakeet model.
	ASR      string
	ASRModel string

	// Animated scenes (optional): Node runs the HyperFrames CLI from the
	// renderer workspace; GSAP and web fonts are vendored there.
	Node        string
	HyperFrames string
	// TTSBridge narrates non-English text (renderer/tts.mjs).
	TTSBridge string
	// ThreeJS is three.js's CommonJS build, for the 3D build style.
	ThreeJS  string
	GSAP     string
	WebFonts string
}

// WithRenderer adds the HyperFrames workspace at dir (the repository's
// renderer folder) when Node and the installed packages are present.
func (t Tools) WithRenderer(dir string) Tools {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	node, err := exec.LookPath("node")
	if err != nil {
		return t
	}
	t.Node = node
	t.HyperFrames = filepath.Join(dir, "node_modules", "hyperframes", "bin", "hyperframes.mjs")
	t.GSAP = filepath.Join(dir, "node_modules", "gsap", "dist", "gsap.min.js")
	t.WebFonts = filepath.Join(dir, "node_modules", "@fontsource")
	if _, err := os.Stat(filepath.Join(dir, "node_modules", "three", "build", "three.cjs")); err == nil {
		t.ThreeJS = filepath.Join(dir, "node_modules", "three", "build", "three.cjs")
	}
	if _, err := os.Stat(filepath.Join(dir, "node_modules", "sherpa-onnx-node")); err == nil {
		t.TTSBridge = filepath.Join(dir, "tts.mjs")
	}
	return t
}

// AnimatedScenes reports whether HyperFrames rendering is available.
func (t Tools) AnimatedScenes() bool {
	if t.Node == "" {
		return false
	}
	for _, p := range []string{t.HyperFrames, t.GSAP, filepath.Join(t.WebFonts, "inter")} {
		if _, err := os.Stat(p); err != nil {
			return false
		}
	}
	return true
}

// ToolsFromRoot returns the default layout created by the setup script
// under root (the repository's .local directory). Paths are made absolute
// because tools run with a different working directory.
func ToolsFromRoot(root string) Tools {
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	exe := func(p string) string { return filepath.Join(root, p) }
	return Tools{
		FFmpeg:   exe(filepath.Join("ffmpeg", "bin", "ffmpeg.exe")),
		FFprobe:  exe(filepath.Join("ffmpeg", "bin", "ffprobe.exe")),
		TTS:      exe(filepath.Join("sherpa-onnx", "bin", "sherpa-onnx-offline-tts.exe")),
		ModelDir: exe(filepath.Join("models", "kokoro-multi-lang-v1_0")),
		FontsDir: exe("fonts"),
		ASR:      exe(filepath.Join("sherpa-onnx", "bin", "sherpa-onnx-offline.exe")),
		ASRModel: exe(filepath.Join("models", AlignerModelDir)),
	}
}

// Missing lists tools that are not present on disk.
func (t Tools) Missing() []string {
	var out []string
	for name, p := range map[string]string{"ffmpeg": t.FFmpeg, "ffprobe": t.FFprobe, "tts": t.TTS, "voice model": filepath.Join(t.ModelDir, "voices.bin"), "lexicon": t.LexiconPath(), "fonts": filepath.Join(t.FontsDir, "Inter-Bold.ttf")} {
		if _, err := os.Stat(p); err != nil {
			out = append(out, name)
		}
	}
	return out
}

// LexiconPath returns the American English lexicon of the installed model
// (v1.0 names it lexicon-us-en.txt; v0.19 used lexicon-en-us.txt).
func (t Tools) LexiconPath() string {
	v1 := filepath.Join(t.ModelDir, "lexicon-us-en.txt")
	if _, err := os.Stat(v1); err == nil {
		return v1
	}
	return filepath.Join(t.ModelDir, "lexicon-en-us.txt")
}

// FallbackLexicon returns the v0.19 lexicon used to supplement the v1.0
// one, or "" when the model has no v1.0 lexicon or no fallback is installed.
func (t Tools) FallbackLexicon() string {
	if filepath.Base(t.LexiconPath()) != "lexicon-us-en.txt" {
		return ""
	}
	p := filepath.Join(t.ModelDir, "lexicon-en-us.txt")
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

// LexiconArg is the --kokoro-lexicon value: the model lexicon plus the
// optional supplement.
func (t Tools) LexiconArg() string {
	if t.ExtraLexicon != "" {
		return t.LexiconPath() + "," + t.ExtraLexicon
	}
	return t.LexiconPath()
}

// DefaultVoice is Heart, the highest-rated Kokoro v1.0 English voice.
const DefaultVoice = 3

// Voices are the English speakers of Kokoro v1.0 offered in the UI
// (speaker ID in the model → name), best-rated first.
var Voices = []struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Lang string `json:"lang"`
}{
	{3, "Heart (American female)", "en"}, {2, "Bella (American female)", "en"}, {6, "Nicole (American female, soft)", "en"},
	{9, "Sarah (American female)", "en"}, {7, "Nova (American female)", "en"}, {16, "Michael (American male)", "en"},
	{14, "Fenrir (American male)", "en"}, {18, "Puck (American male)", "en"}, {11, "Adam (American male)", "en"},
	{21, "Emma (British female)", "en"}, {22, "Isabella (British female)", "en"}, {26, "George (British male)", "en"},
	{25, "Fable (British male)", "en"}, {24, "Daniel (British male)", "en"},
	{42, "Dora (Brazilian Portuguese female)", "pt-br"}, {43, "Alex (Brazilian Portuguese male)", "pt-br"},
	{44, "Santa (Brazilian Portuguese male)", "pt-br"},
}

// ValidVoice reports whether id is an offered speaker.
func ValidVoice(id int) bool {
	for _, v := range Voices {
		if v.ID == id {
			return true
		}
	}
	return false
}

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
	for _, name := range []string{"model.onnx", "model.int8.onnx"} {
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

// TTSArgs builds the sherpa-onnx command line for one utterance. pace
// above 1 speaks faster (0 means normal).
func TTSArgs(tools Tools, model string, voice int, pace float64, text, out string) []string {
	args := []string{
		"--kokoro-model=" + filepath.Join(tools.ModelDir, model),
		"--kokoro-voices=" + filepath.Join(tools.ModelDir, "voices.bin"),
		"--kokoro-tokens=" + filepath.Join(tools.ModelDir, "tokens.txt"),
		"--kokoro-lexicon=" + tools.LexiconArg(),
		"--num-threads=4",
		"--sid=" + strconv.Itoa(voice),
		"--output-filename=" + out,
	}
	if pace > 0 && pace != 1 {
		args = append(args, "--kokoro-length-scale="+strconv.FormatFloat(1/pace, 'f', 3, 64))
	}
	return append(args, text)
}

// Narrator synthesizes speech with a content-addressed cache, so unchanged
// segments are never synthesized twice.
type Narrator struct {
	Tools    Tools
	Runner   Runner
	CacheDir string
	// Pace is the speaking rate (1.2 is 20% faster); 0 means normal.
	Pace float64
	// Lang is the espeak-ng language for non-English narration ("" is
	// English); such text goes through the Node bridge (renderer/tts.mjs).
	Lang string
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
	// The model directory is part of the key so a model upgrade never reuses
	// audio from the previous model.
	key := filepath.Base(n.Tools.ModelDir) + "/" + model
	if n.Pace > 0 && n.Pace != 1 {
		key += "@" + strconv.FormatFloat(n.Pace, 'f', 3, 64) // a new pace is new audio
	}
	if n.Lang != "" {
		key += "#" + n.Lang
	}
	out := filepath.Join(n.CacheDir, CacheKey(key, voice, text)+".wav")
	if st, err := os.Stat(out); err == nil && st.Size() > 44 {
		return out, nil
	}
	tmp := out + ".tmp.wav"
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if n.Lang != "" {
		err = n.synthesizeBridge(ctx, model, voice, text, tmp)
	} else {
		_, err = n.Runner.Run(ctx, n.CacheDir, n.Tools.TTS, TTSArgs(n.Tools, model, voice, n.Pace, text, tmp)...)
	}
	if err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, out); err != nil {
		return "", fmt.Errorf("store narration: %w", err)
	}
	return out, nil
}

// synthesizeBridge narrates non-English text with the Node bridge. The
// request goes in a UTF-8 JSON file: Windows would pass accented text on
// the command line in the ANSI code page and garble it.
func (n Narrator) synthesizeBridge(ctx context.Context, model string, voice int, text, out string) error {
	if n.Tools.Node == "" || n.Tools.TTSBridge == "" {
		return fmt.Errorf("%w: non-English narration needs Node and renderer/tts.mjs (npm install in renderer/)", ErrToolsMissing)
	}
	scale := 1.0
	if n.Pace > 0 {
		scale = 1 / n.Pace
	}
	req, err := json.Marshal(map[string]any{
		"model": filepath.Join(n.Tools.ModelDir, model), "voices": filepath.Join(n.Tools.ModelDir, "voices.bin"),
		"tokens": filepath.Join(n.Tools.ModelDir, "tokens.txt"), "lexicon": n.Tools.LexiconPath(),
		"lang": n.Lang, "sid": voice, "lengthScale": scale, "threads": 4, "text": text, "out": out,
	})
	if err != nil {
		return err
	}
	reqFile := out + ".json"
	if err := os.WriteFile(reqFile, req, 0o644); err != nil {
		return fmt.Errorf("write tts request: %w", err)
	}
	defer os.Remove(reqFile)
	_, err = n.Runner.Run(ctx, n.CacheDir, n.Tools.Node, n.Tools.TTSBridge, reqFile)
	return err
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
