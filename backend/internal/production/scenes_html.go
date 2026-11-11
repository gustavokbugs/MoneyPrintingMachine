package production

import (
	"html/template"
	"regexp"
)

var hexRe = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// cssColor returns a validated hex color for the CSS context, or fallback.
func cssColor(v, fallback string) template.CSS {
	if hexRe.MatchString(v) {
		return template.CSS(v)
	}
	return template.CSS(fallback)
}

// The template only receives: validated colors (template.CSS), numbers,
// a style ID from a fixed set, and text that html/template escapes.
const projectHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width={{.Width}}, height={{.Height}}">
<script src="assets/gsap.min.js"></script>
<style>
@font-face { font-family: "Inter"; src: url("assets/inter-latin-400-normal.woff2") format("woff2"); font-weight: 400; }
@font-face { font-family: "Inter"; src: url("assets/inter-latin-600-normal.woff2") format("woff2"); font-weight: 600; }
@font-face { font-family: "Inter"; src: url("assets/inter-latin-800-normal.woff2") format("woff2"); font-weight: 800; }
@font-face { font-family: "Fraunces"; src: url("assets/fraunces-latin-400-normal.woff2") format("woff2"); font-weight: 400; }
@font-face { font-family: "Fraunces"; src: url("assets/fraunces-latin-700-normal.woff2") format("woff2"); font-weight: 700; }
@font-face { font-family: "Fraunces"; src: url("assets/fraunces-latin-900-normal.woff2") format("woff2"); font-weight: 900; }
@font-face { font-family: "Space Grotesk"; src: url("assets/space-grotesk-latin-500-normal.woff2") format("woff2"); font-weight: 500; }
@font-face { font-family: "Space Grotesk"; src: url("assets/space-grotesk-latin-700-normal.woff2") format("woff2"); font-weight: 700; }
@font-face { font-family: "JetBrains Mono"; src: url("assets/jetbrains-mono-latin-400-normal.woff2") format("woff2"); font-weight: 400; }
@font-face { font-family: "JetBrains Mono"; src: url("assets/jetbrains-mono-latin-700-normal.woff2") format("woff2"); font-weight: 700; }
:root {
  --bg: {{bg .}};
  --ink: {{ink .}};
  --accent: {{accent .}};
  --accent2: {{accent2 .}};
  --u: {{unit .Unit}};
}
* { margin: 0; padding: 0; box-sizing: border-box; }
html, body { width: {{.Width}}px; height: {{.Height}}px; overflow: hidden; background: var(--bg); }
#root { position: relative; width: 100%; height: 100%; overflow: hidden; isolation: isolate; font-family: "Inter", sans-serif; }
.bg { position: absolute; inset: 0; overflow: hidden; }
.orb, .grain, .grid, .band { position: absolute; display: none; }
.scene { position: absolute; left: 0; right: 0; top: 0; bottom: {{.SafeBottom}}px; display: flex; flex-direction: column; justify-content: center; padding: 0 calc(var(--u) * 8); gap: calc(var(--u) * 2.5); }
.kicker { font-size: calc(var(--u) * 2.8); letter-spacing: 0.22em; text-transform: uppercase; color: var(--accent); font-weight: 600; }
.headline { color: var(--ink); font-weight: 800; line-height: 1.04; letter-spacing: -0.025em; }
.tier-1 .headline { font-size: calc(var(--u) * 6.2); }
.tier-2 .headline { font-size: calc(var(--u) * 8.4); }
.tier-3 .headline { font-size: calc(var(--u) * 11.5); }
.word { display: inline-block; margin-right: 0.24em; will-change: transform, opacity; }
.bar { width: calc(var(--u) * 14); height: calc(var(--u) * 1.1); background: var(--accent); transform-origin: left center; }
.num { font-size: calc(var(--u) * 24); font-weight: 800; line-height: 0.95; color: var(--accent); font-variant-numeric: tabular-nums; letter-spacing: -0.04em; }
.suffix { font-size: calc(var(--u) * 5.2); font-weight: 600; color: var(--ink); }
.kind-title .headline, .kind-outro .headline { text-align: center; }
.kind-title, .kind-outro { align-items: center; text-align: center; }
.kind-title .bar, .kind-outro .bar { transform-origin: center; }

/* Minimal: soft drifting accent light. */
.style-minimal .orb { display: block; width: calc(var(--u) * 90); height: calc(var(--u) * 90); left: 45%; top: -20%; border-radius: 50%;
  background: radial-gradient(circle, var(--accent) 0%, transparent 65%); opacity: 0.32; filter: blur(calc(var(--u) * 3)); }

/* Documentary: serif type, warm vignette, film grain, slow drift. */
.style-documentary #root { font-family: "Fraunces", serif; }
.style-documentary .headline { font-weight: 700; letter-spacing: -0.01em; }
.style-documentary .kicker { font-family: "Inter", sans-serif; }
.style-documentary .orb { display: block; inset: -10%; background: radial-gradient(ellipse at 50% 40%, transparent 30%, rgba(0,0,0,0.55) 100%); }
.style-documentary .grain { display: block; inset: -50%; opacity: 0.09;
  background-image: url("data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' width='220' height='220'><filter id='n'><feTurbulence type='fractalNoise' baseFrequency='0.85' numOctaves='3' stitchTiles='stitch'/></filter><rect width='100%' height='100%' filter='url(%23n)'/></svg>"); }

