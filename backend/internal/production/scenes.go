package production

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Visual styles. "auto" picks one from the channel niche.
const (
	StyleAuto        = "auto"
	StyleMinimal     = "minimal"
	StyleDocumentary = "documentary"
	StyleBold        = "bold"
	StyleTech        = "tech"
)

// Styles lists selectable styles with descriptions for the UI.
var Styles = []struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}{
	{StyleAuto, "Automatic", "Chosen from the channel niche."},
	{StyleMinimal, "Minimal", "Clean Swiss typography with a soft drifting accent. Good for general explainers."},
	{StyleDocumentary, "Documentary", "Serif titles, film grain, and slow camera drift. Good for history and true stories."},
	{StyleBold, "Bold", "Huge type, strong color bands, and big counters. Good for finance and business."},
	{StyleTech, "Tech", "Monospace details over a moving grid. Good for science, space, and AI."},
}

// ValidStyle reports whether s is a known style.
func ValidStyle(s string) bool {
	for _, st := range Styles {
		if st.ID == s {
			return true
		}
	}
	return false
}

var styleKeywords = []struct {
	style string
	words []string
}{
	{StyleTech, []string{"tech", "ai", "artificial", "science", "space", "astronomy", "physics", "engineering", "software", "computer", "quantum", "robot"}},
	{StyleDocumentary, []string{"history", "crime", "mystery", "war", "ancient", "biography", "documentary", "stories", "story", "archaeology"}},
	{StyleBold, []string{"finance", "money", "business", "invest", "economy", "market", "crypto", "career", "startup"}},
}

// ResolveStyle turns "auto" into a concrete style using the niche.
func ResolveStyle(style, niche string) string {
	if style != "" && style != StyleAuto && ValidStyle(style) {
		return style
	}
	words := strings.FieldsFunc(strings.ToLower(niche), func(r rune) bool { return !(r >= 'a' && r <= 'z') })
	for _, sk := range styleKeywords {
		for _, w := range words {
			for _, k := range sk.words {
				if w == k {
					return sk.style
				}
			}
		}
	}
	return StyleMinimal
}

// Scene kinds.
const (
	SceneTitle     = "title"
	SceneStatement = "statement"
	SceneNumber    = "number"
	SceneOutro     = "outro"
)

// Scene is one timed visual block.
type Scene struct {
	Kind     string
	Start    float64
	Duration float64
	Kicker   string // small label above the headline
	Headline string
	Words    []string // headline split for staggered reveals
	// Number scenes animate Value with Decimals, framed by Prefix/Suffix.
	Value    float64
	Decimals int
	Prefix   string
	Suffix   string
	SizeTier int // 1 (long text) .. 3 (short text)
	// Emph is the index of the headline word to highlight, or -1.
	Emph int
}

// emphasisIndex picks the word to highlight: one with a digit, else the
// longest word of at least five letters, else none.
func emphasisIndex(words []string) int {
	best, bestLen := -1, 4
	for i, w := range words {
		clean := strings.Trim(w, ".,!?;:\"'()…")
		if strings.ContainsAny(clean, "0123456789") {
			return i
		}
		if n := len([]rune(clean)); n > bestLen {
			best, bestLen = i, n
		}
	}
	return best
}

var leadingNumberRe = regexp.MustCompile(`^(.*?)(-?\d[\d,]*(?:\.\d+)?)(.*)$`)

// parseNumber extracts the first number of a short on-screen line, e.g.
// "3.8 cm per year" -> 3.8, 1 decimal, suffix " cm per year".
func parseNumber(s string) (value float64, decimals int, prefix, suffix string, ok bool) {
	m := leadingNumberRe.FindStringSubmatch(s)
	if m == nil || len(s) > 60 {
		return 0, 0, "", "", false
	}
	raw := strings.ReplaceAll(m[2], ",", "")
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsInf(v, 0) {
		return 0, 0, "", "", false
	}
	if _, frac, has := strings.Cut(raw, "."); has {
		decimals = len(frac)
	}
	return v, decimals, m[1], m[3], true
}

