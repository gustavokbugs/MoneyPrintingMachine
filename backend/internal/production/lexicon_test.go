package production

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConvertV0Phones(t *testing.T) {
	cases := map[string]string{
		"t ə mˈ e ɪ ɾ o ʊ":          "t ə m ˈ A ɾ O",
		"wˈ ɔː ɾ ɚ":                 "w ˈ ɔ ɾ ə ɹ",
		"s ˈ ɛ n t ɪ m ˌ i ː t ɚ z": "s ˈ ɛ n t ɪ m ˌ i t ə ɹ z",
		"dʒ ˈ ʌ dʒ":                 "ʤ ˈ ʌ ʤ",
	}
	for in, want := range cases {
		if got := convertV0Phones(in); got != want {
			t.Errorf("convertV0Phones(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildSupplementLexicon(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	primary := write("v1.txt", "water w ˈ ɔ ɾ ə ɹ\n")
	fallback := write("v0.txt", "water || wˈ ɔː ɾ ɚ\nCentimeters || s ˈ ɛ n t ɪ m ˌ i ː t ɚ z\nweird || w ˈ ɪ ɹ ∅\ncentimeters || s ɛ\n")
	tokens := write("tokens.txt", "ˈ 1\nˌ 2\nw 3\nɔ 4\nɾ 5\nə 6\nɹ 7\ns 8\nɛ 9\nn 10\nt 11\nɪ 12\nm 13\ni 14\nz 15\n")
	out := filepath.Join(dir, "cache", "sup.txt")
	n, err := BuildSupplementLexicon(primary, fallback, tokens, out)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	// "water" is already in the primary lexicon, "weird" uses an unknown
	// phoneme, and the duplicate "centimeters" keeps its first entry.
	if n != 1 || got != "centimeters s ˈ ɛ n t ɪ m ˌ i t ə ɹ z\n" {
		t.Fatalf("n=%d supplement=%q", n, got)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "cache", "*.tmp"))
	if len(matches) != 0 {
		t.Fatalf("temporary files left: %v", matches)
	}
	if _, err := BuildSupplementLexicon(primary, filepath.Join(dir, "missing.txt"), tokens, out); err == nil || !strings.Contains(err.Error(), "fallback") {
		t.Fatalf("missing fallback: %v", err)
	}
}

func TestLexiconArgIncludesSupplement(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "lexicon-us-en.txt"), []byte("a a\n"), 0o644)
	tools := Tools{ModelDir: dir}
	if tools.FallbackLexicon() != "" {
		t.Fatal("no fallback installed")
	}
	_ = os.WriteFile(filepath.Join(dir, "lexicon-en-us.txt"), []byte("a || a\n"), 0o644)
	if tools.FallbackLexicon() == "" {
		t.Fatal("fallback not detected")
	}
	tools.ExtraLexicon = "sup.txt"
	if got := tools.LexiconArg(); got != filepath.Join(dir, "lexicon-us-en.txt")+",sup.txt" {
		t.Fatalf("LexiconArg = %s", got)
	}
}
