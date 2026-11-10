package branding

import (
	"encoding/xml"
	"strings"
	"testing"

	"contentfactory/internal/channel"
)

func identity() channel.Identity {
	return channel.Identity{
		Tagline:    "Stories <from> the dark & beyond",
		Palette:    channel.Palette{Primary: "#1D3557", Secondary: "#457B9D", Accent: "#E63946", Background: "#F1FAEE", Text: "#111111"},
		Typography: channel.Typography{HeadingFont: "Oswald"},
	}
}

// wellFormed checks the SVG parses as XML.
func wellFormed(t *testing.T, svg string) {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(svg))
	for {
		_, err := dec.Token()
		if err != nil {
			if err.Error() == "EOF" {
				return
			}
			t.Fatalf("invalid SVG: %v\n%s", err, svg)
		}
	}
}

func TestAvatarAndBannerAreWellFormedAndEscaped(t *testing.T) {
	name := `Evil"><script>alert(1)</script>`
	for v := Variant(0); v < MaxVariant; v++ {
		a := Avatar(identity(), name, v)
		bn := Banner(identity(), name, v)
		wellFormed(t, a)
		wellFormed(t, bn)
		for _, svg := range []string{a, bn} {
			if strings.Contains(svg, "<script>") {
				t.Fatalf("unescaped markup in variant %d", v)
			}
		}
		if !strings.Contains(a, `width="800" height="800"`) {
			t.Fatalf("avatar must be 800x800")
		}
		if !strings.Contains(bn, `width="2560" height="1440"`) {
			t.Fatalf("banner must be 2560x1440")
		}
		if strings.Contains(a, "<text") {
			t.Fatalf("avatar must not contain text")
		}
	}
}

func TestDeterministicAndVariantsDiffer(t *testing.T) {
	a1 := Avatar(identity(), "Quiet Orbit", 1)
	a2 := Avatar(identity(), "Quiet Orbit", 1)
	if a1 != a2 {
		t.Fatal("avatar output is not deterministic")
	}
	if Avatar(identity(), "Quiet Orbit", 0) == a1 {
		t.Fatal("variants should differ")
	}
	if Avatar(identity(), "Quiet Orbit", 99) != Avatar(identity(), "Quiet Orbit", 0) {
		t.Fatal("out-of-range variant should fall back to 0")
	}
	if ContentKey("avatar", identity(), "Quiet Orbit", 1) == ContentKey("avatar", identity(), "Quiet Orbit", 2) {
		t.Fatal("content keys should differ by variant")
	}
}

func TestFitFontSize(t *testing.T) {
	if got := fitFontSize("Hi", 1000, 150, 64); got != 150 {
		t.Errorf("short text should use max size, got %d", got)
	}
	if got := fitFontSize(strings.Repeat("W", 200), 1000, 150, 64); got != 64 {
		t.Errorf("long text should use min size, got %d", got)
	}
}
