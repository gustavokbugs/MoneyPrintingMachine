package production

import "html/template"

// The gameplay story format: an original, procedurally generated runner
// game plays in the background (drawn on a canvas as a pure function of
// time, so seeking renders any frame exactly), the narration appears one
// word at a time in the middle of the screen, and the hook sits on a card
// at the top. Nothing in the background comes from an existing game.
//
// The template receives the same validated values as projectHTML; text is
// escaped by html/template.
var storyTemplate = template.Must(template.New("story").Funcs(templateFuncs).Parse(storyHTML))

const storyHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width={{.Width}}, height={{.Height}}">
<script src="assets/gsap.min.js"></script>
<style>
@font-face { font-family: "Inter"; src: url("assets/inter-latin-600-normal.woff2") format("woff2"); font-weight: 600; }
@font-face { font-family: "Inter"; src: url("assets/inter-latin-800-normal.woff2") format("woff2"); font-weight: 800; }
:root { --accent: {{accent .}}; --u: {{unit .Unit}}; }
* { margin: 0; padding: 0; box-sizing: border-box; }
html, body { width: {{.Width}}px; height: {{.Height}}px; overflow: hidden; background: #1b1430; }
#root { position: relative; width: 100%; height: 100%; overflow: hidden; font-family: "Inter", sans-serif; }
#game { position: absolute; inset: 0; width: 100%; height: 100%; }
.hook { position: absolute; left: calc(var(--u) * 7); right: calc(var(--u) * 7); top: 9%; z-index: 3;
  background: #FFFFFF; color: #111111; border-radius: calc(var(--u) * 3.2); padding: calc(var(--u) * 4) calc(var(--u) * 5);
  box-shadow: 0 calc(var(--u) * 1.4) calc(var(--u) * 4) rgba(0,0,0,0.35); }
.hook .who { display: flex; align-items: center; gap: calc(var(--u) * 2); font-weight: 800; font-size: calc(var(--u) * 3.6); margin-bottom: calc(var(--u) * 2); }
.hook .dot { width: calc(var(--u) * 6.5); height: calc(var(--u) * 6.5); border-radius: 50%; background: var(--accent); flex: none; }
.hook .title { font-weight: 800; font-size: calc(var(--u) * 5.6); line-height: 1.12; letter-spacing: -0.01em; }
.caps { position: absolute; left: 0; right: 0; top: 0; bottom: 0; z-index: 4; pointer-events: none; }
.cap { position: absolute; left: calc(var(--u) * 5); right: calc(var(--u) * 5); top: {{if .PopData}}60%{{else}}62%{{end}}; transform-origin: 50% 50%;
  text-align: center; visibility: hidden; color: #FFFFFF; font-weight: 800; font-size: calc(var(--u) * 12.5); line-height: 1.05;
  letter-spacing: -0.015em; -webkit-text-stroke: calc(var(--u) * 1.5) #000000; paint-order: stroke fill;
  filter: drop-shadow(0 calc(var(--u) * 0.8) calc(var(--u) * 0.6) rgba(0,0,0,0.55)); }
.cap.hot { color: #FFE14D; }
.cap.problem { color: #FF4D4D; }
.cap.spk-b { color: #7FDBFF; }
.cap.spk-b.hot { color: #FFE14D; }
.pop { position: absolute; left: 50%; top: 20%; width: 80%; height: 34%; z-index: 4; visibility: hidden;
  margin-left: -40%; background: #FFFFFF; padding: calc(var(--u) * 1.6); border-radius: calc(var(--u) * 3);
  box-shadow: 0 calc(var(--u) * 2) calc(var(--u) * 6) rgba(0,0,0,0.5); }
.pop img { display: block; width: 100%; height: 100%; object-fit: cover; border-radius: calc(var(--u) * 1.8); }
.end { position: absolute; left: 50%; bottom: 11%; z-index: 3; transform: translateX(-50%); visibility: hidden; white-space: nowrap;
  background: var(--accent); color: #FFFFFF; font-weight: 800; font-size: calc(var(--u) * 4.4); padding: calc(var(--u) * 2.2) calc(var(--u) * 5);
  border-radius: 999px; box-shadow: 0 calc(var(--u) * 1) calc(var(--u) * 3) rgba(0,0,0,0.35); }
.progress { position: absolute; left: 0; right: 0; top: 0; height: calc(var(--u) * 0.8); background: rgba(255,255,255,0.18); z-index: 6; }
.progress-fill { width: 100%; height: 100%; background: #FFE14D; transform-origin: left center; transform: scaleX(0); }
</style>
</head>
<body>
<div id="root" data-composition-id="main" data-start="0" data-duration="{{sec .Total}}" data-width="{{.Width}}" data-height="{{.Height}}">
{{if .BuildJSON}}  <canvas id="gl3d" class="clip" data-start="0" data-duration="{{sec .Total}}" data-track-index="0" width="{{.Width}}" height="{{.Height}}"
    style="position:absolute;inset:0;width:100%;height:100%"></canvas>
{{else if .HasBG}}  <video id="bgv" class="clip" src="assets/bg.mp4" muted playsinline data-start="0" data-duration="{{sec .Total}}" data-track-index="0"
    style="position:absolute;inset:0;width:100%;height:100%;object-fit:cover"></video>
{{else}}  <div class="bg clip" data-start="0" data-duration="{{sec .Total}}" data-track-index="0">
    <canvas id="game" width="{{.Width}}" height="{{.Height}}"></canvas>
  </div>
{{end}}
{{if .Scenes}}{{with index .Scenes 0}}  <div class="hook clip" id="hook" data-start="0" data-duration="{{sec .Duration}}" data-track-index="1">
    <div class="who"><span class="dot"></span><span>{{$.ChannelName}}</span></div>
    <div class="title">{{if $.HookText}}{{$.HookText}}{{else}}{{.Headline}}{{end}}</div>
  </div>
{{end}}{{end}}  <div class="pops clip" data-start="0" data-duration="{{sec .Total}}" data-track-index="4">
{{range .PopData}}    <div class="pop" id="{{.ID}}"><img src="{{.Src}}" alt=""></div>
{{end}}  </div>
  <div class="caps clip" data-start="0" data-duration="{{sec .Total}}" data-track-index="2">
{{range $i, $c := .Captions}}    <div class="cap{{if $c.Hot}} hot{{end}}{{if $c.Problem}} problem{{end}}{{if eq $c.Speaker "B"}} spk-b{{end}}" id="{{cid $i}}" style="{{capsize $c}}">{{range $c.Words}}{{.Text}} {{end}}</div>
{{end}}  </div>
  <div class="fx clip" data-start="0" data-duration="{{sec .Total}}" data-track-index="3">
    <div class="end" id="end">{{if .FollowText}}{{.FollowText}}{{else}}Follow {{.ChannelName}} for more{{end}}</div>
    <div class="progress"><div class="progress-fill"></div></div>
  </div>
</div>
{{if .BuildJSON}}<script>window.BUILD = {{.BuildJSON}};</script>
<script src="assets/three.js"></script>
<script src="assets/scene.js"></script>
{{end}}<script>
(function () {
  var CAPS = {{.CaptionData}};
  var POPS = {{.PopData}} || [];
  var U = {{.Unit}};
  var TOTAL = {{.Total}};
  var W = {{.Width}}, H = {{.Height}};
  var seed = {{.Seed}};
  var tl = gsap.timeline({ paused: true });

  // Deterministic randomness (mulberry32) so every render is identical.
  function rand() {
    seed = (seed + 0x6D2B79F5) | 0;
    var t = seed;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  }
  function lerp(a, b, k) { return a + (b - a) * k; }

  // ---- World ---------------------------------------------------------
  var THEMES = [
    { sky: ["#2b1b5a", "#7b3fa0", "#ff8a7a"], sun: "#ffd27a", hills: ["#5a3a8c", "#40286e", "#2a1a4d"], top: "#ff9ecf", side: "#3a2163", ball: "#ffe14d", coin: "#fff1a8", crate: "#ff7a59" },
    { sky: ["#0f3b57", "#1f7a8c", "#a8e6cf"], sun: "#fff6c9", hills: ["#2f8f83", "#1f6f6a", "#134e4e"], top: "#b8f2a0", side: "#145a4f", ball: "#ffffff", coin: "#ffe680", crate: "#ffb347" },
    { sky: ["#05051a", "#1a1046", "#3b1f6e"], sun: "#9ef0ff", hills: ["#24205e", "#18154a", "#0e0c33"], top: "#54f2ff", side: "#141040", ball: "#ff5ed1", coin: "#9effa8", crate: "#7b61ff" }
  ];
  var TH = THEMES[(seed >>> 0) % THEMES.length];
  var SPEED = U * 46;          // px per second
  var BASE = H * 0.80;         // ground line at level 0
  var STEP = U * 7;            // height of one level
  var R = U * 4.6;             // ball radius
  var BALLX = W * 0.3;         // ball position on screen
  var span = SPEED * TOTAL + W * 2;

  var plats = [], jumps = [], coins = [], crates = [];
  var x = -W, level = 0;
  while (x < span) {
    var len = U * (30 + rand() * 50);
    if (x < W * 0.6) len = W * 1.6; // calm start
    var p = { x0: x, x1: x + len, y: BASE - level * STEP };
    plats.push(p);
    // A crate to hop over on longer platforms.
    if (len > U * 55 && x > 0) {
      var cx = p.x0 + len * (0.4 + rand() * 0.2), cs = U * 7;
      crates.push({ x: cx, y: p.y, s: cs });
      jumps.push({ xa: cx - U * 11, xb: cx + cs + U * 11, ya: p.y, yb: p.y, h: U * 15 });
    } else if (len > U * 40) {
      for (var k = 0; k < 3; k++) coins.push({ x: p.x0 + len * (0.3 + k * 0.18), y: p.y - R - U * 3 });
    }
    var gap = rand() < 0.75 ? U * (14 + rand() * 16) : 0;
    var next = Math.max(-1, Math.min(2, level + (rand() < 0.5 ? -1 : 1) * (rand() < 0.6 ? 1 : 0)));
    if (gap === 0 && next === level) next = level + (level < 2 ? 1 : -1);
    var ny = BASE - next * STEP;
    var j = { xa: p.x1 - U * 6, xb: p.x1 + gap + U * 8, ya: p.y, yb: ny, h: U * 14 + Math.max(0, p.y - ny) };
    jumps.push(j);
    coins.push({ x: lerp(j.xa, j.xb, 0.5), y: lerp(j.ya, j.yb, 0.5) - j.h - R * 0.2 });
    x = p.x1 + gap;
    level = next;
  }
  jumps.sort(function (a, b) { return a.xa - b.xa; });

  function groundAt(wx) {
    for (var i = 0; i < plats.length; i++) if (wx >= plats[i].x0 && wx <= plats[i].x1) return plats[i].y;
    return BASE;
  }
  // Ball height for a world x: on a jump arc, else on the platform.
  function ballY(wx) {
    for (var i = 0; i < jumps.length; i++) {
      var j = jumps[i];
      if (wx >= j.xa && wx <= j.xb) {
        var s = (wx - j.xa) / (j.xb - j.xa);
        return lerp(j.ya, j.yb, s) - 4 * j.h * s * (1 - s) - R;
      }
      if (j.xa > wx) break;
    }
    return groundAt(wx) - R;
  }
  function sinceLanding(wx) {
    var best = 1e9;
    for (var i = 0; i < jumps.length && jumps[i].xa <= wx; i++) if (wx >= jumps[i].xb) best = Math.min(best, wx - jumps[i].xb);
    return best;
  }

  // Rolling hills: sums of sines with seeded phases.
  var hillPh = [];
  for (var h = 0; h < 9; h++) hillPh.push(rand() * Math.PI * 2);
  function hill(layer, wx) {
    var f = 0.0011 * (layer + 1), a = H * (0.05 + 0.025 * layer);
    return Math.sin(wx * f + hillPh[layer * 3]) * a + Math.sin(wx * f * 2.3 + hillPh[layer * 3 + 1]) * a * 0.45 + Math.sin(wx * f * 5.1 + hillPh[layer * 3 + 2]) * a * 0.15;
  }
  var clouds = [];
  for (var c = 0; c < 9; c++) clouds.push({ x: rand() * W * 1.6, y: H * (0.05 + rand() * 0.35), s: U * (5 + rand() * 7) });

  var cv = document.getElementById("game"), g = cv ? cv.getContext("2d") : null;
  var score = 0;
  function draw(t) {
    var cam = SPEED * t, wx = cam + BALLX;
    // Camera follows the ground height smoothly.
    var follow = 0;
    for (var k = -3; k <= 3; k++) follow += groundAt(wx + k * U * 12);
    var camY = (BASE - follow / 7) * 0.6;

    var sky = g.createLinearGradient(0, 0, 0, H);
    sky.addColorStop(0, TH.sky[0]); sky.addColorStop(0.55, TH.sky[1]); sky.addColorStop(1, TH.sky[2]);
    g.fillStyle = sky; g.fillRect(0, 0, W, H);
    // Sun with glow.
    var sx = W * 0.72, sy = H * 0.3 + camY * 0.1;
    var glow = g.createRadialGradient(sx, sy, 0, sx, sy, U * 40);
    glow.addColorStop(0, TH.sun); glow.addColorStop(0.25, TH.sun); glow.addColorStop(1, "rgba(255,255,255,0)");
    g.globalAlpha = 0.9; g.fillStyle = glow; g.fillRect(0, 0, W, H); g.globalAlpha = 1;
    // Clouds drift slowly.
    g.fillStyle = "rgba(255,255,255,0.22)";
    clouds.forEach(function (cl) {
      var px = ((cl.x - cam * 0.08) % (W * 1.6) + W * 1.6) % (W * 1.6) - W * 0.3;
      g.beginPath(); g.ellipse(px, cl.y, cl.s * 2.2, cl.s * 0.8, 0, 0, Math.PI * 2);
      g.ellipse(px + cl.s * 1.4, cl.y - cl.s * 0.4, cl.s * 1.3, cl.s * 0.9, 0, 0, Math.PI * 2); g.fill();
    });
    // Three parallax hill layers.
    for (var l = 0; l < 3; l++) {
      var fac = 0.15 + l * 0.2, baseY = H * (0.58 + l * 0.08) + camY * fac;
      g.fillStyle = TH.hills[l];
      g.beginPath(); g.moveTo(0, H);
      for (var px = 0; px <= W + 24; px += 24) g.lineTo(px, baseY + hill(l, px + cam * fac));
      g.lineTo(W, H); g.closePath(); g.fill();
    }
    g.save(); g.translate(-cam, camY);
    // Platforms with a bright top edge.
    plats.forEach(function (p) {
      if (p.x1 < cam - 50 || p.x0 > cam + W + 50) return;
      g.fillStyle = TH.side; g.fillRect(p.x0, p.y, p.x1 - p.x0, H);
      g.fillStyle = "rgba(255,255,255,0.06)";
      for (var sx2 = p.x0 + U * 4; sx2 < p.x1 - U * 2; sx2 += U * 9) g.fillRect(sx2, p.y + U * 5, U * 4, H);
      g.fillStyle = TH.top; g.beginPath(); g.roundRect(p.x0 - U * 0.6, p.y - U * 0.6, p.x1 - p.x0 + U * 1.2, U * 2.4, U * 1.2); g.fill();
    });
    crates.forEach(function (cr) {
      if (cr.x < cam - 50 || cr.x > cam + W + 50) return;
      g.fillStyle = TH.crate; g.beginPath(); g.roundRect(cr.x, cr.y - cr.s, cr.s, cr.s, U * 1.2); g.fill();
      g.strokeStyle = "rgba(0,0,0,0.25)"; g.lineWidth = U * 0.6; g.strokeRect(cr.x + U * 1.5, cr.y - cr.s + U * 1.5, cr.s - U * 3, cr.s - U * 3);
    });
    // Coins: collected (with a sparkle) once the ball passes them.
    score = 0;
    coins.forEach(function (co) {
      var passed = wx - co.x;
      if (passed >= 0) score++;
      if (co.x < cam - 50 || co.x > cam + W + 50) return;
      if (passed < 0) {
        var wob = Math.abs(Math.cos(t * 5 + co.x * 0.01));
        g.fillStyle = TH.coin; g.beginPath(); g.ellipse(co.x, co.y, U * 2.4 * wob + U * 0.4, U * 2.4, 0, 0, Math.PI * 2); g.fill();
      } else if (passed < SPEED * 0.35) {
        var k2 = passed / (SPEED * 0.35);
        g.strokeStyle = TH.coin; g.globalAlpha = 1 - k2; g.lineWidth = U * 0.7;
        for (var r2 = 0; r2 < 6; r2++) {
          var an = r2 * Math.PI / 3, d1 = U * (2 + k2 * 6), d2 = U * (3 + k2 * 9);
          g.beginPath(); g.moveTo(co.x + Math.cos(an) * d1, co.y + Math.sin(an) * d1); g.lineTo(co.x + Math.cos(an) * d2, co.y + Math.sin(an) * d2); g.stroke();
        }
        g.globalAlpha = 1;
      }
    });
    // Motion trail from earlier ball positions.
    for (var k3 = 8; k3 >= 1; k3--) {
      var tx = wx - k3 * SPEED * 0.025;
      g.globalAlpha = 0.05 * (9 - k3); g.fillStyle = TH.ball;
      g.beginPath(); g.arc(tx, ballY(tx), R * (1 - k3 * 0.06), 0, Math.PI * 2); g.fill();
    }
    g.globalAlpha = 1;
    // The ball: rolls with distance, squashes on landing.
    var by = ballY(wx), land = sinceLanding(wx) / SPEED, sq = land < 0.14 ? 1 - 0.22 * Math.sin(Math.PI * land / 0.14) : 1;
    g.save(); g.translate(wx, by + R * (1 - sq)); g.scale(1 / Math.sqrt(sq), sq);
    g.fillStyle = "rgba(0,0,0,0.18)"; g.beginPath(); g.arc(R * 0.12, R * 0.15, R, 0, Math.PI * 2); g.fill();
    g.fillStyle = TH.ball; g.beginPath(); g.arc(0, 0, R, 0, Math.PI * 2); g.fill();
    g.rotate(wx / R);
    g.strokeStyle = "rgba(0,0,0,0.28)"; g.lineWidth = U * 0.9;
    g.beginPath(); g.arc(0, 0, R * 0.62, -0.9, 0.9); g.stroke();
    g.fillStyle = "rgba(255,255,255,0.55)"; g.beginPath(); g.arc(-R * 0.35, -R * 0.35, R * 0.22, 0, Math.PI * 2); g.fill();
    g.restore();
    g.restore();
    // Score counter like a game HUD.
    g.fillStyle = "rgba(0,0,0,0.35)"; g.beginPath(); g.roundRect(W - U * 26, H * 0.035, U * 21, U * 8, U * 4); g.fill();
    g.fillStyle = TH.coin; g.beginPath(); g.arc(W - U * 21, H * 0.035 + U * 4, U * 2.2, 0, Math.PI * 2); g.fill();
    g.fillStyle = "#ffffff"; g.font = "800 " + (U * 4.6) + "px Inter"; g.textAlign = "left"; g.textBaseline = "middle";
    g.fillText(String(score), W - U * 17, H * 0.035 + U * 4.3);
  }

  // The drawn game, unless the owner's own clip plays in the background.
  var clock = { t: 0 };
  if (g) {
    draw(0);
    tl.to(clock, { t: TOTAL, duration: TOTAL, ease: "none", onUpdate: function () { draw(clock.t); } }, 0);
  }
  // The 3D build style draws its own scene (scene_*.js).
  if (window.Scene3D) {
    var clock3 = { t: 0 };
    window.Scene3D.draw(0);
    tl.to(clock3, { t: TOTAL, duration: TOTAL, ease: "none", onUpdate: function () { window.Scene3D.draw(clock3.t); } }, 0);
  }
  tl.to(".progress-fill", { scaleX: 1, duration: TOTAL, ease: "none" }, 0);

  // Hook card pops in on the opening impact.
  if (document.getElementById("hook")) {
    tl.from("#hook", { y: -U * 30, scale: 0.8, opacity: 0, duration: 0.5, ease: "back.out(1.6)", immediateRender: false }, 0.02);
  }

  // One word (or a short pair) at a time, popping in the middle.
  CAPS.forEach(function (c, i) {
    var el = document.getElementById(c.id);
    var tilt = (i % 2 ? 1 : -1) * (1 + (i * 7 % 3));
    tl.set(el, { autoAlpha: 1 }, c.start);
    tl.fromTo(el, { scale: 0.55, rotation: tilt }, { scale: 1, rotation: 0, duration: 0.14, ease: "back.out(2.6)", immediateRender: false }, c.start);
    tl.set(el, { autoAlpha: 0 }, c.end);
  });

  // The hook card flies up when the first photo arrives during it.
  var hookEl = document.getElementById("hook");
  if (hookEl && POPS.length && POPS[0].start < Number(hookEl.dataset.duration)) {
    tl.to(hookEl, { y: -U * 40, autoAlpha: 0, duration: 0.22, ease: "power2.in" }, Math.max(0.6, POPS[0].start - 0.18));
  }

  // Photos pop up on their key word, wobble slightly, then shrink away.
  POPS.forEach(function (p, i) {
    var el = document.getElementById(p.id);
    var tilt = i % 2 ? 4 : -4;
    tl.set(el, { autoAlpha: 1 }, p.start);
    tl.fromTo(el, { scale: 0.2, rotation: tilt * 3 }, { scale: 1, rotation: tilt * 0.5, duration: 0.28, ease: "back.out(2.2)", immediateRender: false }, p.start);
    tl.to(el, { rotation: -tilt * 0.25, duration: Math.max(0.1, p.end - p.start - 0.45), ease: "sine.inOut" }, p.start + 0.28);
    tl.to(el, { scale: 0.6, autoAlpha: 0, duration: 0.16, ease: "power2.in" }, p.end - 0.16);
  });

  if (TOTAL > 4) {
    tl.set("#end", { autoAlpha: 1 }, TOTAL - 3);
    tl.from("#end", { y: U * 10, opacity: 0, duration: 0.4, ease: "back.out(1.8)", immediateRender: false }, TOTAL - 3);
  }

  window.__timelines["main"] = tl;
  tl.seek(0);
})();
</script>
</body>
</html>`
