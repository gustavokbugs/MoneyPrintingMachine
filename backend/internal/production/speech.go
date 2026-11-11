package production

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Lexicon is the set of words the TTS engine can pronounce.
type Lexicon map[string]bool

// LoadLexicon reads a sherpa-onnx lexicon ("word || phones" per line).
func LoadLexicon(path string) (Lexicon, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open lexicon: %w", err)
	}
	defer f.Close()
	lex := make(Lexicon, 170000)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == '#' {
			continue
		}
		word, _, ok := strings.Cut(line, " ")
		if ok {
			lex[strings.ToLower(word)] = true
		}
	}
	return lex, sc.Err()
}

// UnspeakableError lists words the voice cannot pronounce. The engine would
// silently skip them, which could change the meaning of a verified claim,
// so production stops instead.
type UnspeakableError struct{ Words []string }

func (e *UnspeakableError) Error() string {
	return fmt.Sprintf("the narration voice cannot pronounce: %s. Rewrite or spell out these words in the script, then produce again", strings.Join(e.Words, ", "))
}

var ones = []string{"zero", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten",
	"eleven", "twelve", "thirteen", "fourteen", "fifteen", "sixteen", "seventeen", "eighteen", "nineteen"}
var tens = []string{"", "", "twenty", "thirty", "forty", "fifty", "sixty", "seventy", "eighty", "ninety"}

// NumberWords spells a non-negative integer in English without hyphens
// (the lexicon has "thirty" and "eight" but not "thirty-eight").
func NumberWords(n int64) string {
	if n < 20 {
		return ones[n]
	}
	if n < 100 {
		if n%10 == 0 {
			return tens[n/10]
		}
		return tens[n/10] + " " + ones[n%10]
	}
	if n < 1000 {
		s := ones[n/100] + " hundred"
		if n%100 != 0 {
			s += " " + NumberWords(n%100)
		}
		return s
	}
	for _, u := range []struct {
		v    int64
		name string
	}{{1_000_000_000_000, "trillion"}, {1_000_000_000, "billion"}, {1_000_000, "million"}, {1000, "thousand"}} {
		if n >= u.v {
			s := NumberWords(n/u.v) + " " + u.name
			if n%u.v != 0 {
				s += " " + NumberWords(n%u.v)
			}
			return s
		}
	}
	return strconv.FormatInt(n, 10)
}

// yearWords reads a four-digit year the way it is spoken ("nineteen sixty nine").
func yearWords(n int64) string {
	switch {
	case n >= 2000 && n < 2010:
		return NumberWords(n)
	case n%100 == 0:
		return NumberWords(n/100) + " hundred"
	case n%100 < 10:
		return NumberWords(n/100) + " oh " + ones[n%100]
	default:
		return NumberWords(n/100) + " " + NumberWords(n%100)
	}
}

var ordinalFix = map[string]string{"one": "first", "two": "second", "three": "third", "five": "fifth", "eight": "eighth", "nine": "ninth", "twelve": "twelfth"}

func ordinalWords(n int64) string {
	w := NumberWords(n)
	parts := strings.Fields(w)
	last := parts[len(parts)-1]
	switch {
	case ordinalFix[last] != "":
		last = ordinalFix[last]
	case strings.HasSuffix(last, "y"):
		last = strings.TrimSuffix(last, "y") + "ieth"
	default:
		last += "th"
	}
	parts[len(parts)-1] = last
	return strings.Join(parts, " ")
}

var (
	numberRe  = regexp.MustCompile(`\$?\d[\d,]*(?:\.\d+)?(?:st|nd|rd|th)?%?`)
	digitsOne = []string{"zero", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine"}
)

func spellNumber(tok string) string {
	dollar := strings.HasPrefix(tok, "$")
	pct := strings.HasSuffix(tok, "%")
	tok = strings.TrimSuffix(strings.TrimPrefix(tok, "$"), "%")
	ordinal := false
	for _, suf := range []string{"st", "nd", "rd", "th"} {
		if strings.HasSuffix(tok, suf) {
			tok, ordinal = strings.TrimSuffix(tok, suf), true
		}
	}
	intPart, frac, hasFrac := strings.Cut(tok, ".")
	hadComma := strings.Contains(intPart, ",")
	n, err := strconv.ParseInt(strings.ReplaceAll(intPart, ",", ""), 10, 64)
	if err != nil {
		return tok
	}
	var out string
	switch {
	case ordinal && !hasFrac:
		out = ordinalWords(n)
	case !hasFrac && !hadComma && !dollar && len(intPart) == 4 && n >= 1100 && n <= 2099:
		out = yearWords(n)
	default:
		out = NumberWords(n)
	}
	if hasFrac {
		var digits []string
		for _, d := range frac {
			digits = append(digits, digitsOne[d-'0'])
		}
		out += " point " + strings.Join(digits, " ")
	}
	if pct {
		out += " percent"
	}
	if dollar {
		out += " dollars"
	}
	return out
}

// NormalizeForSpeech rewrites text so every word is in the lexicon and
// returns any words that still cannot be pronounced.
func NormalizeForSpeech(text string, lex Lexicon) (string, []string) {
	text = numberRe.ReplaceAllStringFunc(text, spellNumber)
	text = strings.NewReplacer("&", " and ", "%", " percent", "—", ", ", "–", " to ", "/", " ").Replace(text)
	var words []string
	oov := map[string]bool{}
	for _, raw := range strings.Fields(text) {
		// Keep sentence punctuation attached for prosody.
		core := strings.TrimFunc(raw, func(r rune) bool { return strings.ContainsRune(`.,!?;:"()[]'’`, r) })
		lead := raw[:strings.Index(raw, core)]
		trail := raw[strings.Index(raw, core)+len(core):]
		if core == "" {
			words = append(words, raw)
			continue
		}
		var parts []string
		for _, p := range strings.Split(strings.ReplaceAll(core, "’", "'"), "-") {
			if p == "" {
				continue
			}
			lw := strings.ToLower(p)
			switch {
			case lex[lw]:
				parts = append(parts, p)
			case strings.HasSuffix(lw, "'s") && lex[strings.TrimSuffix(lw, "'s")]:
				parts = append(parts, strings.TrimSuffix(strings.TrimSuffix(p, "'s"), "'S"))
			case spellable(p, lex):
				// Acronyms the voice does not know are read letter by
				// letter ("POV" as "P O V").
				parts = append(parts, strings.Join(strings.Split(lw, ""), " "))
			default:
				parts = append(parts, p)
				oov[p] = true
			}
		}
		words = append(words, lead+strings.Join(parts, " ")+trail)
	}
	var missing []string
	for w := range oov {
		missing = append(missing, w)
	}
	sort.Strings(missing)
	return strings.Join(words, " "), missing
}

// spellable reports whether w is an all-capital acronym of 2 to 5 letters
// whose letters the voice can say one by one.
func spellable(w string, lex Lexicon) bool {
	if n := len(w); n < 2 || n > 5 {
		return false
	}
	for _, r := range w {
		if r < 'A' || r > 'Z' || !lex[strings.ToLower(string(r))] {
			return false
		}
	}
	return true
}
