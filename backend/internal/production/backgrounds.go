package production

import (
	"context"
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
		if !e.IsDir() && backgroundExts[strings.ToLower(filepath.Ext(e.Name()))] {
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
