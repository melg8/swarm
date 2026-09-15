/*

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT

*/

// The navmesh viewer of the -show-navmesh mode (docs/navmesh.md): the
// three.js rendering of the mesh tiles with the interactive route
// search. A double click on the mesh picks the start, the second
// double click picks the destination, the server runs the real
// corridor search (the same query the hunt loop asks) and the answer
// draws as the funnel polyline with the measured construction time.
// The module boots through the dynamic import of main.js; three.js
// itself is vendored at web/vendor/three.module.min.js so the viewer
// works offline like the rest of the interface.

import * as THREE from "/vendor/three.module.min.js";

// The binary geometry contract of the server (navmesh.go): the NMV1
// header, the contiguous int16 corner block and the area tail.
const GEO_MAGIC = 0x31564d4e;
const GEO_HEADER = 24;
const CELL_SIZE = 16;

// The terrain palette: the ground polygons ramp through the height
// range of the tile, the water polygons keep the flat blue.
const WATER_COLOR = { r: 62, g: 130, b: 214 };
const GROUND_LOW = { r: 47, g: 84, b: 58 };
const GROUND_HIGH = { r: 172, g: 158, b: 128 };

// The path drawing colors (drawn with depthTest off: the corridor
// stays readable through the terrain).
const START_COLOR = 0x3fd66a;
const END_COLOR = 0xff5c5c;
const PATH_COLOR = 0xffb020;
const WAYPOINT_COLOR = 0xffffff;

// The Viewer bundles the three.js state behind one object so the init
// stays a single closure.
const viewer = {
  renderer: null,
  scene: null,
  camera: null,
  rig: null,
  tiles: new Map(),
  heightScale: 1,
  filter: "swim",
  pendingStart: null,
  path: null,
  startMarker: null,
  endMarker: null,
  markerRadius: 24,
};

// init boots the viewer for the /api/config payload of the navmesh
// mode. It builds the full screen surface, loads the tile geometries
// of the initial selection and wires the double click search.
function init(config) {
  const navmesh = config.navmesh;
  buildSurface(navmesh);

  if (!navmesh || !navmesh.tiles || navmesh.tiles.length === 0) {
    showStatus("no tiles", "no navmesh tiles in " + (navmesh ? navmesh.dir : "") +
      " - build them with cmd/navmesh-build first");
    return;
  }

  const initial = navmesh.initialTiles && navmesh.initialTiles.length > 0
    ? navmesh.initialTiles
    : navmesh.tiles;
  const initialKeys = new Set(initial.map(tileKey));
  for (const tile of navmesh.tiles) {
    addTileRow(tile, initialKeys.has(tileKey(tile)));
  }
  frameInitialTiles(initial);
  for (const tile of initial) {
    void ensureTile(tile);
  }
}

// tileKey names one tile of the listing.
function tileKey(tile) {
  return tile.col + "_" + tile.row;
}

