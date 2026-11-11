package production

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Kokoro v1.0 ships a lexicon without some common words (for example
// "centimeters") that the older v0.19 lexicon has. The two use different
// phoneme notations, so instead of passing both to the engine we convert
// only the missing words to v1.0 notation and pass them as a supplement.
// Because the supplement never repeats a primary word, the engine's
// precedence between lexicons does not matter.

// v0ToV1 rewrites v0.19 (espeak-style) phonemes into v1.0 (misaki) notation.
var v0ToV1 = strings.NewReplacer(
	"eɪ", "A", "aɪ", "I", "oʊ", "O", "aʊ", "W", "ɔɪ", "Y",
	"ɚ", "əɹ", "ɝ", "ɜɹ", "dʒ", "ʤ", "tʃ", "ʧ", "ː", "",
)

// convertV0Phones turns "t ə mˈ e ɪ ɾ o ʊ" into "t ə m ˈ A ɾ O".
func convertV0Phones(p string) string {
	joined := v0ToV1.Replace(strings.Join(strings.Fields(p), ""))
	runes := []rune(joined)
	parts := make([]string, len(runes))
	for i, r := range runes {
		parts[i] = string(r)
	}
	return strings.Join(parts, " ")
}

// loadTokens reads the model's token table ("<token> <id>" per line).
func loadTokens(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open tokens: %w", err)
	}
	defer f.Close()
	tokens := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.LastIndexByte(line, ' '); i > 0 {
			tokens[line[:i]] = true
		}
	}
	return tokens, sc.Err()
}

// BuildSupplementLexicon writes to out the words of the v0.19 fallback
// lexicon that primary lacks, converted to v1.0 notation. Entries using a
// phoneme the model does not know are dropped. It returns the word count.
func BuildSupplementLexicon(primary, fallback, tokensPath, out string) (int, error) {
	have, err := LoadLexicon(primary)
	if err != nil {
		return 0, err
	}
	tokens, err := loadTokens(tokensPath)
	if err != nil {
		return 0, err
	}
	in, err := os.Open(fallback)
	if err != nil {
		return 0, fmt.Errorf("open fallback lexicon: %w", err)
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return 0, fmt.Errorf("create lexicon directory: %w", err)
	}
	// A unique temporary name keeps concurrent builds from clobbering each
	// other; the final rename is atomic.
	f, err := os.CreateTemp(filepath.Dir(out), "lexicon-*.tmp")
	if err != nil {
		return 0, fmt.Errorf("create supplement lexicon: %w", err)
	}
	tmp := f.Name()
	w := bufio.NewWriter(f)
	count := 0
	seen := map[string]bool{}
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
outer:
	for sc.Scan() {
		word, phones, ok := strings.Cut(sc.Text(), " ")
		word = strings.ToLower(word)
		if !ok || word == "" || have[word] || seen[word] {
			continue
		}
		phones = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(phones), "||"))
		conv := convertV0Phones(phones)
		if conv == "" {
			continue
		}
		for _, p := range strings.Fields(conv) {
			if !tokens[p] {
				continue outer
			}
		}
		seen[word] = true
		fmt.Fprintf(w, "%s %s\n", word, conv)
		count++
	}
	if err := sc.Err(); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return 0, fmt.Errorf("read fallback lexicon: %w", err)
	}
	if err := w.Flush(); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return 0, fmt.Errorf("write supplement lexicon: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return 0, fmt.Errorf("write supplement lexicon: %w", err)
	}
	if err := os.Rename(tmp, out); err != nil {
		return 0, fmt.Errorf("store supplement lexicon: %w", err)
	}
	return count, nil
}