/* Bold: big uppercase type over a sweeping color band. */
.style-bold .headline { text-transform: uppercase; letter-spacing: -0.03em; }
.style-bold .band { display: block; left: -25%; width: 150%; height: calc(var(--u) * 34); top: 28%; background: var(--accent2); opacity: 0.85; transform: rotate(-8deg); }
.style-bold .num { color: var(--ink); }

/* Tech: monospace details over a slowly panning grid. */
.style-tech #root { font-family: "Space Grotesk", sans-serif; }
.style-tech .kicker, .style-tech .suffix { font-family: "JetBrains Mono", monospace; letter-spacing: 0.08em; }
.style-tech .grid { display: block; inset: -20%; opacity: 0.55;
  background-image: linear-gradient(rgba(255,255,255,0.07) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,0.07) 1px, transparent 1px);
  background-size: calc(var(--u) * 7) calc(var(--u) * 7); }
.style-tech .orb { display: block; width: calc(var(--u) * 60); height: calc(var(--u) * 60); right: -10%; bottom: 10%; border-radius: 50%;
  background: radial-gradient(circle, var(--accent) 0%, transparent 70%); opacity: 0.22; }
</style>
</head>
<body class="style-{{.Style}}">
<div id="root" data-composition-id="main" data-start="0" data-duration="{{sec .Total}}" data-width="{{.Width}}" data-height="{{.Height}}">
  <div class="bg clip" data-start="0" data-duration="{{sec .Total}}" data-track-index="0">
    <div class="band"></div><div class="orb"></div><div class="grid"></div><div class="grain"></div>
  </div>
{{range $i, $s := .Scenes}}  <section id="{{sid $i}}" class="scene clip tier-{{$s.SizeTier}} kind-{{$s.Kind}}" data-start="{{sec $s.Start}}" data-duration="{{sec $s.Duration}}" data-track-index="1">
{{if $s.Kicker}}    <div class="kicker">{{$s.Kicker}}</div>
{{end}}{{if eq $s.Kind "number"}}    <div class="num">{{$s.Prefix}}<span class="count">0</span></div>
    <div class="suffix">{{$s.Suffix}}</div>
{{else}}    <div class="bar"></div>
    <h1 class="headline">{{range $s.Words}}<span class="word">{{.}}</span>{{end}}</h1>
{{end}}  </section>
{{end}}</div>
<script>
(function () {
  var SCENES = {{.Data}};
  var U = {{.Unit}};
  var TOTAL = {{.Total}};
  var tl = gsap.timeline({ paused: true });

  // Background motion spans the whole film; it is slow and constant.
  tl.to(".orb", { x: U * 18, y: U * 10, scale: 1.15, duration: TOTAL, ease: "none" }, 0);
  tl.to(".grid", { x: U * 7, y: U * 7, duration: TOTAL, ease: "none" }, 0);
  tl.to(".band", { x: U * 12, duration: TOTAL, ease: "none" }, 0);
  tl.to(".grain", { x: U * 3, y: -U * 2, duration: TOTAL, ease: "steps(" + Math.max(1, Math.round(TOTAL * 12)) + ")" }, 0);

  SCENES.forEach(function (s) {
    var el = "#" + s.id;
    var t0 = s.start;
    tl.from(el + " .kicker", { opacity: 0, y: U * 2, duration: 0.45, ease: "power3.out" }, t0 + 0.05);
    if (s.kind === "number") {
      var target = document.querySelector(el + " .count");
      var counter = { v: 0 };
      var fmt = function () {
        target.textContent = counter.v.toLocaleString("en-US", { minimumFractionDigits: s.decimals, maximumFractionDigits: s.decimals });
      };
      fmt();
      tl.from(el + " .num", { opacity: 0, scale: 0.85, duration: 0.5, ease: "back.out(1.6)" }, t0 + 0.1);
      tl.to(counter, { v: s.value, duration: Math.min(1.8, s.duration * 0.5), ease: "power2.out", onUpdate: fmt }, t0 + 0.2);
      tl.from(el + " .suffix", { opacity: 0, y: U * 2, duration: 0.45, ease: "power3.out" }, t0 + 0.6);
    } else {
      tl.from(el + " .bar", { scaleX: 0, duration: 0.55, ease: "power3.out" }, t0 + 0.1);
      tl.from(el + " .word", { opacity: 0, y: U * 4, filter: "blur(10px)", duration: 0.55, stagger: 0.07, ease: "power3.out" }, t0 + 0.18);
    }
    if (s.duration > 1.2) {
      tl.to(el, { opacity: 0, duration: 0.25, ease: "power1.in" }, t0 + s.duration - 0.28);
    }
  });

  window.__timelines["main"] = tl;
  tl.seek(0);
})();
</script>
</body>
</html>`
