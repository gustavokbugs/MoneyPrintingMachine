// build3d.js draws the "3D build" style: a low-poly diorama on which one
// piece per narration step appears (dropped, raised, grown, slid in), with
// optional problem effects (red glow, smoke, leak, shake, topple). Pieces
// accumulate, so the video shows a contraption being built step by step.
//
// It is a pure function of time: Build3D.draw(t) renders the frame at t
// seconds, so the frame-by-frame renderer can seek anywhere. Input comes
// from window.BUILD = {total, seed, steps: [{piece, action, problem, at,
// problemAt}]}; every value was validated against fixed lists in Go.
(function () {
  "use strict";
  const B = window.BUILD;
  const canvas = document.getElementById("gl3d");
  if (!B || !canvas || !window.THREE) return;
  const W = canvas.width, H = canvas.height;

  let seed = (B.seed >>> 0) || 7;
  function rand() {
    seed = (seed + 0x6D2B79F5) | 0;
    let t = seed;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  }
  const clamp = (x, a, b) => Math.max(a, Math.min(b, x));
  const easeOut = (x) => 1 - Math.pow(1 - clamp(x, 0, 1), 3);
  const backOut = (x) => { x = clamp(x, 0, 1); const c = 1.7; return 1 + (c + 1) * Math.pow(x - 1, 3) + c * Math.pow(x - 1, 2); };
  const bounce = (x) => {
    x = clamp(x, 0, 1); const n = 7.5625, d = 2.75;
    if (x < 1 / d) return n * x * x;
    if (x < 2 / d) return n * (x -= 1.5 / d) * x + 0.75;
    if (x < 2.5 / d) return n * (x -= 2.25 / d) * x + 0.9375;
    return n * (x -= 2.625 / d) * x + 0.984375;
  };

  const renderer = new THREE.WebGLRenderer({ canvas, antialias: true, preserveDrawingBuffer: true });
  renderer.setSize(W, H, false);
  renderer.shadowMap.enabled = true;
  renderer.shadowMap.type = THREE.PCFSoftShadowMap;
  const scene = new THREE.Scene();
  const SKY = "#9fcbe8";
  scene.background = new THREE.Color(SKY);
  scene.fog = new THREE.Fog(SKY, 45, 95);
  const camera = new THREE.PerspectiveCamera(40, W / H, 0.1, 300);
  scene.add(new THREE.HemisphereLight("#e6f4ff", "#56663a", 1.0));
  const sun = new THREE.DirectionalLight("#fff3dc", 2.3);
  sun.position.set(-16, 30, 12);
  sun.castShadow = true;
  sun.shadow.mapSize.set(2048, 2048);
  Object.assign(sun.shadow.camera, { left: -24, right: 24, top: 24, bottom: -24, near: 1, far: 90 });
  scene.add(sun);

  // ---- Materials -----------------------------------------------------------
  const mats = {};
  function mat(color, opts) {
    const key = color + JSON.stringify(opts || {});
    if (!mats[key]) mats[key] = new THREE.MeshStandardMaterial(Object.assign({ color, roughness: 0.85, flatShading: true }, opts || {}));
    return mats[key];
  }
  function brickTexture(base, brick) {
    const c = document.createElement("canvas"); c.width = c.height = 128;
    const x = c.getContext("2d");
    x.fillStyle = base; x.fillRect(0, 0, 128, 128);
    for (let r = 0; r < 8; r++) for (let k = 0; k < 4; k++) {
      const s = 0.85 + rand() * 0.3; const col = new THREE.Color(brick).multiplyScalar(s);
      x.fillStyle = "#" + col.getHexString();
      x.fillRect((k * 32 + (r % 2) * 16) % 128 + 1, r * 16 + 1, 30, 14);
    }
    const t = new THREE.CanvasTexture(c); t.wrapS = t.wrapT = THREE.RepeatWrapping; return t;
  }
  const stoneTex = brickTexture("#3b3833", "#8a8072"), brickTex = brickTexture("#5a4a40", "#b0533a");

  // ---- Ground: rolling grass that fades into the fog --------------------
  const field = new THREE.PlaneGeometry(140, 140, 110, 110);
  field.rotateX(-Math.PI / 2);
  const fp = field.attributes.position, cols = [];
  const g1 = new THREE.Color("#7a9a3a"), g2 = new THREE.Color("#5d7c28");
  for (let i = 0; i < fp.count; i++) {
    const x = fp.getX(i), z = fp.getZ(i), d = Math.hypot(x, z);
    // Flat where the build stands, gentle hills further out.
    const hills = d < 14 ? 0 : (d - 14) * 0.04 * (Math.sin(x * 0.12) + Math.cos(z * 0.1) + 1);
    fp.setY(i, hills + (rand() - 0.5) * 0.12);
    const c = g1.clone().lerp(g2, 0.5 + 0.5 * Math.sin(x * 0.3) * Math.cos(z * 0.27) * rand());
    cols.push(c.r, c.g, c.b);
  }
  field.setAttribute("color", new THREE.Float32BufferAttribute(cols, 3));
  field.computeVertexNormals();
  const ground = new THREE.Mesh(field, new THREE.MeshStandardMaterial({ vertexColors: true, roughness: 1, flatShading: true }));
  ground.receiveShadow = true; scene.add(ground);

  // ---- Piece builders (each about 2-3 units tall, standing on y = 0) -------
  function box(w, h, d, m, x, y, z) { const o = new THREE.Mesh(new THREE.BoxGeometry(w, h, d), m); o.position.set(x || 0, y === undefined ? h / 2 : y, z || 0); return o; }
  function cyl(rt, rb, h, m, seg, x, y, z) { const o = new THREE.Mesh(new THREE.CylinderGeometry(rt, rb, h, seg || 12), m); o.position.set(x || 0, y === undefined ? h / 2 : y, z || 0); return o; }
  function rockMesh(size, color) {
    const g = new THREE.IcosahedronGeometry(size, 1); const p = g.attributes.position;
    for (let k = 0; k < p.count; k++) p.setXYZ(k, p.getX(k) * (0.85 + rand() * 0.35), p.getY(k) * (0.7 + rand() * 0.3), p.getZ(k) * (0.85 + rand() * 0.3));
    g.computeVertexNormals();
    const o = new THREE.Mesh(g, mat(color || "#857b70")); o.position.y = size * 0.6; return o;
  }
  function flame(h, x, z) {
    const g = new THREE.Group();
    g.add(new THREE.Mesh(new THREE.ConeGeometry(0.5 * h, 1.6 * h, 6), mat("#ff7a1a", { emissive: "#ff5a00", emissiveIntensity: 1.2 })));
    const inner = new THREE.Mesh(new THREE.ConeGeometry(0.28 * h, 1.0 * h, 6), mat("#ffd84d", { emissive: "#ffcc33", emissiveIntensity: 1.5 }));
    inner.position.y = -0.15 * h; g.add(inner);
    g.position.set(x || 0, 0.8 * h, z || 0); g.userData.flame = true; return g;
  }
  const builders = {
    rock() { const g = new THREE.Group(); g.add(rockMesh(1.2)); return g; },
    rocks() { const g = new THREE.Group(); for (let i = 0; i < 4; i++) { const r = rockMesh(0.7 + rand() * 0.4); r.position.x = (i - 1.5) * 1.1; g.add(r); } return g; },
    log() { const g = new THREE.Group(); const l = cyl(0.45, 0.45, 3.6, mat("#7a4f2a"), 10); l.rotation.z = Math.PI / 2; l.position.y = 0.45; g.add(l); return g; },
    sticks() { const g = new THREE.Group(); for (let i = 0; i < 6; i++) { const s = cyl(0.08, 0.1, 3.4, mat("#8a6236"), 6); const a = (i / 6) * Math.PI * 2; s.position.set(Math.cos(a) * 0.6, 1.5, Math.sin(a) * 0.6); s.rotation.set(Math.sin(a) * 0.35, 0, -Math.cos(a) * 0.35); g.add(s); } return g; },
    wall() { const g = new THREE.Group(); const m = new THREE.MeshStandardMaterial({ map: stoneTex, roughness: 0.9 }); g.add(box(4.5, 2.4, 0.9, m)); return g; },
    bricks() { const g = new THREE.Group(); for (let r = 0; r < 4; r++) for (let k = 0; k < 3; k++) g.add(box(0.95, 0.42, 0.5, mat(rand() < 0.5 ? "#b5532f" : "#a14a2a"), (k - 1) + (r % 2) * 0.48 - 0.24, 0.22 + r * 0.45)); return g; },
    clay() { const g = new THREE.Group(); for (let i = 0; i < 4; i++) { const b = new THREE.Mesh(new THREE.DodecahedronGeometry(0.55 + rand() * 0.25, 0), mat("#c06a2c")); b.position.set((rand() - 0.5) * 1.6, 0.45, (rand() - 0.5) * 1.6); g.add(b); } return g; },
    roof() { const g = new THREE.Group(); const r = cyl(2.4, 2.4, 4.6, mat("#b23a2a"), 3); r.rotation.z = Math.PI / 2; r.position.y = 1.2; g.add(r); return g; },
    house() { const g = new THREE.Group(); g.add(box(3.6, 2.4, 3, new THREE.MeshStandardMaterial({ map: brickTex, roughness: 0.9 }))); const r = cyl(2.1, 2.1, 4, mat("#a8392a"), 3); r.rotation.z = Math.PI / 2; r.position.y = 3.25; g.add(r); g.add(box(0.8, 1.4, 0.1, mat("#5a3a20"), 0, 0.7, 1.52)); g.add(box(0.7, 0.6, 0.1, mat("#cfe9ff", { emissive: "#334455" }), 1.1, 1.5, 1.52)); return g; },
    tent() { const g = new THREE.Group(); g.add(cyl(0.01, 2.2, 2.8, mat("#d9c79a"), 4)); return g; },
    fire() { const g = new THREE.Group(); for (let i = 0; i < 3; i++) { const l = cyl(0.18, 0.18, 1.8, mat("#6b4024"), 6); l.rotation.set(0, (i * Math.PI) / 3, Math.PI / 2.4); l.position.y = 0.3; g.add(l); } g.add(flame(1.1)); return g; },
    water() { const g = new THREE.Group(); const w = new THREE.Mesh(new THREE.CylinderGeometry(2.2, 2.2, 0.25, 32), mat("#4fb6cf", { roughness: 0.2, transparent: true, opacity: 0.9 })); w.position.y = 0.13; w.userData.wave = true; g.add(w); const rim = new THREE.Mesh(new THREE.TorusGeometry(2.25, 0.25, 6, 32), mat("#8a8072")); rim.rotation.x = Math.PI / 2; g.add(rim); return g; },
    pipe() { const g = new THREE.Group(); const p = cyl(0.35, 0.35, 4.4, mat("#9aa0a6", { metalness: 0.5, roughness: 0.4 }), 14); p.rotation.z = Math.PI / 2; p.position.y = 0.6; g.add(p); for (const x of [-2.1, 2.1]) { const f = cyl(0.5, 0.5, 0.2, mat("#7d8389"), 14); f.rotation.z = Math.PI / 2; f.position.set(x, 0.6, 0); g.add(f); } return g; },
    pump() { const g = new THREE.Group(); g.add(box(1.4, 1.4, 1.4, mat("#3a6fb0"))); g.add(cyl(0.3, 0.3, 1.6, mat("#9aa0a6"), 10, 0, 2.2)); const h = box(1.8, 0.15, 0.15, mat("#333")); h.position.set(0.6, 3, 0); h.userData.pumpHandle = true; g.add(h); return g; },
    valve() { const g = new THREE.Group(); const p = cyl(0.3, 0.3, 2.4, mat("#9aa0a6"), 12); p.rotation.z = Math.PI / 2; p.position.y = 0.5; g.add(p); g.add(cyl(0.12, 0.12, 0.9, mat("#777"), 8, 0, 1.0)); const w = new THREE.Mesh(new THREE.TorusGeometry(0.55, 0.1, 6, 18), mat("#cc2a2a")); w.position.y = 1.5; w.rotation.x = Math.PI / 2; w.userData.spin = true; g.add(w); return g; },
    wheel() { const g = new THREE.Group(); const w = new THREE.Group(); w.add(new THREE.Mesh(new THREE.TorusGeometry(1.6, 0.14, 6, 24), mat("#7a4f2a"))); for (let i = 0; i < 8; i++) { const p = box(0.15, 1.6, 0.7, mat("#8a6236"), 0, 0); p.position.set(Math.cos((i / 8) * Math.PI * 2) * 1.3, Math.sin((i / 8) * Math.PI * 2) * 1.3, 0); p.rotation.z = (i / 8) * Math.PI * 2; w.add(p); } w.position.y = 1.9; w.userData.spinZ = true; g.add(w); g.add(box(0.3, 2, 0.3, mat("#5a3a20"), 0, 1, -0.6)); return g; },
    gear() { const g = new THREE.Group(); const w = new THREE.Group(); w.add(cyl(1.2, 1.2, 0.4, mat("#b8a040", { metalness: 0.6, roughness: 0.35 }), 20, 0, 0)); for (let i = 0; i < 12; i++) { const tth = box(0.35, 0.4, 0.4, mat("#b8a040", { metalness: 0.6, roughness: 0.35 })); const a = (i / 12) * Math.PI * 2; tth.position.set(Math.cos(a) * 1.35, 0, Math.sin(a) * 1.35); tth.rotation.y = -a; w.add(tth); } w.rotation.x = Math.PI / 2; w.position.y = 1.7; w.userData.spinZ = true; g.add(w); return g; },
    generator() { const g = new THREE.Group(); g.add(box(2.2, 1.6, 1.4, mat("#2f6db5"))); const c = cyl(0.55, 0.55, 1.0, mat("#9aa0a6", { metalness: 0.5 }), 14, 1.5, 0.8); c.rotation.z = Math.PI / 2; g.add(c); g.add(box(0.5, 0.3, 0.1, mat("#ffd84d", { emissive: "#ffcc33" }), 0, 1.0, 0.72)); return g; },
    wire() { const g = new THREE.Group(); const pts = []; for (let i = 0; i <= 20; i++) pts.push(new THREE.Vector3((i / 20 - 0.5) * 4, 0.2 + Math.sin(i / 20 * Math.PI) * 1.2, Math.sin(i * 0.9) * 0.3)); g.add(new THREE.Mesh(new THREE.TubeGeometry(new THREE.CatmullRomCurve3(pts), 40, 0.08, 6), mat("#e0b030"))); return g; },
    battery() { const g = new THREE.Group(); g.add(cyl(0.6, 0.6, 1.8, mat("#2a2a2a"), 16)); g.add(cyl(0.61, 0.61, 0.7, mat("#3cb043"), 16, 0, 1.3)); g.add(cyl(0.2, 0.2, 0.2, mat("#ccc", { metalness: 0.8 }), 10, 0, 1.9)); return g; },
    bulb() { const g = new THREE.Group(); g.add(cyl(0.35, 0.3, 0.6, mat("#aaa", { metalness: 0.7 }), 12)); const s = new THREE.Mesh(new THREE.SphereGeometry(0.75, 16, 12), mat("#fff6c0", { emissive: "#ffd24d", emissiveIntensity: 0.2, transparent: true, opacity: 0.9 })); s.position.y = 1.2; s.userData.glow = true; g.add(s); return g; },
    lamp() { const g = new THREE.Group(); g.add(cyl(0.12, 0.15, 3.6, mat("#333"), 8)); const s = new THREE.Mesh(new THREE.SphereGeometry(0.45, 12, 10), mat("#fff6c0", { emissive: "#ffd24d", emissiveIntensity: 0.3 })); s.position.y = 3.8; s.userData.glow = true; g.add(s); return g; },
    tower() { const g = new THREE.Group(); for (const [x, z] of [[-0.7, -0.7], [0.7, -0.7], [-0.7, 0.7], [0.7, 0.7]]) g.add(box(0.14, 5, 0.14, mat("#8a8f96"), x, 2.5, z)); for (let y = 0.8; y < 5; y += 1.2) g.add(box(1.6, 0.1, 1.6, mat("#8a8f96"), 0, y)); return g; },
    tank() { const g = new THREE.Group(); g.add(cyl(1.4, 1.4, 2.6, mat("#c8ccd0", { metalness: 0.5, roughness: 0.35 }), 20)); g.add(cyl(1.45, 1.45, 0.15, mat("#888"), 20, 0, 2.6)); return g; },
    tree() { const g = new THREE.Group(); g.add(cyl(0.18, 0.25, 1.4, mat("#6b4024"), 6)); const c = new THREE.Mesh(new THREE.ConeGeometry(1.1, 2.4, 7), mat("#3f6e2a")); c.position.y = 2.4; g.add(c); return g; },
    cloud() { const g = new THREE.Group(); for (let i = 0; i < 5; i++) { const s = new THREE.Mesh(new THREE.IcosahedronGeometry(0.7 + rand() * 0.4, 1), mat("#ffffff", { roughness: 1 })); s.position.set((i - 2) * 0.7, 4 + rand() * 0.4, rand() * 0.6); g.add(s); } return g; },
    ice() { const g = new THREE.Group(); g.add(box(1.8, 1.8, 1.8, mat("#bfe8ff", { transparent: true, opacity: 0.75, roughness: 0.1 }))); return g; },
    chimney() { const g = new THREE.Group(); g.add(box(1, 3.6, 1, new THREE.MeshStandardMaterial({ map: brickTex, roughness: 0.9 }))); g.userData.chimney = true; return g; },
    fan() { const g = new THREE.Group(); g.add(cyl(0.1, 0.15, 2.4, mat("#555"), 8)); const b = new THREE.Group(); for (let i = 0; i < 3; i++) { const p = box(0.35, 1.3, 0.06, mat("#d8d8d8"), 0, 0); p.position.y = 0.6; const h = new THREE.Group(); h.add(p); h.rotation.z = (i / 3) * Math.PI * 2; b.add(h); } b.position.set(0, 2.5, 0.2); b.userData.spinZ = true; g.add(b); return g; },
    boat() { const g = new THREE.Group(); const h = cyl(0.8, 0.5, 3.2, mat("#8a5a30"), 6); h.rotation.z = Math.PI / 2; h.scale.set(1, 1, 0.6); h.position.y = 0.4; g.add(h); g.add(cyl(0.06, 0.06, 2.6, mat("#5a3a20"), 6, 0, 2)); const s = box(0.05, 1.6, 1.3, mat("#f2efe6"), 0.1, 2.2); g.add(s); return g; },
    bridge() { const g = new THREE.Group(); for (let i = 0; i < 8; i++) g.add(box(0.45, 0.15, 2.2, mat(i % 2 ? "#8a6236" : "#7a5530"), (i - 3.5) * 0.5, 0.9)); for (const z of [-1, 1]) g.add(box(4, 0.08, 0.08, mat("#5a3a20"), 0, 1.6, z)); for (const x of [-1.9, 1.9]) for (const z of [-1, 1]) g.add(box(0.15, 1.7, 0.15, mat("#5a3a20"), x, 0.85, z)); return g; },
    cart() { const g = new THREE.Group(); g.add(box(2.4, 0.9, 1.4, mat("#9a6a3a"), 0, 0.9)); for (const x of [-0.8, 0.8]) for (const z of [-0.75, 0.75]) { const w = cyl(0.4, 0.4, 0.15, mat("#333"), 12); w.rotation.x = Math.PI / 2; w.position.set(x, 0.4, z); g.add(w); } return g; },
    magnet() { const g = new THREE.Group(); const u = new THREE.Mesh(new THREE.TorusGeometry(0.9, 0.3, 8, 16, Math.PI), mat("#cc2a2a")); u.rotation.z = Math.PI; u.position.y = 1.5; g.add(u); for (const x of [-0.9, 0.9]) g.add(box(0.6, 0.5, 0.6, mat("#c8ccd0"), x, 1.25)); return g; },
    plant() { const g = new THREE.Group(); g.add(cyl(0.6, 0.45, 0.7, mat("#a0522d"), 10)); g.add(cyl(0.05, 0.05, 1.2, mat("#3f8a2a"), 6, 0, 1.2)); for (const s of [-1, 1]) { const l = new THREE.Mesh(new THREE.SphereGeometry(0.35, 8, 6), mat("#4fa03a")); l.scale.set(1, 0.4, 0.6); l.position.set(s * 0.35, 1.5, 0); g.add(l); } return g; },
    computer() { const g = new THREE.Group(); g.add(box(2.2, 1.5, 0.15, mat("#222"), 0, 1.6)); g.add(box(2.0, 1.3, 0.05, mat("#3a8fd8", { emissive: "#1a5fa8", emissiveIntensity: 0.6 }), 0, 1.6, 0.1)); g.add(box(0.3, 0.8, 0.3, mat("#444"), 0, 0.4)); return g; },
    crate() { const g = new THREE.Group(); g.add(box(1.6, 1.6, 1.6, mat("#a87a44"))); return g; },
  };
  function build(piece) {
    const g = (builders[piece] || builders.crate)();
    g.traverse((o) => { if (o.isMesh) { o.castShadow = true; o.receiveShadow = true; } });
    return g;
  }

  // A few decorative trees around the edge.
  for (let i = 0; i < 16; i++) {
    const a = rand() * Math.PI * 2, r = 17 + rand() * 9;
    const tree = build("tree"); tree.position.set(Math.cos(a) * r, 0, Math.sin(a) * r); tree.scale.setScalar(0.7 + rand() * 0.5);
    scene.add(tree);
  }

  // Pieces are modelled 2-3 units tall and shown larger.
  const PIECE_SCALE = 1.5;

  // ---- Steps: one piece each, placed on a spiral around the centre ---------
  const steps = (B.steps || []).map((s, i) => {
    const a = i * 2.25, r = i === 0 ? 0 : Math.min(9, 2.9 * Math.sqrt(i));
    const g = build(s.piece);
    g.position.set(Math.cos(a) * r, 0, Math.sin(a) * r);
    g.rotation.y = -a + Math.PI / 2;
    g.scale.setScalar(PIECE_SCALE);
    g.visible = false;
    scene.add(g);
    const tint = [];
    g.traverse((o) => { if (o.isMesh) { o.material = o.material.clone(); tint.push(o); } });
    return Object.assign({}, s, { g, home: g.position.clone(), rotY: g.rotation.y, meshes: tint });
  });

  // Particles shared by smoke and leaks.
  const puffs = new THREE.InstancedMesh(new THREE.IcosahedronGeometry(0.35, 1), mat("#8a8a8a", { roughness: 1, transparent: true, opacity: 0.7 }), 200);
  const drops = new THREE.InstancedMesh(new THREE.SphereGeometry(0.1, 6, 5), mat("#7fd0ff", { roughness: 0.2 }), 200);
  scene.add(puffs, drops);
  const m4 = new THREE.Matrix4(), q = new THREE.Quaternion(), v = new THREE.Vector3(), sc = new THREE.Vector3();
  const red = new THREE.Color("#ff2020"), black = new THREE.Color("#000000");

  function draw(t) {
    let shown = 0, focus = new THREE.Vector3();
    let np = 0, nd = 0;
    steps.forEach((s, i) => {
      const k = (t - s.at) / 0.7;
      s.g.visible = t >= s.at;
      if (!s.g.visible) return;
      shown++;
      if (i === 0 || t >= s.at) focus.lerp(s.home, i === 0 ? 1 : easeOut((t - s.at) / 0.9));
      const p = s.g.position.copy(s.home);
      s.g.scale.setScalar(PIECE_SCALE);
      s.g.rotation.set(0, s.rotY, 0);
      switch (s.action) {
        case "drop": p.y = 10 * (1 - bounce(k)); break;
        case "rise": p.y = -3.5 * (1 - easeOut(k)); break;
        case "slide": p.x += -9 * (1 - easeOut(k)); break;
        case "spin": s.g.scale.setScalar(PIECE_SCALE * Math.max(0.001, backOut(k))); s.g.rotation.y = s.rotY + (t - s.at) * 1.2; break;
        default: s.g.scale.setScalar(PIECE_SCALE * Math.max(0.001, backOut(k))); // grow and glow
      }
      // Problems.
      const pk = s.problem && s.problem !== "none" ? (t - s.problemAt) : -1;
      const active = pk >= 0 && pk < 2.2;
      s.meshes.forEach((m) => {
        if (!m.material.emissive) return;
        const base = m.userData.baseEmissive || (m.userData.baseEmissive = m.material.emissive.clone());
        const glowOn = m.userData.glow && (s.action === "glow" || (t - s.at) > 0.6);
        m.material.emissive.copy(base);
        if (m.userData.glow) m.material.emissiveIntensity = glowOn ? 1.6 + 0.2 * Math.sin(t * 9) : 0.2;
        if (s.problem === "red" && active) m.material.emissive.copy(black).lerp(red, 0.5 + 0.5 * Math.sin(pk * 12));
      });
      if (s.problem === "shake" && active) s.g.rotation.z = Math.sin(pk * 40) * 0.08 * (1 - pk / 2.2);
      if (s.problem === "topple" && pk >= 0) { const f = easeOut(pk / 0.8); s.g.rotation.z = f * Math.PI / 2.1; p.y += f * 0.4; }
      if (s.problem === "smoke" && pk >= 0 && pk < 3) {
        for (let j = 0; j < 14; j++) { const life = (pk * 0.9 + j / 14) % 1; v.set(s.home.x + Math.sin(j * 7.1) * 0.6 * life, 2.5 + life * 5, s.home.z + Math.cos(j * 3.3) * 0.6 * life); m4.compose(v, q, sc.setScalar(0.6 + life * 1.8)); puffs.setMatrixAt(np++, m4); }
      }
      if (s.problem === "leak" && pk >= 0 && pk < 3) {
        for (let j = 0; j < 20; j++) { const life = (pk * 1.6 + j / 20) % 1; v.set(s.home.x + 0.9 + life * 0.8, 1.4 - life * life * 1.5, s.home.z + Math.sin(j * 5.7) * 0.5); m4.compose(v, q, sc.setScalar(1)); drops.setMatrixAt(nd++, m4); }
      }
      // Living details: spinning wheels, flickering flames, chimney smoke.
      s.g.traverse((o) => {
        if (o.userData.spinZ) o.rotation.z = (t - s.at) * 2.2;
        if (o.userData.spin) o.rotation.z = (t - s.at) * 1.5;
        if (o.userData.flame) o.scale.set(1, 0.85 + 0.25 * Math.abs(Math.sin(t * 9 + i)), 1);
        if (o.userData.wave) o.position.y = 0.13 + Math.sin(t * 3) * 0.03;
        if (o.userData.pumpHandle) o.rotation.z = Math.sin(t * 4) * 0.4;
      });
    });
    puffs.count = np; puffs.instanceMatrix.needsUpdate = true;
    drops.count = nd; drops.instanceMatrix.needsUpdate = true;
    // Camera: follows the newest piece, backing off as the build grows.
    const dist = 11 + Math.min(shown, 12) * 0.75;
    const ang = 0.6 + t * 0.06;
    camera.position.set(focus.x * 0.6 + Math.cos(ang) * dist, dist * 0.85, focus.z * 0.6 + Math.sin(ang) * dist);
    camera.lookAt(focus.x * 0.6, 0.8, focus.z * 0.6);
    renderer.render(scene, camera);
  }

  window.Build3D = { draw };
})();
