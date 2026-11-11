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
//
// Motion design: every scene change is covered by a transition (panel,
// iris, or split bars, in rotation) timed with the whoosh effect; the hook
// slams in with a screen shake on the impact; scenes push in slowly;
// headlines highlight one key word; counters draw a ring while ticking;
// captions pop word by word; particles and a progress bar keep the frame
// alive. Timings must stay in sync with PlanEffects (audio.go).
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
.particles { position: absolute; inset: 0; }
.p { position: absolute; display: block; border-radius: 50%; background: var(--ink); }
.scene { position: absolute; left: 0; right: 0; top: 0; bottom: {{.SafeBottom}}px; }
.inner { position: absolute; inset: 0; display: flex; flex-direction: column; justify-content: center; padding: 0 calc(var(--u) * 8); gap: calc(var(--u) * 2.5); }
.kicker { font-size: calc(var(--u) * 2.8); letter-spacing: 0.22em; text-transform: uppercase; color: var(--accent); font-weight: 600; }
.headline { color: var(--ink); font-weight: 800; line-height: 1.06; letter-spacing: -0.025em; }
.tier-1 .headline { font-size: calc(var(--u) * 6.2); }
.tier-2 .headline { font-size: calc(var(--u) * 8.4); }
.tier-3 .headline { font-size: calc(var(--u) * 11.5); }
.word { display: inline-block; margin-right: 0.24em; will-change: transform, opacity; }
.em { color: var(--accent); background-image: linear-gradient(var(--accent2), var(--accent2)); background-repeat: no-repeat;
  background-position: 0 92%; background-size: 0% 26%; }
.bar { width: calc(var(--u) * 14); height: calc(var(--u) * 1.1); background: var(--accent); transform-origin: left center; }
.numwrap { position: relative; display: flex; align-items: center; justify-content: center; width: calc(var(--u) * 64); height: calc(var(--u) * 64); align-self: center; }
.ring { position: absolute; inset: 0; width: 100%; height: 100%; transform: rotate(-90deg); }
.ring circle { fill: none; stroke-width: 3; }
.ring .track { stroke: var(--ink); opacity: 0.12; }
.ring .arc { stroke: var(--accent); stroke-linecap: round; stroke-dasharray: 283; stroke-dashoffset: 283; }
.num { position: relative; font-size: calc(var(--u) * 17); font-weight: 800; line-height: 0.95; color: var(--ink); font-variant-numeric: tabular-nums; letter-spacing: -0.04em; text-align: center; }
.suffix { font-size: calc(var(--u) * 5.2); font-weight: 600; color: var(--ink); text-align: center; }
.kind-number .inner { align-items: center; text-align: center; }
.kind-title .inner, .kind-outro .inner { align-items: center; text-align: center; }
.kind-title .bar, .kind-outro .bar { transform-origin: center; }
.progress { position: absolute; left: 0; right: 0; top: 0; height: calc(var(--u) * 0.7); background: rgba(255,255,255,0.12); z-index: 6; }
.progress-fill { width: 100%; height: 100%; background: var(--accent); transform-origin: left center; transform: scaleX(0); }
.captions { position: absolute; left: calc(var(--u) * 6); right: calc(var(--u) * 6); bottom: {{.CaptionBottom}}px; height: calc(var(--u) * 20); z-index: 4; }
.cap { position: absolute; left: 0; right: 0; bottom: 0; text-align: center; visibility: hidden; opacity: 0;
  font-family: "Inter", sans-serif; font-weight: 800; font-size: calc(var(--u) * 7.4); line-height: 1.18; color: #FFFFFF; letter-spacing: -0.01em;
  text-shadow: 0 calc(var(--u) * 0.35) calc(var(--u) * 1.2) rgba(0,0,0,0.85), 0 0 calc(var(--u) * 0.3) rgba(0,0,0,0.9); }