// buildSurface creates the DOM overlay, the renderer and the camera
// rig; a WebGL failure degrades into an error panel.
function buildSurface(navmesh) {
  document.body.classList.add("mode-navmesh");
  const root = document.createElement("div");
  root.id = "navmesh-view";
  root.innerHTML = `
    <canvas id="nmv-canvas"></canvas>
    <div class="nmv-panel nmv-side">
      <div class="nmv-title">swarm &middot; navmesh viewer</div>
      <div class="nmv-sub" id="nmv-dir"></div>
      <div class="nmv-section">tiles</div>
      <label class="nmv-row"><input type="checkbox" id="nmv-all" checked>
        <span>all stitched</span></label>
      <div id="nmv-tiles" class="nmv-tiles"></div>
      <div class="nmv-section">route filter</div>
      <select id="nmv-filter" class="nmv-select">
        <option value="swim" selected>swim (water costs 3x)</option>
        <option value="dry">dry (water is a wall)</option>
      </select>
      <div class="nmv-section">height scale</div>
      <select id="nmv-height" class="nmv-select">
        <option value="1" selected>1x (true terrain)</option>
        <option value="2">2x</option>
        <option value="4">4x</option>
      </select>
      <div class="nmv-section">legend</div>
      <div class="nmv-row"><span class="nmv-swatch nmv-ground"></span>
        <span>ground (height ramp)</span></div>
      <div class="nmv-row"><span class="nmv-swatch nmv-water"></span>
        <span>water area</span></div>
      <button id="nmv-clear" class="btn nmv-clear">clear route</button>
    </div>
    <div class="nmv-panel nmv-result" id="nmv-result">
      <div class="nmv-section">route search</div>
      <div class="nmv-status" id="nmv-status">double click the mesh</div>
      <div class="nmv-timer" id="nmv-timer"></div>
      <div class="nmv-timer-sub" id="nmv-timer-sub"></div>
      <div class="nmv-stats" id="nmv-stats"></div>
    </div>
    <div class="nmv-hint">drag rotates &middot; wheel zooms &middot;
      right drag pans &middot; double click: first the start, then the
      destination</div>`;
  document.body.appendChild(root);

  if (navmesh) {
    document.getElementById("nmv-dir").textContent =
      navmesh.tileFiles + " tiles in " + navmesh.dir;
  }

  const canvas = document.getElementById("nmv-canvas");
  try {
    viewer.renderer = new THREE.WebGLRenderer({
      canvas,
      antialias: true,
      powerPreference: "high-performance",
    });
  } catch (err) {
    showStatus("error", "WebGL is unavailable: " + err.message);
    return;
  }
  viewer.renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
  viewer.renderer.setClearColor(0x0d1117, 1);

  viewer.scene = new THREE.Scene();
  viewer.camera = new THREE.PerspectiveCamera(
    60, 1, 8, 400000);
  const hemi = new THREE.HemisphereLight(0xdfe7f0, 0x2c241a, 0.9);
  viewer.scene.add(hemi);
  const sun = new THREE.DirectionalLight(0xffffff, 1.15);
  sun.position.set(0.45, 1, 0.25);
  viewer.scene.add(sun);

  viewer.rig = createOrbitRig(viewer.camera, canvas);
  canvas.addEventListener("dblclick", onDoubleClick);

  document.getElementById("nmv-filter").addEventListener("change", (e) => {
    viewer.filter = e.target.value;
  });
  document.getElementById("nmv-height").addEventListener("change", (e) => {
    setHeightScale(Number(e.target.value));
  });
  document.getElementById("nmv-all").addEventListener("change", (e) => {
    setAllTiles(e.target.checked);
  });
  document.getElementById("nmv-clear").addEventListener("click", clearRoute);

  const resize = () => {
    const width = window.innerWidth, height = window.innerHeight;
    viewer.renderer.setSize(width, height, false);
    viewer.camera.aspect = width / height;
    viewer.camera.updateProjectionMatrix();
  };
  window.addEventListener("resize", resize);
  resize();
  requestAnimationFrame(renderLoop);
}

// renderLoop redraws the scene (the tiles are static, but the camera
// rig and the route overlays move between frames).
function renderLoop() {
  if (viewer.rig) {
    viewer.rig.update();
  }
  if (viewer.renderer) {
    viewer.renderer.render(viewer.scene, viewer.camera);
  }
  requestAnimationFrame(renderLoop);
}

// createOrbitRig is the minimal orbit camera of the viewer: left drag
// rotates around the target, the wheel dollies, right drag (or
// shift-drag) pans the target. No damping - the debug viewer prefers
// the direct response.
function createOrbitRig(camera, dom) {
  const rig = {
    camera,
    target: new THREE.Vector3(),
    radius: 60000,
    phi: 0.95,
    theta: Math.PI / 4,
    update() {
      const sinPhi = Math.sin(this.phi);
      this.camera.position.set(
        this.target.x + this.radius * sinPhi * Math.sin(this.theta),
        this.target.y + this.radius * Math.cos(this.phi),
        this.target.z + this.radius * sinPhi * Math.cos(this.theta));
      this.camera.lookAt(this.target);
    },
  };
  let mode = null;
  let lastX = 0, lastY = 0;

  dom.addEventListener("contextmenu", (e) => e.preventDefault());
  dom.addEventListener("pointerdown", (e) => {
    mode = (e.button === 0 && !e.shiftKey) ? "rotate" : "pan";
    lastX = e.clientX;
    lastY = e.clientY;
    dom.setPointerCapture(e.pointerId);
  });
  dom.addEventListener("pointermove", (e) => {
    if (!mode) {
      return;
    }
    const dx = e.clientX - lastX, dy = e.clientY - lastY;
    lastX = e.clientX;
    lastY = e.clientY;
    if (mode === "rotate") {
      rig.theta -= dx * 0.005;
      rig.phi = Math.min(1.52, Math.max(0.08, rig.phi - dy * 0.005));
    } else {
      const scale = rig.radius * 0.0016;
      const forward = new THREE.Vector3();
      rig.camera.getWorldDirection(forward);
      const right = new THREE.Vector3().crossVectors(
        forward, new THREE.Vector3(0, 1, 0)).normalize();
      const up = new THREE.Vector3().crossVectors(
        right, forward).normalize();
      rig.target.addScaledVector(right, -dx * scale);
      rig.target.addScaledVector(up, dy * scale);
    }
  });
  const release = () => { mode = null; };
  dom.addEventListener("pointerup", release);
  dom.addEventListener("pointercancel", release);
  dom.addEventListener("wheel", (e) => {
    e.preventDefault();
    const factor = e.deltaY > 0 ? 1.12 : 1 / 1.12;
    rig.radius = Math.min(300000, Math.max(120, rig.radius * factor));
  }, { passive: false });

  return rig;
}

