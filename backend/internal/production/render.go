package production

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"
)

// Timed is one narrated segment with its audio and measured duration.
type Timed struct {
	Audio    string
	Duration float64
	Title    string
	Text     string
	// Words holds the start time of each word of Text (split on spaces),
	// relative to the segment, when it was measured from the audio.
	Words []float64
	// Speaker is "B" for the second voice of a dialogue.
	Speaker string
}

// wordStarts returns the start of each word of s.Text within the segment:
// the measured times when present, else an estimate proportional to word
// length.
func wordStarts(s Timed) []float64 {
	words := strings.Fields(s.Text)
	if len(s.Words) == len(words) {
		return s.Words
	}
	total := 0
	for _, w := range words {
		total += len([]rune(w)) + 1
	}
	out := make([]float64, len(words))
	t := 0.0
	for i, w := range words {
		out[i] = t
		t += s.Duration * float64(len([]rune(w))+1) / float64(max(total, 1))
	}
	return out
}

// Style holds brand values used on screen.
type Style struct {
	Background string // #RRGGBB
	Text       string // #RRGGBB
	Accent     string // #RRGGBB
	Font       string // font family name
}

// CaptionChunks splits narration into short caption lines of at most
// maxWords words, preferring to break after punctuation.
func CaptionChunks(text string, maxWords int) []string {
	words := strings.Fields(text)
	var out []string
	var cur []string
	for _, w := range words {
		cur = append(cur, w)
		end := strings.ContainsAny(w[len(w)-1:], ".!?;:,")
		if len(cur) >= maxWords || (end && len(cur) >= maxWords/2) {
			out = append(out, strings.Join(cur, " "))
			cur = nil
		}
	}
	if len(cur) > 0 {
		out = append(out, strings.Join(cur, " "))
	}
	return out
}

// Cue is a caption shown between Start and End seconds.
type Cue struct {
	Start, End float64
	Text       string
	Speaker    string
}

// Cues times each segment's caption chunks: a chunk starts with its first
// word and lasts until the next chunk (or the segment end).
func Cues(segs []Timed, maxWords int) []Cue {
	var cues []Cue
	offset := 0.0
	for _, s := range segs {
		chunks := CaptionChunks(s.Text, maxWords)
		starts := wordStarts(s)
		idx := 0
		first := len(cues)
		for _, c := range chunks {
			start := 0.0
			if idx < len(starts) {
				start = starts[idx]
			}
			if len(cues) == first {
				start = 0 // the first chunk covers any leading silence
			}
			cues = append(cues, Cue{Start: offset + start, Text: c, Speaker: s.Speaker})
			idx += len(strings.Fields(c))
		}
		for i := first; i < len(cues); i++ {
			if i+1 < len(cues) {
				cues[i].End = cues[i+1].Start
			} else {
				cues[i].End = offset + s.Duration
			}
		}
		offset += s.Duration
	}
	return cues
}

func assTime(sec float64) string {
	cs := int(math.Round(sec * 100))
	return fmt.Sprintf("%d:%02d:%02d.%02d", cs/360000, cs/6000%60, cs/100%60, cs%100)
}