func sizeTier(s string) int {
	switch n := len([]rune(s)); {
	case n <= 18:
		return 3
	case n <= 40:
		return 2
	default:
		return 1
	}
}

// PlanScenes maps narrated segments to scenes. The first segment (hook)
// becomes a title, the last (outro) an outro card, numbers become counters.
func PlanScenes(segs []Timed, kickers []string, channelName string) []Scene {
	scenes := make([]Scene, 0, len(segs))
	t := 0.0
	for i, s := range segs {
		sc := Scene{Start: t, Duration: s.Duration}
		headline := strings.TrimSpace(s.Title)
		switch {
		case i == 0 && headline == "":
			sc.Kind = SceneTitle
			headline = firstSentence(s.Text, 70)
		case i == len(segs)-1 && headline == "" && len(segs) > 1:
			sc.Kind = SceneOutro
			headline = channelName
			sc.Kicker = "Thanks for watching"
		default:
			sc.Kind = SceneStatement
			if v, d, p, suf, ok := parseNumber(headline); ok {
				sc.Kind, sc.Value, sc.Decimals, sc.Prefix, sc.Suffix = SceneNumber, v, d, p, suf
			}
			if i < len(kickers) {
				sc.Kicker = kickers[i]
			}
		}
		if headline == "" {
			headline = firstSentence(s.Text, 60)
		}
		sc.Headline = headline
		sc.Words = strings.Fields(headline)
		sc.Emph = -1
		if sc.Kind == SceneStatement || sc.Kind == SceneTitle {
			sc.Emph = emphasisIndex(sc.Words)
		}
		sc.SizeTier = sizeTier(headline)
		if sc.Kind == SceneTitle && sc.SizeTier < 2 {
			sc.SizeTier = 2 // the hook gets large type even when long
		}
		scenes = append(scenes, sc)
		t += s.Duration
	}
	return scenes
}

func firstSentence(s string, max int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, ".!?"); i > 0 {
		s = s[:i+1]
	}
	if r := []rune(s); len(r) > max {
		s = strings.TrimSpace(string(r[:max])) + "…"
	}
	return s
}

// CaptionWord is one caption word and the time it is spoken.
type CaptionWord struct {
	Text string
	At   float64
}

// CaptionChunk is a short caption line shown from Start to End.
type CaptionChunk struct {
	Start, End float64
	Words      []CaptionWord
}

// CaptionTrack splits narration into short caption lines with an estimated
// time per word (proportional to word length within each line).
func CaptionTrack(segs []Timed, maxWords int) []CaptionChunk {
	var out []CaptionChunk
	for _, c := range Cues(segs, maxWords) {
		words := strings.Fields(c.Text)
		total := 0
		for _, w := range words {
			total += len([]rune(w)) + 1
		}
		ch := CaptionChunk{Start: c.Start, End: c.End}
		t := c.Start
		for _, w := range words {
			ch.Words = append(ch.Words, CaptionWord{Text: w, At: t})
			t += (c.End - c.Start) * float64(len([]rune(w))+1) / float64(max(total, 1))
		}
		out = append(out, ch)
	}
	return out
}

// ProjectInput is everything needed to write a HyperFrames project.
type ProjectInput struct {
	Width, Height int
	Style         string
	Palette       Style // background/text/accent colors
	Secondary     string
	ChannelName   string
	Scenes        []Scene
	// Captions are animated word by word inside the scene layer.
	Captions []CaptionChunk
	// Seed makes decorative randomness (particles) repeatable.
	Seed uint32
}

type projectView struct {
	ProjectInput
	Total float64
	Unit  float64 // 1% of the shorter side, in px
	// Safe area keeps scene text above the burned-in captions.
	SafeBottom    int
	CaptionBottom int
	Data          []sceneData
	CaptionData   []captionData
}

type captionData struct {
	ID    string    `json:"id"`
	Start float64   `json:"start"`
	End   float64   `json:"end"`
	Words []float64 `json:"words"`
}