// frameInitialTiles aims the camera at the union of the initially
// selected tiles.
function frameInitialTiles(tiles) {
  if (!tiles || tiles.length === 0 || !viewer.rig) {
    return;
  }
  let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
  for (const tile of tiles) {
    minX = Math.min(minX, tile.minX);
    minY = Math.min(minY, tile.minY);
    maxX = Math.max(maxX, tile.maxX);
    maxY = Math.max(maxY, tile.maxY);
  }
  const size = Math.max(maxX - minX, maxY - minY);
  viewer.rig.target.set((minX + maxX) / 2, 0, (minY + maxY) / 2);
  viewer.rig.radius = Math.max(200, size * 1.1);
  viewer.rig.phi = 0.95;
  viewer.rig.theta = Math.PI / 4;
  viewer.markerRadius = Math.max(10, Math.min(60, size * 0.0022));
}

// addTileRow appends one tile checkbox row.
function addTileRow(tile, checked) {
  const list = document.getElementById("nmv-tiles");
  const row = document.createElement("label");
  row.className = "nmv-row nmv-tile";
  const key = tileKey(tile);
  row.innerHTML = `<input type="checkbox" data-tile="${key}"` +
    (checked ? " checked" : "") + `><span>${key}</span>` +
    `<span class="nmv-tile-status" data-status="${key}"></span>`;
  row.querySelector("input").addEventListener("change", (e) => {
    setTileVisible(key, e.target.checked);
    syncAllBox();
  });
  list.appendChild(row);
  viewer.tiles.set(key, {
    mesh: null,
    info: tile,
    loading: false,
    visible: checked,
  });
}

// syncAllBox mirrors the per tile checkboxes into the master box.
function syncAllBox() {
  const all = document.getElementById("nmv-all");
  if (!all) {
    return;
  }
  let every = true;
  for (const entry of viewer.tiles.values()) {
    if (!entry.visible) {
      every = false;
      break;
    }
  }
  all.checked = every && viewer.tiles.size > 0;
}

// setAllTiles toggles every tile at once (the stitched world view).
function setAllTiles(visible) {
  for (const key of viewer.tiles.keys()) {
    setTileVisible(key, visible);
    const box = document.querySelector(
      `#nmv-tiles input[data-tile="${key}"]`);
    if (box) {
      box.checked = visible;
    }
  }
}

// setTileVisible shows or hides one tile, loading its geometry on
// the first show.
function setTileVisible(key, visible) {
  const entry = viewer.tiles.get(key);
  if (!entry) {
    return;
  }
  entry.visible = visible;
  if (visible) {
    void ensureTile(entry.info);
  }
  if (entry.mesh) {
    entry.mesh.visible = visible;
  }
}

// ensureTile fetches and builds the geometry of one tile exactly
// once; the visible flag follows the checkbox.
async function ensureTile(tile) {
  const key = tileKey(tile);
  const entry = viewer.tiles.get(key);
  if (!entry || entry.mesh || entry.loading) {
    return;
  }
  entry.loading = true;
  const status = document.querySelector(`[data-status="${key}"]`);
  if (status) {
    status.textContent = "loading";
  }
  try {
    const response = await fetch("/api/navmesh/geometry/" + key);
    if (!response.ok) {
      throw new Error("http " + response.status);
    }
    const buffer = await response.arrayBuffer();
    entry.mesh = buildTileMesh(buffer);
    entry.mesh.visible = entry.visible;
    viewer.scene.add(entry.mesh);
    if (status) {
      const polys = entry.mesh.userData.polys;
      status.textContent = polys.toLocaleString() + " polys";
    }
  } catch (err) {
    if (status) {
      status.textContent = "failed";
    }
    showStatus("error", "tile " + key + " failed to load: " + err.message);
  } finally {
    entry.loading = false;
  }
}

