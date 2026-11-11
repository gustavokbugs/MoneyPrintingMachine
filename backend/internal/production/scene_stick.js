// scene_stick.js: a 2D stick-figure explainer drawn in SVG.
//
// A hand-drawn stick figure in the lower half explains with poses, and a
// whiteboard in the upper half draws one icon and a short label per beat.
// The mouth moves on every spoken word. window.STICK holds:
//   {total, beats: [{at, pose, icon, label}], words: [start times]}
// Poses and icons come from the fixed lists below (validated in Go).
// Scene2D.build(tl) adds every animation to the page's GSAP timeline, so
// the renderer can seek to any frame.
(function () {
  "use strict";
  const D = window.STICK;
  const svg = document.getElementById("stick");
  if (!D || !svg || !window.gsap) return;
  const NS = "http://www.w3.org/2000/svg";
  const el = (tag, attrs, parent) => {
    const e = document.createElementNS(NS, tag);
    for (const k in attrs) e.setAttribute(k, attrs[k]);
    (parent || svg).appendChild(e);
    return e;
  };
  const INK = "#1d1b19";

  // ---- Paper, hand-drawn wobble filter -------------------------------------
  const defs = el("defs", {});
  const filt = el("filter", { id: "boil", x: "-5%", y: "-5%", width: "110%", height: "110%" }, defs);
  const turb = el("feTurbulence", { type: "fractalNoise", baseFrequency: "0.02", numOctaves: "2", seed: "1", result: "n" }, filt);
  el("feDisplacementMap", { in: "SourceGraphic", in2: "n", scale: "5" }, filt);
  el("rect", { x: 0, y: 0, width: 1080, height: 1920, fill: "#f4efe4" });
  for (let i = 0; i < 26; i++) el("line", { x1: 0, y1: 80 + i * 72, x2: 1080, y2: 80 + i * 72, stroke: "#e6dfcf", "stroke-width": 2 });

  // ---- Whiteboard ------------------------------------------------------------
  const board = el("g", { filter: "url(#boil)" });
  el("rect", { x: 110, y: 170, width: 860, height: 690, rx: 26, fill: "#ffffff", stroke: INK, "stroke-width": 9 }, board);
  el("line", { x1: 540, y1: 860, x2: 470, y2: 930, stroke: INK, "stroke-width": 9, "stroke-linecap": "round" }, board);
  el("line", { x1: 540, y1: 860, x2: 610, y2: 930, stroke: INK, "stroke-width": 9, "stroke-linecap": "round" }, board);

  // ---- Icons: simple line drawings in a 300x300 box ------------------------
  const ICONS = {
    bulb: ["M150 40 C 80 40 55 105 85 150 C 105 180 115 195 115 220 L 185 220 C 185 195 195 180 215 150 C 245 105 220 40 150 40 Z", "M120 245 L 180 245", "M128 268 L 172 268", "M150 220 L 150 150 L 130 130 M150 150 L 170 130"],
    fire: ["M150 260 C 80 260 70 190 105 150 C 110 185 125 190 130 185 C 115 140 140 90 170 60 C 165 110 210 130 210 185 C 210 230 190 260 150 260 Z", "M150 255 C 125 255 120 230 135 210 C 140 225 150 222 150 222 C 148 205 158 195 170 188 C 175 215 185 230 175 245"],
    water: ["M150 40 C 150 40 80 140 80 185 C 80 230 112 260 150 260 C 188 260 220 230 220 185 C 220 140 150 40 150 40 Z", "M118 190 C 118 215 132 228 150 230"],
    house: ["M60 150 L 150 70 L 240 150", "M85 130 L 85 255 L 215 255 L 215 130", "M130 255 L 130 195 L 170 195 L 170 255", "M185 105 L 185 75 L 205 75 L 205 122"],
    money: ["M150 150 m -95 0 a 95 95 0 1 0 190 0 a 95 95 0 1 0 -190 0", "M180 110 C 170 98 160 95 145 95 C 120 95 110 110 115 125 C 120 140 180 145 185 170 C 190 190 170 205 150 205 C 130 205 118 198 110 186", "M150 80 L 150 220"],
    clock: ["M150 150 m -105 0 a 105 105 0 1 0 210 0 a 105 105 0 1 0 -210 0", "M150 150 L 150 85", "M150 150 L 200 175", "M150 52 L 150 64 M248 150 L 236 150 M150 248 L 150 236 M52 150 L 64 150"],
    gear: ["M150 150 m -45 0 a 45 45 0 1 0 90 0 a 45 45 0 1 0 -90 0", "M150 55 L 150 85 M150 215 L 150 245 M55 150 L 85 150 M215 150 L 245 150 M83 83 L 104 104 M196 196 L 217 217 M83 217 L 104 196 M196 104 L 217 83", "M150 150 m -80 0 a 80 80 0 1 0 160 0 a 80 80 0 1 0 -160 0"],
    bolt: ["M175 35 L 85 165 L 145 165 L 120 265 L 215 125 L 155 125 Z"],
    globe: ["M150 150 m -105 0 a 105 105 0 1 0 210 0 a 105 105 0 1 0 -210 0", "M45 150 L 255 150", "M150 45 C 95 95 95 205 150 255 C 205 205 205 95 150 45", "M65 95 L 235 95 M65 205 L 235 205"],
    heart: ["M150 245 C 60 180 40 130 70 95 C 100 60 140 75 150 105 C 160 75 200 60 230 95 C 260 130 240 180 150 245 Z"],
    brain: ["M150 70 C 120 50 75 60 75 100 C 50 110 50 160 75 170 C 70 210 110 235 150 220 C 190 235 230 210 225 170 C 250 160 250 110 225 100 C 225 60 180 50 150 70 Z", "M150 70 L 150 220", "M105 115 C 120 120 125 135 120 150 M195 115 C 180 120 175 135 180 150"],
    check: ["M70 160 L 125 215 L 235 90"],
    cross: ["M80 80 L 220 220 M220 80 L 80 220"],
    question: ["M100 105 C 100 60 200 55 200 110 C 200 150 150 150 150 190", "M150 235 L 150 240"],
    chart: ["M60 250 L 60 60 M60 250 L 250 250", "M80 220 L 130 170 L 170 195 L 235 100", "M205 100 L 235 100 L 235 130"],
    battery: ["M70 100 L 220 100 L 220 200 L 70 200 Z", "M220 130 L 240 130 L 240 170 L 220 170", "M95 125 L 95 175 M125 125 L 125 175 M155 125 L 155 175"],
    phone: ["M105 40 L 195 40 C 205 40 210 45 210 55 L 210 245 C 210 255 205 260 195 260 L 105 260 C 95 260 90 255 90 245 L 90 55 C 90 45 95 40 105 40 Z", "M90 75 L 210 75 M90 220 L 210 220", "M145 240 L 155 240"],
    rocket: ["M150 35 C 195 75 205 140 185 210 L 115 210 C 95 140 105 75 150 35 Z", "M150 115 m -18 0 a 18 18 0 1 0 36 0 a 18 18 0 1 0 -36 0", "M115 170 L 80 225 L 115 215 M185 170 L 220 225 L 185 215", "M135 215 L 140 260 M165 215 L 160 260"],
    tree: ["M150 260 L 150 175", "M150 40 L 85 130 L 115 130 L 70 190 L 230 190 L 185 130 L 215 130 Z"],
    sun: ["M150 150 m -55 0 a 55 55 0 1 0 110 0 a 55 55 0 1 0 -110 0", "M150 40 L 150 70 M150 230 L 150 260 M40 150 L 70 150 M230 150 L 260 150 M72 72 L 93 93 M207 207 L 228 228 M72 228 L 93 207 M207 93 L 228 72"],
    wheel: ["M150 150 m -100 0 a 100 100 0 1 0 200 0 a 100 100 0 1 0 -200 0", "M150 150 m -18 0 a 18 18 0 1 0 36 0 a 18 18 0 1 0 -36 0", "M150 50 L 150 250 M50 150 L 250 150 M80 80 L 220 220 M220 80 L 80 220"],
    magnet: ["M80 70 L 80 160 C 80 260 220 260 220 160 L 220 70 L 175 70 L 175 160 C 175 200 125 200 125 160 L 125 70 Z", "M80 105 L 125 105 M175 105 L 220 105"],
    virus: ["M150 150 m -60 0 a 60 60 0 1 0 120 0 a 60 60 0 1 0 -120 0", "M150 90 L 150 55 M150 210 L 150 245 M90 150 L 55 150 M210 150 L 245 150 M108 108 L 83 83 M192 192 L 217 217 M108 192 L 83 217 M192 108 L 217 83", "M130 135 L 132 137 M168 160 L 170 162 M140 170 L 142 172"],
    computer: ["M55 60 L 245 60 L 245 190 L 55 190 Z", "M120 190 L 110 245 L 190 245 L 180 190", "M85 225 L 215 225"],
    microscope: ["M120 60 L 160 50 L 185 140 L 145 150 Z", "M150 150 C 200 165 215 205 200 235", "M90 255 L 230 255", "M165 245 L 165 210 L 120 210"],
  };
  const iconLayer = el("g", { filter: "url(#boil)" });
  const label = el("text", { x: 540, y: 800, "text-anchor": "middle", "font-family": "Inter", "font-weight": 800, "font-size": 78, fill: INK, opacity: 0 });

  // ---- The stick figure: joints are nested groups --------------------------
  const STROKE = { stroke: INK, "stroke-width": 15, "stroke-linecap": "round", "stroke-linejoin": "round", fill: "none" };
  const fig = el("g", { filter: "url(#boil)" });
  const root = el("g", {}, fig); // hip at (0,0)
  const limb = (parent, len) => { const g = el("g", {}, parent); el("line", Object.assign({ x1: 0, y1: 0, x2: 0, y2: len }, STROKE), g); return g; };
  const at = (parent, y) => el("g", { transform: "translate(0," + y + ")" }, parent);
  // Legs from the hip.
  const thighL = limb(root, 150), shinL = limb(at(thighL, 150), 150);
  const thighR = limb(root, 150), shinR = limb(at(thighR, 150), 150);
  // Torso up from the hip.
  const torso = el("g", {}, root);
  el("line", Object.assign({ x1: 0, y1: 0, x2: 0, y2: -230 }, STROKE), torso);
  const neck = at(torso, -230);
  const armL = limb(neck, 120), foreL = limb(at(armL, 120), 115);
  const armR = limb(neck, 120), foreR = limb(at(armR, 120), 115);
  const headG = el("g", {}, neck);
  el("circle", Object.assign({ cx: 0, cy: -75, r: 70 }, STROKE, { fill: "#f4efe4" }), headG);
  el("circle", { cx: -24, cy: -88, r: 8, fill: INK }, headG);
  el("circle", { cx: 24, cy: -88, r: 8, fill: INK }, headG);
  const mouth = el("ellipse", { cx: 0, cy: -50, rx: 20, ry: 4, fill: INK }, headG);
  root.setAttribute("transform", "translate(300,1600) scale(1.12)");

  // Poses: joint angles in degrees (0 = straight down; positive = clockwise).
  const POSES = {
    idle:      { tl: 8, sl: -4, tr: -8, sr: 4, al: 22, fl: -8, ar: -22, fr: 8, lean: 0, head: 0 },
    point:     { tl: 8, sl: -4, tr: -8, sr: 4, al: 25, fl: -10, ar: -125, fr: -20, lean: -4, head: -8 },
    explain:   { tl: 10, sl: -4, tr: -10, sr: 4, al: 55, fl: 70, ar: -55, fr: -70, lean: 0, head: 0 },
    think:     { tl: 8, sl: -4, tr: -8, sr: 4, al: 25, fl: -5, ar: -35, fr: -150, lean: 3, head: 10 },
    shrug:     { tl: 10, sl: -4, tr: -10, sr: 4, al: 70, fl: 95, ar: -70, fr: -95, lean: 0, head: -10 },
    surprised: { tl: 14, sl: -6, tr: -14, sr: 6, al: 150, fl: 20, ar: -150, fr: -20, lean: -6, head: 0 },
    facepalm:  { tl: 8, sl: -4, tr: -8, sr: 4, al: 25, fl: -5, ar: -60, fr: -150, lean: 8, head: 18 },
    celebrate: { tl: 18, sl: -10, tr: -18, sr: 10, al: 160, fl: 10, ar: -160, fr: -10, lean: 0, head: -5 },
    sad:       { tl: 5, sl: -2, tr: -5, sr: 2, al: 8, fl: 0, ar: -8, fr: 0, lean: 12, head: 25 },
  };
  const parts = { tl: thighL, sl: shinL, tr: thighR, sr: shinR, al: armL, fl: foreL, ar: armR, fr: foreR, lean: torso, head: headG };

  function setPose(tl, pose, t, dur) {
    const p = POSES[pose] || POSES.idle;
    for (const k in parts) tl.to(parts[k], { rotation: p[k], svgOrigin: "0 0", duration: dur, ease: "back.out(1.6)" }, t);
  }

  window.Scene2D = {
    build(tl) {
      const total = D.total;
      // Hand-drawn boil: the wobble pattern changes 8 times a second.
      for (let t = 0, s = 1; t < total; t += 0.125, s++) tl.set(turb, { attr: { seed: s % 5 } }, t);
      setPose(tl, "idle", 0, 0.01);
      // Breathing: a gentle bob of the whole figure.
      tl.to(root, { y: "-=6", duration: 1.2, ease: "sine.inOut", repeat: Math.max(1, Math.floor(total / 1.2)), yoyo: true }, 0);
      // Talking: the mouth opens on every word.
      (D.words || []).forEach((w) => {
        tl.to(mouth, { attr: { ry: 16 }, duration: 0.07, ease: "power1.out" }, w);
        tl.to(mouth, { attr: { ry: 4 }, duration: 0.12, ease: "power1.in" }, w + 0.09);
      });
      // Beats: a pose, an icon drawn on the board, and a label.
      let prevIcon = null;
      (D.beats || []).forEach((b) => {
        setPose(tl, b.pose, b.at, 0.4);
        if (b.pose === "celebrate") tl.to(root, { y: "-=60", duration: 0.25, ease: "power2.out", yoyo: true, repeat: 1 }, b.at);
        if (prevIcon) tl.to(prevIcon, { opacity: 0, duration: 0.2 }, b.at - 0.05);
        const paths = ICONS[b.icon];
        if (paths) {
          // Icons are drawn in a 300 box and shown 1.65x, centred above the label.
          const g = el("g", { transform: "translate(292,195) scale(1.65)", opacity: 0 }, iconLayer);
          paths.forEach((d) => {
            const p = el("path", Object.assign({ d }, STROKE, { "stroke-width": 12 }), g);
            const len = p.getTotalLength();
            p.setAttribute("stroke-dasharray", len);
            p.setAttribute("stroke-dashoffset", len);
            tl.to(p, { attr: { "stroke-dashoffset": 0 }, duration: 0.7, ease: "power1.inOut" }, b.at + 0.05);
          });
          tl.set(g, { opacity: 1 }, b.at + 0.05);
          prevIcon = g;
        }
        tl.set(label, { textContent: b.label || "" }, b.at);
        tl.fromTo(label, { opacity: 0, y: 20 }, { opacity: 1, y: 0, duration: 0.3, ease: "power2.out", immediateRender: false }, b.at + 0.3);
      });
    },
  };
})();
