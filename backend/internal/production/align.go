package production

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// Word timing: the narration is transcribed back with a small local speech
// recognizer (NVIDIA Parakeet through sherpa-onnx) and the heard words are
// matched to the script, so captions follow the voice instead of an
// estimate. Words the recognizer hears differently (numbers spoken as
// words, for example) get times interpolated between matched neighbors.

// AlignerModelDir is the Parakeet model directory under .local/models.
const AlignerModelDir = "sherpa-onnx-nemo-parakeet_tdt_transducer_110m-en-36000-int8"

// heardWord is one recognized word and when it starts (seconds).
type heardWord struct {
	Text  string
	Start float64
}

// The recognizer prints one JSON-like result line, but some of its fields
// (log probabilities) are not always valid JSON, so only the two arrays
// needed are extracted and decoded.
var (
	tokensRe     = regexp.MustCompile(`"tokens":\s*(\[(?:\s*"(?:[^"\\]|\\.)*"\s*,?)*\s*\])`)
	timestampsRe = regexp.MustCompile(`"timestamps":\s*(\[[-0-9.eE,\s]*\])`)
)

// parseRecognition reads the result line printed by sherpa-onnx-offline
// and joins sub-word tokens into words.
func parseRecognition(out []byte) ([]heardWord, error) {
	var tokens []string
	var stamps []float64
	found := false
	for _, line := range bytes.Split(out, []byte("\n")) {
		tm, sm := tokensRe.FindSubmatch(line), timestampsRe.FindSubmatch(line)
		if tm == nil || sm == nil {
			continue
		}
		tokens, stamps = nil, nil
		if err := json.Unmarshal(tm[1], &tokens); err != nil {
			return nil, fmt.Errorf("parse recognizer tokens: %w", err)
		}
		if err := json.Unmarshal(sm[1], &stamps); err != nil {
			return nil, fmt.Errorf("parse recognizer timestamps: %w", err)
		}
		found = true
	}
	if !found {
		return nil, errors.New("recognizer printed no result")
	}
	if len(tokens) != len(stamps) {
		return nil, errors.New("recognizer result has mismatched tokens and timestamps")
	}
	var words []heardWord
	for i, tok := range tokens {
		if strings.HasPrefix(tok, " ") || strings.HasPrefix(tok, "▁") || len(words) == 0 {
			words = append(words, heardWord{Text: strings.TrimLeft(tok, " ▁"), Start: stamps[i]})
		} else {
			words[len(words)-1].Text += tok
		}
	}
	return words, nil
}

// normWord lowercases and keeps only letters and digits.
func normWord(w string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(w) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// alignWords returns a start time for every script word: matched words take
// the heard time (longest common subsequence), the rest are interpolated
// between neighbors. Times never decrease and stay within [0, duration).
func alignWords(script []string, heard []heardWord, duration float64) []float64 {
	n, m := len(script), len(heard)
	a := make([]string, n)
	for i, w := range script {
		a[i] = normWord(w)
	}
	b := make([]string, m)
	for j, w := range heard {
		b[j] = normWord(w.Text)
	}
	// LCS table from the end so the walk below goes forward.
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] != "" && a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else {
				dp[i][j] = max(dp[i+1][j], dp[i][j+1])
			}
		}
	}
	times := make([]float64, n)
	known := make([]bool, n)
	for i, j := 0, 0; i < n && j < m; {
		switch {
		case a[i] != "" && a[i] == b[j]:
			times[i], known[i] = heard[j].Start, true
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			i++
		default:
			j++
		}
	}
	for i := 0; i < n; i++ {
		if known[i] {
			continue
		}
		lo, hi := -1, -1
		for k := i - 1; k >= 0; k-- {
			if known[k] {
				lo = k
				break
			}
		}
		for k := i + 1; k < n; k++ {
			if known[k] {
				hi = k
				break
			}
		}
		switch {
		case lo >= 0 && hi >= 0:
			times[i] = times[lo] + (times[hi]-times[lo])*float64(i-lo)/float64(hi-lo)
		case lo >= 0:
			// Spread trailing words over the remaining audio.
			times[i] = times[lo] + (duration-times[lo])*float64(i-lo)/float64(n-lo)
		case hi >= 0:
			times[i] = times[hi] * float64(i) / float64(hi)
		default:
			times[i] = duration * float64(i) / float64(max(n, 1))
		}
	}
	for i := range times {
		times[i] = min(max(times[i], 0), duration)
		if i > 0 && times[i] < times[i-1] {
			times[i] = times[i-1]
		}
	}
	return times
}

// Aligner times narration words with the local recognizer.
type Aligner struct {
	Exe      string // sherpa-onnx-offline executable
	ModelDir string
	Runner   Runner
}

// Available reports whether the recognizer and its model are installed.
func (a Aligner) Available() bool {
	for _, p := range []string{a.Exe, filepath.Join(a.ModelDir, "encoder.int8.onnx"), filepath.Join(a.ModelDir, "tokens.txt")} {
		if _, err := os.Stat(p); err != nil {
			return false
		}
	}
	return true
}

// WordTimes returns the start time of each word of text (split on spaces)
// in the narration file wav. Results are cached next to the cached WAV,
// whose name is already a content hash.
func (a Aligner) WordTimes(ctx context.Context, wav, text string, duration float64) ([]float64, error) {
	words := strings.Fields(text)
	cache := strings.TrimSuffix(wav, filepath.Ext(wav)) + ".words.json"
	var heard []heardWord
	if b, err := os.ReadFile(cache); err == nil && json.Unmarshal(b, &heard) == nil {
		return alignWords(words, heard, duration), nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	m := a.ModelDir
	out, err := a.Runner.Run(ctx, filepath.Dir(wav), a.Exe,
		"--tokens="+filepath.Join(m, "tokens.txt"), "--encoder="+filepath.Join(m, "encoder.int8.onnx"),
		"--decoder="+filepath.Join(m, "decoder.int8.onnx"), "--joiner="+filepath.Join(m, "joiner.int8.onnx"),
		"--model-type=nemo_transducer", "--num-threads=2", wav)
	if err != nil {
		return nil, err
	}
	heard, err = parseRecognition(out)
	if err != nil {
		return nil, err
	}
	if b, err := json.Marshal(heard); err == nil {
		_ = os.WriteFile(cache, b, 0o644) // best effort; alignment is cheap to redo
	}
	return alignWords(words, heard, duration), nil
}