// buildTileMesh decodes the NMV1 payload into one draw call: the
// positions float block (worldMin + cell * 16), the per corner colors
// (the ground height ramp or the flat water blue) and the quad
// indices (0 2 1 / 0 2 3 - both triangles face up in the viewer
// mapping y = world height).
function buildTileMesh(buffer) {
  const view = new DataView(buffer);
  if (view.getUint32(0, true) !== GEO_MAGIC) {
    throw new Error("bad geometry magic");
  }
  const worldMinX = view.getInt32(8, true);
  const worldMinY = view.getInt32(12, true);
  const minH = view.getInt16(16, true);
  const maxH = view.getInt16(18, true);
  const polyCount = view.getUint32(20, true);
  const corners = new Int16Array(buffer, GEO_HEADER, polyCount * 12);
  const areas = new Uint8Array(
    buffer, GEO_HEADER + polyCount * 24, polyCount);

  const positions = new Float32Array(polyCount * 12);
  const colors = new Uint8Array(polyCount * 12);
  const span = Math.max(1, maxH - minH);
  for (let poly = 0; poly < polyCount; poly++) {
    const water = areas[poly] === 1;
    let sumH = 0;
    for (let corner = 0; corner < 4; corner++) {
      const src = (poly * 4 + corner) * 3;
      const dst = src;
      const cellX = corners[src], cellY = corners[src + 1];
      const height = corners[src + 2];
      sumH += height;
      // The viewer mapping: three x = world x, three y = world height
      // (scaled by the height exaggeration), three z = world y.
      positions[dst] = worldMinX + cellX * CELL_SIZE;
      positions[dst + 1] = height;
      positions[dst + 2] = worldMinY + cellY * CELL_SIZE;
      if (water) {
        colors[dst] = WATER_COLOR.r;
        colors[dst + 1] = WATER_COLOR.g;
        colors[dst + 2] = WATER_COLOR.b;
      } else {
        const t = Math.min(1, Math.max(0, (height - minH) / span));
        colors[dst] = GROUND_LOW.r + (GROUND_HIGH.r - GROUND_LOW.r) * t;
        colors[dst + 1] = GROUND_LOW.g + (GROUND_HIGH.g - GROUND_LOW.g) * t;
        colors[dst + 2] = GROUND_LOW.b + (GROUND_HIGH.b - GROUND_LOW.b) * t;
      }
    }
    // One flat color per polygon reads better than per corner
    // gradients over a warped quad: copy the first corner color.
    for (let corner = 1; corner < 4; corner++) {
      const dst = (poly * 4 + corner) * 3;
      colors[dst] = colors[poly * 12];
      colors[dst + 1] = colors[poly * 12 + 1];
      colors[dst + 2] = colors[poly * 12 + 2];
    }
  }

  const indices = new Uint32Array(polyCount * 6);
  for (let poly = 0; poly < polyCount; poly++) {
    const base = poly * 4;
    const at = poly * 6;
    indices[at] = base;
    indices[at + 1] = base + 2;
    indices[at + 2] = base + 1;
    indices[at + 3] = base;
    indices[at + 4] = base + 2;
    indices[at + 5] = base + 3;
  }

  const geometry = new THREE.BufferGeometry();
  geometry.setAttribute("position",
    new THREE.BufferAttribute(positions, 3));
  geometry.setAttribute("color",
    new THREE.BufferAttribute(colors, 3, true));
  geometry.setIndex(new THREE.BufferAttribute(indices, 1));
  geometry.computeVertexNormals();
  geometry.computeBoundingSphere();
  const material = new THREE.MeshLambertMaterial({
    vertexColors: true,
    flatShading: true,
    side: THREE.DoubleSide,
  });
  const mesh = new THREE.Mesh(geometry, material);
  mesh.userData.polys = polyCount;
  mesh.scale.y = viewer.heightScale;

  return mesh;
}

