package production

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// BackgroundsDir, inside the media directory, holds the owner's own
// background clips (for example gameplay they recorded). The gameplay
// story plays one of them behind the captions instead of the drawn game.
// Only clips the owner has the rights to belong here.
const BackgroundsDir = "backgrounds"

var backgroundExts = map[string]bool{".mp4": true, ".mov": true, ".mkv": true, ".webm": true, ".m4v": true}

// listBackgrounds returns the background clips, sorted for stable picks.
func listBackgrounds(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		// Skip clips still being written (see CutBackgrounds).
		if !e.IsDir() && backgroundExts[strings.ToLower(filepath.Ext(e.Name()))] && !strings.Contains(e.Name(), ".tmp.") {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out
}

// pickBackground chooses a clip and a start offset from a seed, so each
// video gets its own stretch and re-renders stay identical. Clips shorter
// than the video start at 0 and loop.
func pickBackground(clips []string, durations []float64, seed uint32, total float64) (string, float64) {
	if len(clips) == 0 {
		return "", 0
	}
	i := int(seed % uint32(len(clips)))
	spare := durations[i] - total - 1
	if spare <= 0 {
		return clips[i], 0
	}
	// Skip the first second (intros and fades), then a seeded offset.
	return clips[i], 1 + float64((seed/7)%1000)/1000*(spare-1)
}

// BackgroundArgs cuts, loops if needed, mutes, and crops a clip to fill
// width x height at 30 fps. A short keyframe interval keeps seeking cheap
// for the frame-by-frame renderer.
func BackgroundArgs(src string, offset, length float64, width, height int, out string) []string {
	sec := func(f float64) string { return strconv.FormatFloat(f, 'f', 3, 64) }
	vf := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,setsar=1,fps=30,format=yuv420p", width, height, width, height)
	return []string{"-y", "-hide_banner", "-loglevel", "error", "-stream_loop", "-1", "-ss", sec(offset), "-i", src,
		"-t", sec(length), "-an", "-vf", vf, "-c:v", "libx264", "-preset", "veryfast", "-crf", "21", "-g", "15", "-pix_fmt", "yuv420p", out}
}

// prepareBackground picks one of the owner's clips and writes it, cropped
// and trimmed to the video, into dir. It returns "" when there are no
// clips, so the drawn game is used.
func (s *Service) prepareBackground(ctx context.Context, dir string, seed uint32, total float64, width, height int) (string, string, error) {
	clips := listBackgrounds(filepath.Join(s.mediaDir, BackgroundsDir))
	if len(clips) == 0 {
		return "", "", nil
	}
	durations := make([]float64, len(clips))
	for i, c := range clips {
		d, err := ProbeDuration(ctx, s.runner, s.tools.FFprobe, c)
		if err != nil {
			return "", "", fmt.Errorf("read background %s: %w", filepath.Base(c), err)
		}
		durations[i] = d
	}
	src, offset := pickBackground(clips, durations, seed, total)
	out := filepath.Join(dir, "background.mp4")
	if _, err := s.runner.Run(ctx, dir, s.tools.FFmpeg, BackgroundArgs(src, offset, total+0.5, width, height, out)...); err != nil {
		return "", "", fmt.Errorf("prepare background: %w", err)
	}
	return out, filepath.Base(src), nil
}

// SourcesDir, inside BackgroundsDir, holds long source videos that
// CutBackgrounds splits into ready-to-use vertical clips.
const SourcesDir = "sources"

// cutStarts spreads up to count non-overlapping clips of length seconds
// evenly across a source, skipping its first and last two seconds
// (intros, fades, outros).
func cutStarts(duration, length float64, count int) []float64 {
	usable := duration - 4
	n := min(count, int(usable/length))
	if n <= 0 {
		return nil
	}
	if n == 1 {
		return []float64{2}
	}
	step := (usable - length) / float64(n-1)
	out := make([]float64, n)
	for i := range out {
		out[i] = 2 + float64(i)*step
	}
	return out
}

// cutName names clip i (from 1) of a source: a short safe form of its
// name plus a fingerprint of the full name, so similar names never clash.
func cutName(src string, i int) string {
	base := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	var b strings.Builder
	for _, r := range strings.ToLower(base) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteByte('-')
		}
	}
	name := strings.Trim(b.String(), "-")
	if len(name) > 40 {
		name = strings.Trim(name[:40], "-")
	}
	if name == "" {
		name = "clip"
	}
	sum := sha256.Sum256([]byte(filepath.Base(src)))
	return fmt.Sprintf("%s-%s-%02d.mp4", name, hex.EncodeToString(sum[:3]), i)
}

// CutBackgrounds splits every source video into up to count vertical
// clips of length seconds (cropped to width x height and muted) in the
// backgrounds folder. Existing clips are kept, so it can be re-run after
// adding sources. It returns the paths of the clips it made.
func CutBackgrounds(ctx context.Context, tools Tools, runner Runner, mediaDir string, count int, length float64, width, height int) ([]string, error) {
	// Tools run in other working directories, so paths must be absolute.
	if abs, err := filepath.Abs(mediaDir); err == nil {
		mediaDir = abs
	}
	outDir := filepath.Join(mediaDir, BackgroundsDir)
	var made []string
	for _, src := range listBackgrounds(filepath.Join(outDir, SourcesDir)) {
		d, err := ProbeDuration(ctx, runner, tools.FFprobe, src)
		if err != nil {
			return made, fmt.Errorf("read %s: %w", filepath.Base(src), err)
		}
		for i, start := range cutStarts(d, length, count) {
			out := filepath.Join(outDir, cutName(src, i+1))
			if st, err := os.Stat(out); err == nil && st.Size() > 0 {
				continue
			}
			tmp := strings.TrimSuffix(out, ".mp4") + ".tmp.mp4"
			if _, err := runner.Run(ctx, outDir, tools.FFmpeg, BackgroundArgs(src, start, length, width, height, tmp)...); err != nil {
				_ = os.Remove(tmp)
				return made, fmt.Errorf("cut %s: %w", filepath.Base(out), err)
			}
			if err := os.Rename(tmp, out); err != nil {
				return made, fmt.Errorf("store %s: %w", filepath.Base(out), err)
			}
			made = append(made, out)
		}
	}
	return made, nil
}