.cw { display: inline-block; margin: 0 0.1em; transform-origin: center bottom; }
.wipe { position: absolute; z-index: 5; visibility: hidden; }
.wipe-a { inset: 0; background: var(--accent); }
.wipe-b { inset: 0; background: var(--accent2); }
.wipe-top, .wipe-bot { left: 0; right: 0; height: 50.5%; background: var(--accent); }
.wipe-top { top: 0; } .wipe-bot { bottom: 0; }

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
.style-documentary .p { background: #FFE8C2; }

/* Bold: big uppercase type over a sweeping color band. */
.style-bold .headline { text-transform: uppercase; letter-spacing: -0.03em; }
.style-bold .band { display: block; left: -25%; width: 150%; height: calc(var(--u) * 34); top: 28%; background: var(--accent2); opacity: 0.85; transform: rotate(-8deg); }
.style-bold .p { background: transparent; border: calc(var(--u) * 0.5) solid var(--accent); }

/* Tech: monospace details over a slowly panning grid. */
.style-tech #root { font-family: "Space Grotesk", sans-serif; }
.style-tech .kicker, .style-tech .suffix { font-family: "JetBrains Mono", monospace; letter-spacing: 0.08em; }
.style-tech .grid { display: block; inset: -20%; opacity: 0.55;
  background-image: linear-gradient(rgba(255,255,255,0.07) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,0.07) 1px, transparent 1px);
  background-size: calc(var(--u) * 7) calc(var(--u) * 7); }
.style-tech .orb { display: block; width: calc(var(--u) * 60); height: calc(var(--u) * 60); right: -10%; bottom: 10%; border-radius: 50%;
  background: radial-gradient(circle, var(--accent) 0%, transparent 70%); opacity: 0.22; }
.style-tech .p { border-radius: 0; background: var(--accent); }
</style>
</head>
<body class="style-{{.Style}}">
<div id="root" data-composition-id="main" data-start="0" data-duration="{{sec .Total}}" data-width="{{.Width}}" data-height="{{.Height}}">
  <div class="bg clip" data-start="0" data-duration="{{sec .Total}}" data-track-index="0">
    <div class="band"></div><div class="orb"></div><div class="grid"></div><div class="particles"></div><div class="grain"></div>
  </div>
{{range $i, $s := .Scenes}}  <section id="{{sid $i}}" class="scene clip tier-{{$s.SizeTier}} kind-{{$s.Kind}}" data-start="{{sec $s.Start}}" data-duration="{{sec $s.Duration}}" data-track-index="1">
    <div class="inner">
{{if $s.Kicker}}      <div class="kicker">{{$s.Kicker}}</div>
{{end}}{{if eq $s.Kind "number"}}      <div class="numwrap">
        <svg class="ring" viewBox="0 0 100 100" aria-hidden="true"><circle class="track" cx="50" cy="50" r="45"/><circle class="arc" cx="50" cy="50" r="45"/></svg>
        <div class="num">{{$s.Prefix}}<span class="count">0</span></div>
      </div>
      <div class="suffix">{{$s.Suffix}}</div>
{{else}}      <div class="bar"></div>
      <h1 class="headline">{{range $j, $w := $s.Words}}<span class="word{{if eq $j $s.Emph}} em{{end}}">{{$w}}</span>{{end}}</h1>
{{end}}    </div>
  </section>
{{end}}  <div class="captions clip" data-start="0" data-duration="{{sec .Total}}" data-track-index="2">
{{range $i, $c := .Captions}}    <div class="cap" id="{{cid $i}}">{{range $j, $w := $c.Words}}<span class="cw" id="{{wid $i $j}}">{{$w.Text}}</span> {{end}}</div>
{{end}}  </div>
  <div class="fx clip" data-start="0" data-duration="{{sec .Total}}" data-track-index="3">
    <div class="wipe wipe-a"></div><div class="wipe wipe-b"></div><div class="wipe wipe-top"></div><div class="wipe wipe-bot"></div>
    <div class="progress"><div class="progress-fill"></div></div>
  </div>