// setHeightScale exaggerates the vertical axis of every tile (the
// terrain relief of the geodata is subtle next to the 32768 unit
// region span); the route overlays rescale with it.
function setHeightScale(scale) {
  viewer.heightScale = scale;
  for (const entry of viewer.tiles.values()) {
    if (entry.mesh) {
      entry.mesh.scale.y = scale;
    }
  }
  if (viewer.path) {
    drawRouteOverlays(viewer.path);
  }
  if (viewer.pendingStart) {
    drawStartMarker(viewer.pendingStart);
  }
}

// onDoubleClick resolves the clicked mesh point and drives the two
// click search: the first double click arms the start marker, the
// second asks the server for the route.
function onDoubleClick(event) {
  if (!viewer.renderer) {
    return;
  }
  event.preventDefault();
  const rect = event.target.getBoundingClientRect();
  const pointer = new THREE.Vector2(
    ((event.clientX - rect.left) / rect.width) * 2 - 1,
    -((event.clientY - rect.top) / rect.height) * 2 + 1);
  const raycaster = new THREE.Raycaster();
  raycaster.setFromCamera(pointer, viewer.camera);
  const meshes = [];
  for (const entry of viewer.tiles.values()) {
    if (entry.mesh && entry.mesh.visible) {
      meshes.push(entry.mesh);
    }
  }
  const hits = raycaster.intersectObjects(meshes, false);
  if (hits.length === 0) {
    showStatus("missed", "double click on the mesh surface");
    return;
  }
  const point = hits[0].point;
  const world = {
    x: point.x,
    y: point.z,
    z: point.y / viewer.heightScale,
  };
  if (!viewer.pendingStart) {
    viewer.pendingStart = world;
    clearRouteOverlays();
    drawStartMarker(world);
    showStatus("start set", "double click the destination");
    document.getElementById("nmv-timer").textContent = "";
    document.getElementById("nmv-stats").innerHTML = "";
    return;
  }
  const start = viewer.pendingStart;
  viewer.pendingStart = null;
  drawEndMarker(world);
  void requestRoute(start, world);
}

// requestRoute posts the two clicked points and renders the answer.
async function requestRoute(start, end) {
  showStatus("searching", "corridor search running");
  document.getElementById("nmv-timer").textContent = "";
  document.getElementById("nmv-timer-sub").textContent = "";
  document.getElementById("nmv-stats").innerHTML = "";
  try {
    const response = await fetch("/api/navmesh/path", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        start: { x: start.x, y: start.y, z: start.z },
        end: { x: end.x, y: end.y, z: end.z },
        filter: viewer.filter,
      }),
    });
    if (!response.ok) {
      throw new Error("http " + response.status);
    }
    const answer = await response.json();
    renderRouteAnswer(answer);
  } catch (err) {
    showStatus("error", "route request failed: " + err.message);
  }
}

// renderRouteAnswer draws the funnel waypoints and fills the result
// panel: the status, the measured construction time and the search
// statistics.
function renderRouteAnswer(answer) {
  viewer.path = answer;
  if (answer.error) {
    showStatus("error", answer.error);
    return;
  }
  drawRouteOverlays(answer);
  if (answer.found) {
    showStatus("found", "route found");
  } else if (answer.partial) {
    showStatus("partial", "destination unreachable - closest reachable corridor");
  } else {
    showStatus("not found", "no route under the filter");
  }
  const timer = document.getElementById("nmv-timer");
  const timerSub = document.getElementById("nmv-timer-sub");
  if (answer.durationMs < 1) {
    timer.textContent = formatMicros(answer.durationMs);
    timerSub.textContent = "construction time";
  } else {
    timer.textContent = answer.durationMs.toFixed(3);
    timerSub.textContent = "ms construction time";
  }
  const stats = document.getElementById("nmv-stats");
  const waypoints = answer.waypoints || [];
  const rows = [
    ["waypoints", String(waypoints.length)],
    ["corridor polys", String(answer.corridor)],
    ["explored polys", String(answer.explored)],
    ["path length", Math.round(routeLength(waypoints)).toLocaleString() +
      " units"],
    ["filter", answer.filter],
  ];
  stats.innerHTML = rows.map(([name, value]) =>
    `<div class="nmv-stat"><span>${name}</span><b>${value}</b></div>`)
    .join("");
}

// formatMicros renders a sub millisecond construction time in
// microseconds.
function formatMicros(ms) {
  return Math.max(1, Math.round(ms * 1000)).toLocaleString() + " us";
}

