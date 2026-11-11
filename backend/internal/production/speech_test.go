package production

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNumberWords(t *testing.T) {
	for n, want := range map[int64]string{0: "zero", 15: "fifteen", 38: "thirty eight", 100: "one hundred", 1005: "one thousand five", 384400: "three hundred eighty four thousand four hundred", 2_000_000: "two million"} {
		if got := NumberWords(n); got != want {
			t.Errorf("NumberWords(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestSpellNumber(t *testing.T) {
	cases := map[string]string{
		"3.8": "three point eight", "1969": "nineteen sixty nine", "2026": "twenty twenty six", "2005": "two thousand five",
		"1900": "nineteen hundred", "1905": "nineteen oh five", "1,500": "one thousand five hundred", "21st": "twenty first",
		"3rd": "third", "40%": "forty percent", "$5": "five dollars", "12th": "twelfth", "90th": "ninetieth", "5000": "five thousand",
	}
	for in, want := range cases {
		if got := spellNumber(in); got != want {
			t.Errorf("spellNumber(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeForSpeech(t *testing.T) {
	lex := Lexicon{}
	for _, w := range strings.Fields("the moon drifts about three point eight centimeters away from earth every year measured by nasa lasers in nineteen sixty nine forty percent rock and roll well known") {
		lex[w] = true
	}
	got, oov := NormalizeForSpeech("The Moon drifts about 3.8 centimeters away from Earth's every year, measured by NASA's lasers in 1969: 40% rock & roll. Well-known Zorblax!", lex)
	want := "The Moon drifts about three point eight centimeters away from Earth every year, measured by NASA lasers in nineteen sixty nine: forty percent rock and roll. Well known Zorblax!"
	if got != want {
		t.Fatalf("normalized:\n got %q\nwant %q", got, want)
	}
	if strings.Join(oov, ",") != "Zorblax" {
		t.Fatalf("oov = %v", oov)
	}
}

func TestLoadLexicon(t *testing.T) {
	p := filepath.Join(t.TempDir(), "lex.txt")
	_ = os.WriteFile(p, []byte("# header\nMoon || m u n\nearth || e r th\n"), 0o644)
	lex, err := LoadLexicon(p)
	if err != nil || !lex["moon"] || !lex["earth"] || len(lex) != 2 {
		t.Fatalf("lex = %v %v", lex, err)
	}
}
