// scene_dam.js: "river to hydropower", a hand-built 3D scene in stages.
//
// Stages (window.BUILD.stages holds the start time of each, in seconds):
//  0 river     a river flows through a grassy valley
//  1 stones    stones drop across the river
//  2 leak      water jets through red-glowing gaps
//  3 clay      clay plugs the gaps
//  4 lake      a lake spreads behind the wall
//  5 overflow  water pours over the top
//  6 washout   the wall tumbles downstream, the lake drains
//  7 dam       a thick stone dam rises layer by layer, the lake refills
//  8 spillway  a side channel carries the extra water down
//  9 wheel     a water wheel at the channel spins
// 10 power     a generator lights up a house
// window.BUILD.outro is when the camera pulls back over everything.
//
// Scene3D.draw(t) is a pure function of time, so any frame renders alone.
(function () {
  "use strict";
  const B = window.BUILD;
  const canvas = document.getElementById("gl3d");
  if (!B || !canvas || !window.THREE) return;
  const W = canvas.width, H = canvas.height;
  const S = B.stages.slice();
  while (S.length < 11) S.push(1e9);
  const OUTRO = B.outro || 1e9;

  let seed = 11;
  function rand() {
    seed = (seed + 0x6D2B79F5) | 0;
    let t = seed;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  }
  const clamp = (x, a, b) => Math.max(a, Math.min(b, x));
  const smooth = (x) => { x = clamp(x, 0, 1); return x * x * (3 - 2 * x); };
  const easeOut = (x) => 1 - Math.pow(1 - clamp(x, 0, 1), 3);
  const backOut = (x) => { x = clamp(x, 0, 1); const c = 1.6; return 1 + (c + 1) * Math.pow(x - 1, 3) + c * Math.pow(x - 1, 2); };
  const bounce = (x) => {
    x = clamp(x, 0, 1); const n = 7.5625, d = 2.75;
    if (x < 1 / d) return n * x * x;
    if (x < 2 / d) return n * (x -= 1.5 / d) * x + 0.75;
    if (x < 2.5 / d) return n * (x -= 2.25 / d) * x + 0.9375;
    return n * (x -= 2.625 / d) * x + 0.984375;
  };
  // p(k, dur): progress through stage k's main animation.
  const p = (k, dur) => clamp((t_ - S[k]) / dur, 0, 1);
  let t_ = 0;
  const after = (k) => t_ >= S[k];

  // ---- Renderer, light, sky ------------------------------------------------
  const renderer = new THREE.WebGLRenderer({ canvas, antialias: true, preserveDrawingBuffer: true });
  renderer.setSize(W, H, false);
  renderer.shadowMap.enabled = true;
  renderer.shadowMap.type = THREE.PCFSoftShadowMap;
  const scene = new THREE.Scene();
  scene.background = new THREE.Color("#a6d0ea");
  scene.fog = new THREE.Fog("#a6d0ea", 55, 120);
  const camera = new THREE.PerspectiveCamera(38, W / H, 0.1, 300);
  scene.add(new THREE.HemisphereLight("#e8f4ff", "#5b6b3a", 1.0));
  const sun = new THREE.DirectionalLight("#fff2d8", 2.4);
  sun.position.set(-20, 34, 16);
  sun.castShadow = true;
  sun.shadow.mapSize.set(2048, 2048);
  Object.assign(sun.shadow.camera, { left: -30, right: 30, top: 30, bottom: -30, near: 1, far: 100 });
  sun.shadow.bias = -0.0005;
  scene.add(sun);

  function canvasTexture(size, paint, repeatX, repeatY) {
    const c = document.createElement("canvas"); c.width = c.height = size;
    paint(c.getContext("2d"), size);
    const tx = new THREE.CanvasTexture(c);
    tx.wrapS = tx.wrapT = THREE.RepeatWrapping;
    tx.repeat.set(repeatX || 1, repeatY || 1);
    return tx;
  }
  const stoneTex = canvasTexture(256, (x, n) => {
    x.fillStyle = "#4a453f"; x.fillRect(0, 0, n, n);
    for (let r = 0; r < 8; r++) for (let k = 0; k < 5; k++) {
      const s = 0.8 + rand() * 0.35;
      x.fillStyle = `rgb(${150 * s | 0},${140 * s | 0},${126 * s | 0})`;
      const w = 46 + rand() * 6;
      x.fillRect(((k * 52 + (r % 2) * 26) % n) + 2, r * 32 + 2, w, 28);
    }
  }, 3, 1);
  const foamTex = canvasTexture(128, (x, n) => {
    x.fillStyle = "rgba(255,255,255,0.0)"; x.clearRect(0, 0, n, n);
    for (let i = 0; i < 90; i++) {
      x.fillStyle = `rgba(255,255,255,${0.35 + rand() * 0.6})`;
      x.fillRect(rand() * n, rand() * n, 2 + rand() * 6, 10 + rand() * 30);
    }
  }, 4, 2);
  const concreteTex = canvasTexture(128, (x, n) => {
    x.fillStyle = "#a9a59d"; x.fillRect(0, 0, n, n);
    for (let i = 0; i < 400; i++) { const g = 140 + rand() * 50 | 0; x.fillStyle = `rgb(${g},${g - 3},${g - 8})`; x.fillRect(rand() * n, rand() * n, 2, 2); }
  }, 6, 1);

  // ---- Terrain: a valley with the river along x (flowing to +x) ----------
  const RIVER = 5;
  function groundHeight(x, z) {
    const a = Math.abs(z);
    const bank = smooth((a - RIVER + 0.5) / 4);
    let y = -1.9 * (1 - bank) + bank * 0.4;
    y += smooth((a - 16) / 14) * (3.5 + 1.5 * Math.sin(x * 0.07));
    return y + 0.25 * Math.sin(x * 0.21 + z * 0.13) * bank;
  }
  const geo = new THREE.PlaneGeometry(160, 120, 200, 150);
  geo.rotateX(-Math.PI / 2);
  const gp = geo.attributes.position, cols = [];
  const cGravel = new THREE.Color("#6e675d"), cSand = new THREE.Color("#a99a73"), cGrass = new THREE.Color("#78983a"), cGrass2 = new THREE.Color("#5a7a28"), cHill = new THREE.Color("#4f6d26");
  for (let i = 0; i < gp.count; i++) {
    const x = gp.getX(i), z = gp.getZ(i);
    const y = groundHeight(x, z) + (rand() - 0.5) * 0.08;
    gp.setY(i, y);
    const a = Math.abs(z);
    let c;
    if (a < RIVER - 0.5) c = cGravel.clone().multiplyScalar(0.85 + rand() * 0.25);
    else if (a < RIVER + 1.2) c = cSand.clone().lerp(cGravel, rand() * 0.4);
    else c = cGrass.clone().lerp(cGrass2, rand() * 0.8).lerp(cHill, smooth((a - 16) / 12));
    cols.push(c.r, c.g, c.b);
  }
  geo.setAttribute("color", new THREE.Float32BufferAttribute(cols, 3));
  geo.computeVertexNormals();
  const ground = new THREE.Mesh(geo, new THREE.MeshStandardMaterial({ vertexColors: true, roughness: 1, flatShading: true }));
  ground.receiveShadow = true;
  scene.add(ground);
  // Trees and boulders on the banks, away from the build.
  const trunkMat = new THREE.MeshStandardMaterial({ color: "#6b4024", flatShading: true });
  const leafMat = new THREE.MeshStandardMaterial({ color: "#3e6b28", flatShading: true });
  for (let i = 0; i < 46; i++) {
    const x = -55 + rand() * 110, side = rand() < 0.5 ? -1 : 1, z = side * (10 + rand() * 22);
    if (side > 0 && x > -18 && x < 42 && z < 36) continue; // keep the power site and camera paths clear
    const g = new THREE.Group();
    const tr = new THREE.Mesh(new THREE.CylinderGeometry(0.18, 0.26, 1.5, 6), trunkMat); tr.position.y = 0.75; g.add(tr);
    const lf = new THREE.Mesh(new THREE.ConeGeometry(1.2, 2.8, 7), leafMat); lf.position.y = 2.6; g.add(lf);
    g.position.set(x, groundHeight(x, z), z); g.scale.setScalar(0.8 + rand() * 0.7);
    g.traverse((o) => { o.castShadow = true; });
    scene.add(g);
  }

  // ---- Water ---------------------------------------------------------------
  const waterMat = new THREE.MeshStandardMaterial({ color: "#3aa3c4", roughness: 0.12, metalness: 0.15, transparent: true, opacity: 0.9 });
  function waterPlane(x0, x1, zw) {
    const g = new THREE.PlaneGeometry(x1 - x0, zw, 90, 40);
    g.rotateX(-Math.PI / 2); g.translate((x0 + x1) / 2, 0, 0);
    const m = new THREE.Mesh(g, waterMat); m.userData.base = g.attributes.position.array.slice();
    m.receiveShadow = true; scene.add(m); return m;
  }
  const upW = waterPlane(-80, 0.2, 60), downW = waterPlane(0.2, 80, 2 * RIVER + 1.5);
  function setWater(m, level, tt) {
    const pos = m.geometry.attributes.position, b = m.userData.base;
    for (let i = 0; i < pos.count; i++) {
      const x = b[i * 3], z = b[i * 3 + 2];
      pos.setY(i, level + Math.sin(x * 1.1 - tt * 3.2 + z * 0.3) * 0.05 + Math.cos(z * 1.6 + tt * 2.4) * 0.03);
    }
    pos.needsUpdate = true; m.geometry.computeVertexNormals();
  }
  // Flow streaks: thin foam lines drifting downstream.
  const streakMat = new THREE.MeshBasicMaterial({ color: "#eafcff", transparent: true, opacity: 0.6 });
  const streaks = new THREE.InstancedMesh(new THREE.BoxGeometry(1.6, 0.02, 0.06), streakMat, 90);
  const streakSeed = Array.from({ length: 90 }, () => [rand(), rand(), 0.6 + rand() * 0.8]);
  scene.add(streaks);
  // Spray and droplets.
  const sprayMat = new THREE.MeshStandardMaterial({ color: "#f2fbff", roughness: 0.4, transparent: true, opacity: 0.7 });
  const spray = new THREE.InstancedMesh(new THREE.IcosahedronGeometry(0.06, 0), sprayMat, 600);
  scene.add(spray);
  const splashSeed = Array.from({ length: 600 }, () => [rand(), rand(), rand(), rand()]);

  // ---- Stones and clay (the first wall) -----------------------------------
  const stoneMat = new THREE.MeshStandardMaterial({ color: "#8a8174", roughness: 0.95, flatShading: true });
  const stones = [];
  for (let i = 0; i < 9; i++) {
    const g = new THREE.IcosahedronGeometry(0.95, 1); const ps = g.attributes.position;
    for (let k = 0; k < ps.count; k++) ps.setXYZ(k, ps.getX(k) * (1.0 + rand() * 0.25), ps.getY(k) * (0.95 + rand() * 0.3), ps.getZ(k) * (0.85 + rand() * 0.2));
    g.computeVertexNormals();
    const m = new THREE.Mesh(g, stoneMat); m.castShadow = true; m.receiveShadow = true;
    const z = -RIVER + 0.65 + i * ((2 * RIVER - 1.3) / 8);
    m.userData = { z, delay: i * 0.22, rot: rand() * 6, drift: 4 + rand() * 6, spin: (rand() - 0.5) * 6 };
    scene.add(m); stones.push(m);
  }
  const gapMat = new THREE.MeshBasicMaterial({ color: "#ff2a2a", transparent: true, opacity: 0 });
  const clayMat = new THREE.MeshStandardMaterial({ color: "#c8692b", roughness: 1, flatShading: true });
  const gaps = [], clays = [];
  for (let i = 0; i < 8; i++) {
    const z = (stones[i].userData.z + stones[i + 1].userData.z) / 2;
    const gm = new THREE.Mesh(new THREE.BoxGeometry(1.4, 1.8, 0.32), gapMat.clone()); gm.position.set(0, -1.0, z); scene.add(gm); gaps.push(gm);
    const cg = new THREE.DodecahedronGeometry(0.62, 1); const cp = cg.attributes.position;
    for (let k = 0; k < cp.count; k++) cp.setXYZ(k, cp.getX(k) * 1.15, cp.getY(k) * (0.9 + rand() * 0.2), cp.getZ(k) * 0.75);
    cg.computeVertexNormals();
    const c = new THREE.Mesh(cg, clayMat); c.castShadow = true;
    c.userData = { z, delay: i * 0.16, drift: 3 + rand() * 5 };
    scene.add(c); clays.push(c);
  }
  // Overflow curtain: a curved sheet of foamy water over the wall.
  const curtainGeo = new THREE.PlaneGeometry(1, 2 * RIVER - 0.6, 12, 1);
  const cpos = curtainGeo.attributes.position;
  for (let i = 0; i < cpos.count; i++) {
    const u = cpos.getX(i) + 0.5; // 0..1 across the drop
    cpos.setXYZ(i, u * 1.8, 0.6 - u * u * 2.2, cpos.getY(i));
  }
  curtainGeo.computeVertexNormals();
  const curtainMat = new THREE.MeshStandardMaterial({ color: "#e8f9ff", emissive: "#7fc8de", emissiveIntensity: 0.6, map: foamTex, transparent: true, opacity: 0.0, roughness: 0.3, side: THREE.DoubleSide });
  const curtain = new THREE.Mesh(curtainGeo, curtainMat);
  curtain.position.set(0.4, 0, 0);
  scene.add(curtain);

  // ---- The real dam: trapezoid layers of cut stone ------------------------
  const damMat = new THREE.MeshStandardMaterial({ map: stoneTex, roughness: 0.9 });
  const LAYERS = 7, DAM_W = 2 * (RIVER + 3.2);
  const damLayers = [];
  for (let l = 0; l < LAYERS; l++) {
    const h = 0.48, bottomW = 3.4 - l * 0.38;
    const m = new THREE.Mesh(new THREE.BoxGeometry(bottomW, h, DAM_W - l * 0.2), damMat);
    m.castShadow = true; m.receiveShadow = true;
    m.userData = { y: -1.75 + h / 2 + l * h, delay: l * 0.32 };
    scene.add(m); damLayers.push(m);
  }
  // ---- Spillway: a sloped concrete channel on the right bank --------------
  const SPILL_Z = RIVER + 0.9; // on the bank slope, above the river
  const spill = new THREE.Group();
  const chanMat = new THREE.MeshStandardMaterial({ map: concreteTex, roughness: 0.9 });
  const run = 12, drop = 1.5, slope = Math.atan2(drop, run);
  const floor = new THREE.Mesh(new THREE.BoxGeometry(run, 0.3, 2.4), chanMat); floor.castShadow = true; floor.receiveShadow = true;
  spill.add(floor);
  for (const s of [-1, 1]) { const wl = new THREE.Mesh(new THREE.BoxGeometry(run, 0.8, 0.25), chanMat); wl.position.set(0, 0.4, s * 1.2); wl.castShadow = true; spill.add(wl); }
  const spillWaterMat = new THREE.MeshStandardMaterial({ color: "#7fd6f0", emissive: "#1d5f78", map: foamTex.clone(), transparent: true, opacity: 0.9, roughness: 0.2 });
  spillWaterMat.map.repeat.set(6, 1);
  const spillWater = new THREE.Mesh(new THREE.BoxGeometry(run, 0.1, 2.1), spillWaterMat); spillWater.position.y = 0.22;
  spill.add(spillWater);
  spill.rotation.z = -slope;
  spill.position.set(run / 2 - 1.2, 0.3 - drop / 2, SPILL_Z);
  scene.add(spill);
  const spillEnd = new THREE.Vector3(run - 1.2, 0.3 - drop, SPILL_Z);

  // ---- Water wheel at the end of the channel -------------------------------
  const wheel = new THREE.Group();
  const woodMat = new THREE.MeshStandardMaterial({ color: "#7a4c28", roughness: 0.9, flatShading: true });
  const rotor = new THREE.Group();
  for (const zz of [-0.55, 0.55]) { const rim = new THREE.Mesh(new THREE.TorusGeometry(1.7, 0.12, 6, 24), woodMat); rim.position.z = zz; rotor.add(rim); }
  for (let i = 0; i < 10; i++) {
    const a = (i / 10) * Math.PI * 2;
    const paddle = new THREE.Mesh(new THREE.BoxGeometry(0.12, 0.75, 1.2), woodMat);
    paddle.position.set(Math.cos(a) * 1.55, Math.sin(a) * 1.55, 0); paddle.rotation.z = a; rotor.add(paddle);
    const spoke = new THREE.Mesh(new THREE.BoxGeometry(1.7, 0.08, 0.08), woodMat);
    spoke.position.set(Math.cos(a) * 0.85, Math.sin(a) * 0.85, 0); spoke.rotation.z = a; rotor.add(spoke);
  }
  const axle = new THREE.Mesh(new THREE.CylinderGeometry(0.12, 0.12, 3.2, 8), new THREE.MeshStandardMaterial({ color: "#555", metalness: 0.6 }));
  axle.rotation.x = Math.PI / 2; axle.position.z = 0.9; rotor.add(axle);
  rotor.traverse((o) => { o.castShadow = true; });
  wheel.add(rotor);
  for (const zz of [-0.95, 0.95]) { const post = new THREE.Mesh(new THREE.BoxGeometry(0.25, 2.2, 0.25), woodMat); post.position.set(0, -1.0, zz); post.castShadow = true; wheel.add(post); }
  wheel.position.set(spillEnd.x + 1.4, spillEnd.y + 0.7, SPILL_Z);
  scene.add(wheel);

  // ---- Generator, poles, house ---------------------------------------------
  const power = new THREE.Group();
  const gen = new THREE.Mesh(new THREE.BoxGeometry(1.6, 1.3, 1.3), new THREE.MeshStandardMaterial({ color: "#2f6db5", roughness: 0.5, metalness: 0.3 }));
  gen.position.set(wheel.position.x, groundHeight(wheel.position.x, SPILL_Z + 2.4) + 0.65, SPILL_Z + 2.4); gen.castShadow = true; power.add(gen);
  const genLight = new THREE.Mesh(new THREE.BoxGeometry(0.4, 0.2, 0.05), new THREE.MeshStandardMaterial({ color: "#333", emissive: "#ffcc33", emissiveIntensity: 0 }));
  genLight.position.set(gen.position.x, gen.position.y + 0.3, gen.position.z + 0.66); power.add(genLight);
  const house = new THREE.Group();
  const walls = new THREE.Mesh(new THREE.BoxGeometry(3, 2.2, 2.6), new THREE.MeshStandardMaterial({ color: "#e8dcc4", roughness: 0.9 }));
  walls.position.y = 1.1; walls.castShadow = true; house.add(walls);
  const roof = new THREE.Mesh(new THREE.CylinderGeometry(1.9, 1.9, 3.4, 3), new THREE.MeshStandardMaterial({ color: "#a8392a", flatShading: true }));
  roof.rotation.z = Math.PI / 2; roof.rotation.y = Math.PI / 2; roof.position.y = 2.75; roof.castShadow = true; house.add(roof);
  const windows = [];
  for (const xx of [-0.8, 0.8]) {
    const w = new THREE.Mesh(new THREE.BoxGeometry(0.6, 0.6, 0.05), new THREE.MeshStandardMaterial({ color: "#334", emissive: "#ffd46b", emissiveIntensity: 0 }));
    w.position.set(xx, 1.3, 1.31); house.add(w); windows.push(w);
  }
  const HX = gen.position.x + 9, HZ = gen.position.z + 6;
  house.position.set(HX, groundHeight(HX, HZ), HZ);
  house.rotation.y = Math.atan2(gen.position.x - HX, gen.position.z - HZ); // front (lit windows) faces the generator
  power.add(house);
  const poleMat = new THREE.MeshStandardMaterial({ color: "#5a3a20", flatShading: true });
  const polePts = [];
  for (let i = 0; i < 3; i++) {
    const u = (i + 1) / 4, x = gen.position.x + (HX - gen.position.x) * u, z = gen.position.z + (HZ - gen.position.z) * u;
    const y0 = groundHeight(x, z);
    const pole = new THREE.Mesh(new THREE.CylinderGeometry(0.08, 0.1, 3.4, 6), poleMat); pole.position.set(x, y0 + 1.7, z); pole.castShadow = true; power.add(pole);
    polePts.push(new THREE.Vector3(x, y0 + 3.3, z));
  }
  const wirePts = [new THREE.Vector3(gen.position.x, gen.position.y + 0.65, gen.position.z), ...polePts, new THREE.Vector3(HX, house.position.y + 2.2, HZ)];
  const wire = new THREE.Mesh(new THREE.TubeGeometry(new THREE.CatmullRomCurve3(wirePts), 60, 0.035, 5), new THREE.MeshStandardMaterial({ color: "#222" }));
  power.add(wire);
  scene.add(power);

  // ---- Camera anchors per stage -------------------------------------------
  const cams = [
    [[-30, 26, 26], [0, -1, 0]],     // river
    [[-12, 9, 15], [0, -1, 0]],      // stones
    [[10, 5, 11], [0, -1, 0]],       // leak (downstream side)
    [[-7, 6.5, 10], [0, -0.8, 0]],   // clay
    [[-18, 22, 22], [-3, -1, 0]],    // lake
    [[10, 4.5, 10], [0, -0.4, 0]],   // overflow
    [[17, 9, 13], [5, -1, 0]],       // washout
    [[-14, 10, 17], [0, -0.5, 0]],   // dam
    [[-3, 8, 19], [5, -0.6, 5.9]],    // spillway
    [[20, 6, 17], [12, -0.5, 6]],     // wheel
    [[5, 7.5, 2.5], [18, 1.5, 12.5]],  // power: behind the generator, looking at the house
    [[-34, 34, 38], [3, 0, 5]],      // outro
  ];
  const camPos = new THREE.Vector3(), camTgt = new THREE.Vector3(), a = new THREE.Vector3(), b = new THREE.Vector3();
  function placeCamera(t) {
    const starts = S.slice(0, 11).concat([OUTRO]);
    let k = 0;
    for (let i = 0; i < starts.length; i++) if (t >= starts[i]) k = i;
    const prev = cams[Math.max(0, k - 1)], cur = cams[k];
    const m = k === 0 ? 1 : smooth((t - starts[k]) / 1.6);
    camPos.copy(a.fromArray(prev[0])).lerp(b.fromArray(cur[0]), m);
    camTgt.copy(a.fromArray(prev[1])).lerp(b.fromArray(cur[1]), m);
    // A slow drift keeps every shot alive.
    const drift = (t - starts[k]) * 0.35;
    camera.position.set(camPos.x + Math.sin(drift * 0.5) * 1.2, camPos.y + drift * 0.05, camPos.z + drift * 0.25);
    camera.lookAt(camTgt);
  }

  const m4 = new THREE.Matrix4(), q = new THREE.Quaternion(), v = new THREE.Vector3(), sc = new THREE.Vector3(), e = new THREE.Euler();

  function draw(t) {
    t_ = t;
    // Water levels.
    const lakeUp = smooth(p(4, 4.5)), drained = smooth(p(6, 2.2)), refill = smooth((t - S[7] - 1.8) / 4);
    let up = -1.25 + 0.95 * lakeUp;
    if (after(6)) up = -1.25 + 0.95 * (1 - drained);
    if (after(7)) up = -1.25 + 1.25 * refill;
    let down = -1.25;
    if (after(4) && !after(6)) down = -1.25 - 0.3 * lakeUp;
    if (after(6) && !after(7)) down = -1.55 + 0.6 * Math.sin(Math.PI * clamp((t - S[6]) / 2.2, 0, 1));
    if (after(7)) down = -1.4;
    setWater(upW, up, t);
    setWater(downW, down, t);

    // Stones: drop in one by one, then tumble away in the washout.
    const wash = p(6, 1.8);
    stones.forEach((s) => {
      const d = s.userData, k = (t - S[1] - d.delay) / 0.75;
      s.visible = t >= S[1] + d.delay && wash < 1;
      if (!s.visible) return;
      const drop = bounce(k);
      s.position.set(wash * d.drift, -1.0 + 9 * (1 - drop) - wash * 1.2, d.z + wash * (d.z * 0.15));
      s.rotation.set(d.rot + wash * d.spin, d.rot * 0.7 + wash * 2, wash * d.spin * 0.5);
    });
    // Gaps glow red while water leaks, then clay seals them.
    const leaking = after(2) && !after(3);
    const sealing = p(3, 1.6);
    gaps.forEach((g, i) => {
      g.visible = leaking || (after(3) && sealing < 0.6);
      g.material.opacity = (0.45 + 0.35 * Math.sin(t * 9 + i)) * (1 - sealing);
    });
    clays.forEach((c) => {
      const d = c.userData, k = (t - S[3] - d.delay) / 0.5;
      c.visible = k > 0 && wash < 1;
      if (!c.visible) return;
      const sq = backOut(k);
      c.scale.set(sq, sq * (0.9 + 0.1 * Math.sin(k * 6)), sq);
      c.position.set(wash * d.drift, -0.55 - wash * 1.1, d.z);
      c.rotation.set(wash * 3, 0, wash * 2);
    });
    // Overflow curtain.
    const over = after(5) && !after(6) ? smooth(p(5, 1.2)) : (after(6) ? 1 - smooth(p(6, 0.8)) : 0);
    curtain.material.opacity = 0.85 * over;
    curtain.visible = over > 0.01;
    curtain.position.y = up + 0.15;
    curtainMat.map.offset.y = -t * 1.8;

    // The new dam rises layer by layer.
    damLayers.forEach((l) => {
      const k = (t - S[7] - l.userData.delay) / 0.45;
      l.visible = k > 0;
      l.position.set(0, l.userData.y - 2 * (1 - easeOut(k)), 0);
    });
    // Spillway rises out of the bank, then water runs down it.
    const sp = p(8, 1.2);
    spill.visible = sp > 0;
    spill.position.y = 0.3 - drop / 2 - 3 * (1 - easeOut(sp));
    spillWater.visible = t > S[8] + 1.2;
    spillWaterMat.map.offset.x = -t * 2.2;
    // Wheel grows in, then spins faster as water hits it.
    const wk = p(9, 0.8);
    wheel.visible = wk > 0;
    wheel.scale.setScalar(Math.max(0.001, backOut(wk)));
    const spinT = Math.max(0, t - S[9] - 0.8);
    // Spins up over the first second, then turns steadily.
    rotor.rotation.z = -(spinT < 1 ? 1.2 * spinT * spinT : 1.2 + 2.4 * (spinT - 1));
    // Power: generator and poles appear, then the house lights up.
    const pw = p(10, 0.9);
    power.visible = pw > 0;
    power.position.y = -4 * (1 - easeOut(pw));
    const lit = smooth((t - S[10] - 1.6) / 0.6);
    windows.forEach((w) => { w.material.emissiveIntensity = 2.2 * lit; });
    genLight.material.emissiveIntensity = 2 * lit * (0.8 + 0.2 * Math.sin(t * 20));

    // Flow streaks: downstream always; upstream only while the river runs.
    let ns = 0;
    const upFlows = !after(4) || (after(6) && !after(7));
    streakSeed.forEach(([r1, r2, len]) => {
      const x = ((r1 * 120 + t * 3.0) % 120) - 60;
      if (x < 0 && !upFlows) return;
      if (Math.abs(x) < 1.5) return;
      v.set(x, (x < 0 ? up : down) + 0.07, -RIVER + 0.6 + r2 * (2 * RIVER - 1.2));
      m4.compose(v, q, sc.set(len, 1, 1)); streaks.setMatrixAt(ns++, m4);
    });
    streaks.count = ns; streaks.instanceMatrix.needsUpdate = true;

    // Spray: splashes as stones land, jets through gaps, foam under the
    // overflow, the washout burst, and foam where the channel meets the wheel.
    let n = 0;
    const put = (x, y, z, s) => { if (n < 600) { v.set(x, y, z); m4.compose(v, q, sc.setScalar(s)); spray.setMatrixAt(n++, m4); } };
    stones.forEach((s, i) => {
      const k = t - S[1] - s.userData.delay - 0.32;
      if (k > 0 && k < 0.6) for (let j = 0; j < 10; j++) { const r = splashSeed[i * 10 + j], ang = r[0] * 6.28, sp2 = k * (2 + r[1] * 2); put(Math.cos(ang) * sp2, -1.2 + Math.sin(Math.PI * k / 0.6) * (0.8 + r[2]), s.userData.z + Math.sin(ang) * sp2, 1 - k); }
    });
    if (leaking || (after(3) && sealing < 0.5)) gaps.forEach((g, i) => {
      for (let j = 0; j < 14; j++) { const r = splashSeed[100 + i * 14 + j], life = (t * 1.4 + r[0]) % 1; put(0.4 + life * 3.2, -0.9 + life * 0.5 - life * life * 1.2, g.position.z + (r[1] - 0.5) * 0.25, 1.1 * (1 - sealing)); }
    });
    if (over > 0.3) for (let j = 0; j < 160; j++) {
      const r = splashSeed[220 + j], life = (t * 1.2 + r[0]) % 1;
      put(2.1 + life * 2.4 * r[1], -1.2 + Math.sin(Math.PI * life) * 0.9 * r[2], -RIVER + 0.4 + r[3] * (2 * RIVER - 0.8), 1.3 * (1 - life));
    }
    const burst = t - S[6];
    if (burst > 0 && burst < 1.6) for (let j = 0; j < 140; j++) {
      const r = splashSeed[380 + j], ang = r[0] * 6.28, sp2 = burst * (3 + r[1] * 5);
      put(Math.abs(Math.cos(ang)) * sp2, -1.2 + Math.sin(Math.PI * burst / 1.6) * (1 + r[2] * 2.5), -RIVER + r[3] * 2 * RIVER, 1.6 * (1 - burst / 1.6));
    }
    if (spillWater.visible) for (let j = 0; j < 50; j++) {
      const r = splashSeed[520 + j], life = (t * 1.5 + r[0]) % 1;
      put(spillEnd.x + 0.3 + life * 1.0, spillEnd.y + 0.6 - life * life * 1.6, SPILL_Z + (r[1] - 0.5) * 1.2, 1.2 * (1 - life));
    }
    spray.count = n; spray.instanceMatrix.needsUpdate = true;

    placeCamera(t);
    renderer.render(scene, camera);
  }

  window.Scene3D = { draw };
})();