func srtTime(sec float64) string {
	ms := int(math.Round(sec * 1000))
	return fmt.Sprintf("%02d:%02d:%02d,%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
}

// assColor converts #RRGGBB to ASS &H00BBGGRR.
func assColor(hex string) string {
	h := strings.TrimPrefix(hex, "#")
	if len(h) != 6 {
		return "&H00FFFFFF"
	}
	return "&H00" + strings.ToUpper(h[4:6]+h[2:4]+h[0:2])
}

// assEscape neutralizes ASS override syntax in user or model text.
func assEscape(s string) string {
	s = strings.NewReplacer("\\", "/", "{", "(", "}", ")", "\r", " ", "\n", " ").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

// BuildASS returns styled subtitles: a title card per segment and
// bottom captions.
func BuildASS(segs []Timed, width, height int, st Style) string {
	font := assEscape(st.Font)
	if font == "" {
		font = "Inter"
	}
	titleSize := height / 12
	captionSize := height / 20
	if width < height {
		titleSize, captionSize = width/11, width/16
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[Script Info]\nScriptType: v4.00+\nPlayResX: %d\nPlayResY: %d\nWrapStyle: 0\nScaledBorderAndShadow: yes\n\n", width, height)
	b.WriteString("[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n")
	fmt.Fprintf(&b, "Style: Title,%s,%d,%s,%s,%s,&H00000000,-1,0,0,0,100,100,0,0,1,0,0,5,%d,%d,0,1\n",
		font, titleSize, assColor(st.Text), assColor(st.Accent), assColor(st.Background), width/12, width/12)
	fmt.Fprintf(&b, "Style: Caption,%s,%d,&H00FFFFFF,&H00FFFFFF,&H00000000,&H80000000,-1,0,0,0,100,100,0,0,3,%d,0,2,%d,%d,%d,1\n\n",
		font, captionSize, max(captionSize/5, 4), width/14, width/14, height/12)
	b.WriteString("[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n")
	offset := 0.0
	for _, s := range segs {
		if title := assEscape(s.Title); title != "" {
			fmt.Fprintf(&b, "Dialogue: 0,%s,%s,Title,,0,0,0,,{\\fad(300,200)}%s\n", assTime(offset), assTime(offset+s.Duration), title)
		}
		offset += s.Duration
	}
	for _, c := range Cues(segs, 8) {
		fmt.Fprintf(&b, "Dialogue: 1,%s,%s,Caption,,0,0,0,,%s\n", assTime(c.Start), assTime(c.End), assEscape(c.Text))
	}
	return b.String()
}

// BuildWordASS shows captions one word (or short pair) at a time in the
// middle of the frame, popping in, with key words highlighted.
func BuildWordASS(chunks []CaptionChunk, width, height int) string {
	unit := float64(min(width, height)) / 100
	var b strings.Builder
	fmt.Fprintf(&b, "[Script Info]\nScriptType: v4.00+\nPlayResX: %d\nPlayResY: %d\nWrapStyle: 0\nScaledBorderAndShadow: yes\n\n", width, height)
	b.WriteString("[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n")
	fmt.Fprintf(&b, "Style: Word,Inter,%d,&H00FFFFFF,&H00FFFFFF,&H00000000,&H99000000,-1,0,0,0,100,100,0,0,1,%d,%d,5,%d,%d,0,1\n\n",
		int(unit*12.5), max(int(unit*1.3), 3), max(int(unit*0.6), 1), int(unit*5), int(unit*5))
	b.WriteString("[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n")
	for _, c := range chunks {
		words := make([]string, 0, len(c.Words))
		for _, w := range c.Words {
			words = append(words, w.Text)
		}
		text := assEscape(strings.Join(words, " "))
		if text == "" {
			continue
		}
		color := ""
		switch {
		case c.Hot:
			color = `\c&H004DE1FF&` // #FFE14D in ASS BGR order
		case c.Speaker == "B":
			color = `\c&H00FFDB7F&` // #7FDBFF: the second voice
		}
		size := int(unit * captionFontUnits(c))
		fmt.Fprintf(&b, "Dialogue: 0,%s,%s,Word,,0,0,0,,{\\pos(%d,%d)\\fs%d%s\\fscx55\\fscy55\\t(0,140,\\fscx100\\fscy100)}%s\n",
			assTime(c.Start), assTime(c.End), width/2, height*48/100, size, color, text)
	}
	return b.String()
}

// BuildSRT returns plain captions for upload to platforms.
func BuildSRT(segs []Timed) string {
	var b strings.Builder
	for i, c := range Cues(segs, 10) {
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", i+1, srtTime(c.Start), srtTime(c.End), strings.Join(strings.Fields(c.Text), " "))
	}
	return b.String()
}

// filterPath makes a relative path safe inside an FFmpeg filter argument:
// forward slashes, and characters with filter meaning escaped.
func filterPath(p string) string {
	p = filepath.ToSlash(p)
	return strings.NewReplacer(`\`, `\\`, `'`, `\'`, `:`, `\:`, `,`, `\,`, `[`, `\[`, `]`, `\]`).Replace(p)
}

// FFmpegArgs combines the visual track, concatenated narration, and
// burned-in subtitles. visual is a rendered scene video; when empty, a solid
// brand background is generated instead. Paths in the filter graph are
// relative to the working directory (the video folder) so no drive letters
// appear there.
func FFmpegArgs(visual string, mix AudioMix, total float64, width, height int, background, assFile, fontsDir, out string) []string {
	bg := strings.TrimPrefix(background, "#")
	if len(bg) != 6 {
		bg = "111827"
	}
	args := []string{"-y", "-hide_banner", "-loglevel", "error"}
	if visual != "" {
		args = append(args, "-i", visual)
	} else {
		args = append(args, "-f", "lavfi", "-i", fmt.Sprintf("color=c=0x%s:s=%dx%d:r=30:d=%s", bg, width, height, strconv.FormatFloat(total+0.5, 'f', 2, 64)))
	}
	for _, a := range mix.Voice {
		args = append(args, "-i", a)
	}
	next := len(mix.Voice) + 1
	musicIn, sfxIn := -1, -1
	if mix.Music != "" {
		args = append(args, "-i", mix.Music)
		musicIn, next = next, next+1
	}
	if mix.Effects != "" {
		args = append(args, "-i", mix.Effects)
		sfxIn = next
	}
	var fc strings.Builder
	for i := range mix.Voice {
		fmt.Fprintf(&fc, "[%d:a]", i+1)
	}
	// Voice: concatenate, upsample, remove rumble, and even out dynamics.
	fmt.Fprintf(&fc, "concat=n=%d:v=0:a=1,aresample=%d,aformat=channel_layouts=stereo,highpass=f=70,"+
		"acompressor=threshold=0.125:ratio=3:attack=5:release=120:makeup=2", len(mix.Voice), AudioRate)
	layers := 1
	if musicIn >= 0 {
		// The voice keys a compressor on the music so the bed ducks under speech.
		fmt.Fprintf(&fc, ",asplit=2[voice][key];[%d:a]volume=%s[bed];[bed][key]sidechaincompress=threshold=0.02:ratio=8:attack=20:release=500[music];[voice][music]",
			musicIn, strconv.FormatFloat(mix.musicGain(), 'f', 2, 64))
		layers++
	} else {
		fc.WriteString("[voice];[voice]")
	}
	if sfxIn >= 0 {
		fmt.Fprintf(&fc, "[%d:a]", sfxIn)
		layers++
	}
	// duration=first keeps the narration length; loudnorm targets the
	// -14 LUFS level YouTube normalizes to.
	fmt.Fprintf(&fc, "amix=inputs=%d:duration=first:normalize=0,loudnorm=I=-14:TP=-1.5:LRA=11,aresample=%d[a];", layers, AudioRate)
	if assFile != "" {
		fmt.Fprintf(&fc, "[0:v]subtitles=filename='%s':fontsdir='%s'[v]", filterPath(assFile), filterPath(fontsDir))
	} else {
		// Captions are already part of the animated visual.
		fc.WriteString("[0:v]null[v]")
	}
	args = append(args, "-filter_complex", fc.String(), "-map", "[v]", "-map", "[a]",
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "192k", "-ar", strconv.Itoa(AudioRate), "-movflags", "+faststart", "-shortest", out)
	return args
}

// AudioMix lists the audio inputs of a render: narration segments in order,
// plus optional music bed and effects tracks (WAV paths).
type AudioMix struct {
	Voice   []string
	Music   string
	Effects string
	// MusicGain scales the music bed before ducking (default 0.30).
	MusicGain float64
}

func (m AudioMix) musicGain() float64 {
	if m.MusicGain <= 0 || m.MusicGain > 1 {
		return 0.30
	}
	return m.MusicGain
}