// routeLength sums the 3D segment lengths of the waypoints.
function routeLength(waypoints) {
  let total = 0;
  for (let i = 1; i < waypoints.length; i++) {
    const a = waypoints[i - 1], b = waypoints[i];
    total += Math.hypot(b.x - a.x, b.y - a.y, b.z - a.z);
  }
  return total;
}

// drawRouteOverlays rebuilds the path line, the waypoint dots and
// the endpoint markers from the last answer (the height scale change
// re-renders through it).
function drawRouteOverlays(answer) {
  clearRouteOverlays();
  const waypoints = answer.waypoints || [];
  if (waypoints.length === 0) {
    return;
  }
  const lift = viewer.markerRadius * 0.4;
  const positions = new Float32Array(waypoints.length * 3);
  for (let i = 0; i < waypoints.length; i++) {
    const w = waypoints[i];
    positions[i * 3] = w.x;
    positions[i * 3 + 1] = w.z * viewer.heightScale + lift;
    positions[i * 3 + 2] = w.y;
  }
  const geometry = new THREE.BufferGeometry();
  geometry.setAttribute("position",
    new THREE.BufferAttribute(positions, 3));
  viewer.path = answer;
  const line = new THREE.Line(geometry, new THREE.LineBasicMaterial({
    color: PATH_COLOR,
    depthTest: false,
    transparent: true,
  }));
  line.renderOrder = 9;
  viewer.scene.add(line);
  viewer.routeLine = line;

  const dots = new THREE.Points(geometry, new THREE.PointsMaterial({
    color: WAYPOINT_COLOR,
    size: 7,
    sizeAttenuation: false,
    depthTest: false,
  }));
  dots.renderOrder = 10;
  viewer.scene.add(dots);
  viewer.routeDots = dots;

  drawStartMarker(waypoints[0]);
  drawEndMarker(waypoints[waypoints.length - 1]);
}

// drawStartMarker places the green start sphere.
function drawStartMarker(world) {
  if (viewer.startMarker) {
    viewer.scene.remove(viewer.startMarker);
    viewer.startMarker = null;
  }
  viewer.startMarker = makeMarker(START_COLOR);
  placeMarker(viewer.startMarker, world);
}

// drawEndMarker places the red destination sphere.
function drawEndMarker(world) {
  if (viewer.endMarker) {
    viewer.scene.remove(viewer.endMarker);
    viewer.endMarker = null;
  }
  viewer.endMarker = makeMarker(END_COLOR);
  placeMarker(viewer.endMarker, world);
}

// makeMarker builds one depthTest free sphere (the endpoints stay
// visible through the terrain).
function makeMarker(color) {
  const mesh = new THREE.Mesh(
    new THREE.SphereGeometry(1, 16, 12),
    new THREE.MeshBasicMaterial({
      color,
      depthTest: false,
      transparent: true,
    }));
  mesh.renderOrder = 11;

  return mesh;
}

// placeMarker positions one endpoint sphere over the world position.
function placeMarker(marker, world) {
  const radius = viewer.markerRadius;
  marker.scale.setScalar(radius);
  marker.position.set(
    world.x, world.z * viewer.heightScale + radius * 0.9, world.y);
  viewer.scene.add(marker);
}

// clearRoute drops the whole search state (the button and every new
// first click run through it).
function clearRoute() {
  viewer.pendingStart = null;
  viewer.path = null;
  clearRouteOverlays();
  showStatus("idle", "double click the mesh");
  document.getElementById("nmv-timer").textContent = "";
  document.getElementById("nmv-timer-sub").textContent = "";
  document.getElementById("nmv-stats").innerHTML = "";
}

// clearRouteOverlays removes the line, the dots and the markers.
function clearRouteOverlays() {
  for (const key of ["routeLine", "routeDots", "startMarker", "endMarker"]) {
    if (viewer[key]) {
      viewer.scene.remove(viewer[key]);
      viewer[key] = null;
    }
  }
}

// showStatus writes the badge text and the explanatory line of the
// result panel.
function showStatus(kind, text) {
  const status = document.getElementById("nmv-status");
  if (!status) {
    return;
  }
  status.className = "nmv-status nmv-status-" +
    kind.replace(/[^a-z-]/g, "");
  status.textContent = kind;
  status.title = text;
  const timerSub = document.getElementById("nmv-timer-sub");
  if (timerSub && !timerSub.textContent) {
    timerSub.textContent = text;
  }
}

export { init };
