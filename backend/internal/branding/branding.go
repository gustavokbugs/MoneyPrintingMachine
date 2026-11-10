// Package branding renders deterministic brand assets (avatar and banner)
// as SVG from a channel's identity. Rendering is local, needs no GPU, and
// produces the same output for the same input, so assets can be cached by
// content hash and regenerated reliably.
package branding

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"math"
	"strings"

	"contentfactory/internal/channel"
)

// Asset dimensions. YouTube recommends an 800x800 profile picture and a
// 2560x1440 banner whose text-and-logo safe area is the central 1546x423.
const (
	AvatarSize   = 800
	BannerWidth  = 2560
	BannerHeight = 1440
	SafeWidth    = 1546
	SafeHeight   = 423
)

// Variant selects one of the deterministic motif variations so the user
// can regenerate alternatives.
type Variant int

// MaxVariant is the number of motif variations available.
const MaxVariant = 4

func esc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// seed derives stable pseudo-random values from the channel name.
type seed []byte

func newSeed(name string, v Variant) seed {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%d", strings.ToLower(name), v)))
	return h[:]
}

func (s seed) unit(i int) float64 { return float64(s[i%len(s)]) / 255 }

// ContentKey returns a stable cache key for an asset.
func ContentKey(kind string, idn channel.Identity, name string, v Variant) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%d|%s|%s|%s|%s|%s|%s|%s", kind, name, v,
		idn.Palette.Primary, idn.Palette.Secondary, idn.Palette.Accent, idn.Palette.Background,
		idn.Palette.Text, idn.Tagline, idn.Typography.HeadingFont)
	return hex.EncodeToString(h.Sum(nil))[:32]
}

func normalizeVariant(v Variant) Variant {
	if v < 0 || v >= MaxVariant {
		return 0
	}
	return v
}

// motif draws a geometric mark centered at (cx, cy) with radius r. It uses
// no text, so it stays legible at small sizes.
func motif(b *strings.Builder, s seed, v Variant, cx, cy, r float64, p channel.Palette) {
	switch v {
	case 0: // Concentric rings with an offset orbit dot.
		for i := 0; i < 3; i++ {
			rr := r * (1 - float64(i)*0.28)
			fmt.Fprintf(b, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="none" stroke="%s" stroke-width="%.1f"/>`,
				cx, cy, rr, p.Accent, r*0.07)
		}
		a := s.unit(1) * 2 * math.Pi
		fmt.Fprintf(b, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s"/>`,
			cx+math.Cos(a)*r*0.72, cy+math.Sin(a)*r*0.72, r*0.14, p.Secondary)
	case 1: // Stacked chevrons.
		for i := 0; i < 3; i++ {
			y := cy - r*0.45 + float64(i)*r*0.38
			fmt.Fprintf(b, `<polyline points="%.1f,%.1f %.1f,%.1f %.1f,%.1f" fill="none" stroke="%s" stroke-width="%.1f" stroke-linecap="round" stroke-linejoin="round"/>`,
				cx-r*0.6, y+r*0.3, cx, y-r*0.1, cx+r*0.6, y+r*0.3, []string{p.Accent, p.Secondary, p.Text}[i], r*0.12)
		}
	case 2: // Rotated square frame with a core.
		rot := 15 + s.unit(2)*30
		fmt.Fprintf(b, `<g transform="rotate(%.1f %.1f %.1f)"><rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="%.1f" fill="none" stroke="%s" stroke-width="%.1f"/></g>`,
			rot, cx, cy, cx-r*0.7, cy-r*0.7, r*1.4, r*1.4, r*0.12, p.Accent, r*0.1)
		fmt.Fprintf(b, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s"/>`, cx, cy, r*0.3, p.Secondary)
	default: // Radiating bars.
		n := 6 + int(s.unit(3)*4)
		for i := 0; i < n; i++ {
			a := float64(i) / float64(n) * 2 * math.Pi
			fmt.Fprintf(b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="%.1f" stroke-linecap="round"/>`,
				cx+math.Cos(a)*r*0.3, cy+math.Sin(a)*r*0.3, cx+math.Cos(a)*r*0.85, cy+math.Sin(a)*r*0.85,
				[]string{p.Accent, p.Secondary}[i%2], r*0.09)
		}
	}
}

// Avatar renders a square 1:1 profile image without text.
func Avatar(idn channel.Identity, name string, v Variant) string {
	v = normalizeVariant(v)
	p := idn.Palette
	s := newSeed(name, v)
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="%s avatar">`,
		AvatarSize, AvatarSize, AvatarSize, AvatarSize, esc(name))
	c := float64(AvatarSize) / 2
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="%s"/>`, AvatarSize, AvatarSize, p.Primary)
	fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s" opacity="0.18"/>`, c, c, c*0.92, p.Background)
	motif(&b, s, v, c, c, c*0.62, p)
	b.WriteString(`</svg>`)
	return b.String()
}

// Banner renders a 2560x1440 banner. The name and tagline sit inside the
// central safe area so they remain visible on every device crop.
func Banner(idn channel.Identity, name string, v Variant) string {
	v = normalizeVariant(v)
	p := idn.Palette
	s := newSeed(name, v)
	font := idn.Typography.HeadingFont
	if font == "" {
		font = "Inter"
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="%s banner">`,
		BannerWidth, BannerHeight, BannerWidth, BannerHeight, esc(name))
	fmt.Fprintf(&b, `<defs><linearGradient id="bg" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="%s"/><stop offset="1" stop-color="%s"/></linearGradient></defs>`,
		p.Primary, p.Secondary)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="url(#bg)"/>`, BannerWidth, BannerHeight)

	// Background pattern outside the safe area.
	for i := 0; i < 14; i++ {
		x := s.unit(i*2) * BannerWidth
		y := s.unit(i*2+1) * BannerHeight
		r := 40 + s.unit(i+7)*160
		fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="none" stroke="%s" stroke-width="6" opacity="0.18"/>`, x, y, r, p.Background)
	}

	safeX := float64(BannerWidth-SafeWidth) / 2
	safeY := float64(BannerHeight-SafeHeight) / 2
	markR := float64(SafeHeight) * 0.36
	markCX := safeX + markR + 30
	markCY := safeY + float64(SafeHeight)/2
	fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s"/>`, markCX, markCY, markR*1.05, p.Primary)
	motif(&b, s, v, markCX, markCY, markR*0.8, p)

	textX := markCX + markR + 60
	maxTextWidth := safeX + SafeWidth - textX
	nameSize := fitFontSize(name, maxTextWidth, 150, 64)
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="%s, sans-serif" font-size="%d" font-weight="700" fill="%s">%s</text>`,
		textX, markCY+float64(nameSize)*0.1, esc(font), nameSize, p.Background, esc(name))
	if idn.Tagline != "" {
		tagSize := fitFontSize(idn.Tagline, maxTextWidth, 60, 32)
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-family="%s, sans-serif" font-size="%d" fill="%s" opacity="0.9">%s</text>`,
			textX, markCY+float64(nameSize)*0.1+float64(tagSize)*1.5, esc(font), tagSize, p.Background, esc(idn.Tagline))
	}
	b.WriteString(`</svg>`)
	return b.String()
}

// fitFontSize estimates a font size that keeps text within width, assuming
// an average glyph width of 0.58em.
func fitFontSize(text string, width float64, max, min int) int {
	n := len([]rune(text))
	if n == 0 {
		return max
	}
	size := int(width / (float64(n) * 0.58))
	if size > max {
		return max
	}
	if size < min {
		return min
	}
	return size
}