// sceneData is passed to the animation script as JSON (html/template
// escapes it for the script context).
type sceneData struct {
	Kind     string  `json:"kind"`
	ID       string  `json:"id"`
	Start    float64 `json:"start"`
	Duration float64 `json:"duration"`
	Value    float64 `json:"value"`
	Decimals int     `json:"decimals"`
}

// WriteProject writes index.html plus the vendored GSAP and fonts into dir.
// All user and model text goes through html/template escaping.
func WriteProject(dir string, in ProjectInput, gsapPath, fontsPkgDir string) error {
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		return fmt.Errorf("create project: %w", err)
	}
	if err := copyFile(gsapPath, filepath.Join(dir, "assets", "gsap.min.js")); err != nil {
		return fmt.Errorf("copy gsap: %w", err)
	}
	for _, f := range projectFonts {
		src := filepath.Join(fontsPkgDir, f.pkg, "files", f.file)
		if err := copyFile(src, filepath.Join(dir, "assets", f.file)); err != nil {
			return fmt.Errorf("copy font %s: %w", f.file, err)
		}
	}
	v := projectView{ProjectInput: in, Unit: float64(min(in.Width, in.Height)) / 100, SafeBottom: in.Height * 24 / 100, CaptionBottom: in.Height * 7 / 100}
	for i, s := range in.Scenes {
		v.Total = s.Start + s.Duration
		v.Data = append(v.Data, sceneData{Kind: s.Kind, ID: fmt.Sprintf("s%d", i), Start: s.Start, Duration: s.Duration, Value: s.Value, Decimals: s.Decimals})
	}
	for i, c := range in.Captions {
		d := captionData{ID: fmt.Sprintf("c%d", i), Start: c.Start, End: c.End, Words: []float64{}}
		for _, w := range c.Words {
			d.Words = append(d.Words, w.At)
		}
		v.CaptionData = append(v.CaptionData, d)
	}
	var b bytes.Buffer
	if err := projectTemplate.Execute(&b, v); err != nil {
		return fmt.Errorf("render project template: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, "index.html"), b.Bytes(), 0o644)
}

var projectFonts = []struct{ pkg, file string }{
	{"inter", "inter-latin-400-normal.woff2"}, {"inter", "inter-latin-600-normal.woff2"}, {"inter", "inter-latin-800-normal.woff2"},
	{"fraunces", "fraunces-latin-400-normal.woff2"}, {"fraunces", "fraunces-latin-700-normal.woff2"}, {"fraunces", "fraunces-latin-900-normal.woff2"},
	{"space-grotesk", "space-grotesk-latin-500-normal.woff2"}, {"space-grotesk", "space-grotesk-latin-700-normal.woff2"},
	{"jetbrains-mono", "jetbrains-mono-latin-400-normal.woff2"}, {"jetbrains-mono", "jetbrains-mono-latin-700-normal.woff2"},
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

var projectTemplate = template.Must(template.New("index").Funcs(template.FuncMap{
	"unit":    func(u float64) template.CSS { return template.CSS(strconv.FormatFloat(u, 'f', 2, 64) + "px") },
	"bg":      func(v projectView) template.CSS { return cssColor(v.Palette.Background, "#0F172A") },
	"ink":     func(v projectView) template.CSS { return cssColor(v.Palette.Text, "#F8FAFC") },
	"accent":  func(v projectView) template.CSS { return cssColor(v.Palette.Accent, "#60A5FA") },
	"accent2": func(v projectView) template.CSS { return cssColor(v.Secondary, "#1E3A8A") },
	"sec":     func(f float64) string { return strconv.FormatFloat(f, 'f', 3, 64) },
	"sid":     func(i int) string { return fmt.Sprintf("s%d", i) },
	"cid":     func(i int) string { return fmt.Sprintf("c%d", i) },
	"wid":     func(i, j int) string { return fmt.Sprintf("c%dw%d", i, j) },
}).Parse(projectHTML))