</div>
<script>
(function () {
  var SCENES = {{.Data}};
  var CAPS = {{.CaptionData}};
  var U = {{.Unit}};
  var TOTAL = {{.Total}};
  var seed = {{.Seed}};
  var css = getComputedStyle(document.documentElement);
  var ACCENT = css.getPropertyValue("--accent").trim();
  var STYLE = document.body.className.replace("style-", "");
  var tl = gsap.timeline({ paused: true });
  // Optional elements (kicker, highlight) are only animated when present.
  function has(sel) { return document.querySelector(sel) !== null; }

  // Deterministic randomness (mulberry32) so every render is identical.
  function rand() {
    seed = (seed + 0x6D2B79F5) | 0;
    var t = seed;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  }

  // Background motion spans the whole film; it is slow and constant.
  tl.to(".orb", { x: U * 18, y: U * 10, scale: 1.15, duration: TOTAL, ease: "none" }, 0);
  tl.to(".grid", { x: U * 7, y: U * 7, duration: TOTAL, ease: "none" }, 0);
  tl.to(".band", { x: U * 12, duration: TOTAL, ease: "none" }, 0);
  tl.to(".grain", { x: U * 3, y: -U * 2, duration: TOTAL, ease: "steps(" + Math.max(1, Math.round(TOTAL * 12)) + ")" }, 0);
  tl.to(".progress-fill", { scaleX: 1, duration: TOTAL, ease: "none" }, 0);

  // Floating particles, styled per theme.
  var counts = { minimal: 18, documentary: 46, bold: 7, tech: 30 };
  var field = document.querySelector(".particles");
  for (var i = 0; i < (counts[STYLE] || 18); i++) {
    var d = document.createElement("i");
    d.className = "p";
    var size = STYLE === "bold" ? U * (6 + rand() * 10) : STYLE === "documentary" ? U * (0.2 + rand() * 0.5) : U * (0.4 + rand() * 1.2);
    d.style.width = size + "px";
    d.style.height = size + "px";
    d.style.left = (rand() * 100) + "%";
    d.style.top = (rand() * 110 - 5) + "%";
    d.style.opacity = String(STYLE === "bold" ? 0.18 : 0.12 + rand() * 0.4);
    field.appendChild(d);
    tl.to(d, { y: -U * (8 + rand() * 30), x: U * (rand() * 10 - 5), rotation: STYLE === "bold" || STYLE === "tech" ? rand() * 360 - 180 : 0,
      duration: TOTAL, ease: "none" }, 0);
  }

  // Scene transitions in rotation, centered on the cut at time t.
  var HALF = 0.22;
  function transition(kind, t) {
    if (kind === 0) {
      tl.set(".wipe-a", { visibility: "visible", xPercent: -101 }, t - HALF);
      tl.to(".wipe-a", { xPercent: 0, duration: HALF, ease: "power3.in" }, t - HALF);
      tl.to(".wipe-a", { xPercent: 101, duration: HALF + 0.05, ease: "power3.out" }, t);
      tl.set(".wipe-a", { visibility: "hidden" }, t + HALF + 0.05);
    } else if (kind === 1) {
      tl.set(".wipe-b", { visibility: "visible", clipPath: "circle(0% at 50% 42%)" }, t - HALF);
      tl.to(".wipe-b", { clipPath: "circle(120% at 50% 42%)", duration: HALF, ease: "power2.in" }, t - HALF);
      tl.to(".wipe-b", { opacity: 0, duration: HALF + 0.1, ease: "power1.out" }, t);
      tl.set(".wipe-b", { visibility: "hidden", opacity: 1 }, t + HALF + 0.1);
    } else {
      tl.set([".wipe-top", ".wipe-bot"], { visibility: "visible" }, t - HALF);
      tl.fromTo(".wipe-top", { yPercent: -101 }, { yPercent: 0, duration: HALF, ease: "power3.in", immediateRender: false }, t - HALF);
      tl.fromTo(".wipe-bot", { yPercent: 101 }, { yPercent: 0, duration: HALF, ease: "power3.in", immediateRender: false }, t - HALF);
      tl.to(".wipe-top", { yPercent: -101, duration: HALF + 0.05, ease: "power3.out" }, t);
      tl.to(".wipe-bot", { yPercent: 101, duration: HALF + 0.05, ease: "power3.out" }, t);
      tl.set([".wipe-top", ".wipe-bot"], { visibility: "hidden" }, t + HALF + 0.05);
    }
  }

  SCENES.forEach(function (s, idx) {
    var el = "#" + s.id;
    var t0 = s.start;
    var last = idx === SCENES.length - 1;
    if (idx > 0) transition((idx - 1) % 3, t0);
    // Slow push-in keeps every scene moving.
    tl.fromTo(el + " .inner", { scale: 1 }, { scale: 1.07, duration: s.duration, ease: "none", immediateRender: false }, t0);
    if (has(el + " .kicker")) tl.from(el + " .kicker", { opacity: 0, x: -U * 4, duration: 0.45, ease: "power3.out", immediateRender: false }, t0 + 0.12);
    if (s.kind === "number") {
      var target = document.querySelector(el + " .count");
      var counter = { v: 0 };
      var fmt = function () {
        target.textContent = counter.v.toLocaleString("en-US", { minimumFractionDigits: s.decimals, maximumFractionDigits: s.decimals });
      };
      fmt();
      var dur = Math.min(1.8, s.duration * 0.5);
      tl.from(el + " .numwrap", { opacity: 0, scale: 0.6, duration: 0.5, ease: "back.out(1.8)", immediateRender: false }, t0 + 0.05);
      tl.to(el + " .arc", { strokeDashoffset: 0, duration: dur, ease: "power2.out" }, t0 + 0.2);
      tl.to(counter, { v: s.value, duration: dur, ease: "power2.out", onUpdate: fmt }, t0 + 0.2);
      tl.to(el + " .num", { scale: 1.12, duration: 0.12, yoyo: true, repeat: 1, ease: "power2.out" }, t0 + 0.2 + dur);
      tl.from(el + " .suffix", { opacity: 0, y: U * 3, duration: 0.45, ease: "power3.out", immediateRender: false }, t0 + 0.5);
    } else if (s.kind === "title") {
      // The hook slams in on the impact sound with a short screen shake.
      tl.from(el + " .word", { opacity: 0, scale: 2.2, filter: "blur(14px)", duration: 0.42, stagger: 0.06, ease: "power4.out", immediateRender: false }, t0 + 0.02);
      tl.from(el + " .bar", { scaleX: 0, duration: 0.5, ease: "power3.out", immediateRender: false }, t0 + 0.3);
      tl.to("#root", { keyframes: [{ x: U * 1.4, y: -U * 0.6 }, { x: -U * 1.1, y: U * 0.5 }, { x: U * 0.6, y: 0 }, { x: 0, y: 0 }], duration: 0.32, ease: "none" }, t0 + 0.06);
    } else {
      tl.from(el + " .bar", { scaleX: 0, duration: 0.5, ease: "power3.out", immediateRender: false }, t0 + 0.1);
      tl.from(el + " .word", { opacity: 0, y: U * 6, rotation: 5, duration: 0.5, stagger: 0.06, ease: "back.out(1.7)", immediateRender: false }, t0 + 0.16);
    }
    if (has(el + " .em")) tl.to(el + " .em", { backgroundSize: "100% 26%", duration: 0.45, ease: "power2.out" }, t0 + 0.75);
    if (last && s.duration > 1.2) {
      tl.to(el, { opacity: 0, duration: 0.4, ease: "power1.in" }, t0 + s.duration - 0.45);
    }
  });

  // Captions: each line pops in; the spoken word lights up.
  CAPS.forEach(function (c) {
    var line = document.getElementById(c.id);
    tl.set(line, { autoAlpha: 1 }, c.start);
    tl.from(line, { scale: 0.85, y: U * 1.5, duration: 0.14, ease: "back.out(2.2)", immediateRender: false }, c.start);
    tl.set(line, { autoAlpha: 0 }, c.end);
    c.words.forEach(function (at, j) {
      var w = document.getElementById(c.id + "w" + j);
      var next = j + 1 < c.words.length ? c.words[j + 1] : c.end;
      tl.set(w, { color: ACCENT, scale: 1.08 }, at);
      tl.set(w, { color: "#FFFFFF", scale: 1 }, next);
    });
  });

  window.__timelines["main"] = tl;
  tl.seek(0);
})();
</script>
</body>
</html>`
