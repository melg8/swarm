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
//
// The defect round taught the render its honesty rules: the
// logarithmic depth buffer keeps the 32768 unit tiles from z fighting
// at the viewing distances of the stitched world, the balanced
// hemisphere lighting keeps the steep cascade quads (the l2j slope
// smoothing cells - a fifth of the polygons) readable instead of
// black, and the water areas render as the submerged terrain they
// really are: the geodata water class is everything below the C1
// water level, so the blue ramps with depth instead of faking a flat
// surface at heights the riverbed never held.
//
// The inspection surface: every loaded tile carries its region grid
// outline, the cursor readout names the tile square under the pointer
// with its region local cell and world coordinates (a progressive
// raycast sweep - one tile per frame, the nearest bounding sphere
// first), and every route waypoint wears its coordinates as a label.
//
// The solid surface round closed the two standing render defects: the
// height step walls (the NMV2 wall block) fill the vertical gaps
// between the bilinear surfaces of adjacent rectangles - the polygon
// corners come from each rectangle's own inside cells, so every
// geodata height step used to read as a see-through black wedge - and
// the edge connections overlay draws the real link portals of the
// mesh as small open spans, green for the field to field connections,
// blue for the water to water ones and teal for the shore pairs: the
// honest answer of where a route may cross each shared edge.
//
// The camera is a flight rig: wasd flies along the view vector (the
// airplane feel - W follows the pitch), q/e descend and climb, the
// pointer drag yaws and pitches, the wheel retunes the cruise speed
// and shift boosts. The framing and the restore write the euler
// angles directly (order YXZ, roll always zero) - a lookAt under the
// default XYZ order once left a z angle behind that read as a rolled
// horizon. The feedback channel: the copy button freezes
// the whole view state - the camera pose, the route pair, the tile
// selection, the filter and the height scale - into one URL the
// owner pastes back; the viewer boots from those query parameters
// and re-runs the route automatically, so a pasted link reproduces
// the exact view and its answer anywhere the viewer runs.
// The module boots through the dynamic import of main.js; three.js
// itself is vendored at web/vendor/three.module.min.js so the viewer
// works offline like the rest of the interface.

import * as THREE from "/vendor/three.module.min.js";

// The binary geometry contract of the server (navmesh_geometry.go):
// the NMV2 header, the contiguous int16 corner block, the area tail,
// the link portal records (the edge connections overlay) and the
// height step wall records (the vertical filler between the bilinear
// surfaces of adjacent rectangles).
const GEO_MAGIC = 0x32564d4e;
const GEO_HEADER = 32;
const CELL_SIZE = 16;

// The C1 water surface height (navbuild.waterLevel): the water class
// polygons are the layers below it - the submerged terrain.
const WATER_LEVEL = -3780;

// The water depth ramp: the shallow shore approaches the light blue,
// the deep riverbed fades toward the dark navy.
const WATER_SHALLOW = { r: 86, g: 152, b: 220 };
const WATER_DEEP = { r: 16, g: 32, b: 60 };
// The depth that maps to the fully deep color (the deepest riverbeds
// of the shipped pack sit ~2500 units under the surface).
const WATER_DEEP_RANGE = 1800;

// The terrain palette: the ground polygons ramp through the height
// range of the tile (the per tile ramp doubles as the tile identity).
const GROUND_LOW = { r: 47, g: 84, b: 58 };
const GROUND_HIGH = { r: 172, g: 158, b: 128 };

// The path drawing colors (drawn with depthTest off: the corridor
// stays readable through the terrain).
const START_COLOR = 0x3fd66a;
const END_COLOR = 0xff5c5c;
const PATH_COLOR = 0xffb020;
const WAYPOINT_COLOR = 0xffffff;

// The tile grid outline colors: every loaded tile draws its region
// rectangle, the tile under the cursor lights up.
const TILE_OUTLINE_COLOR = 0x586063;
const TILE_HOVER_COLOR = 0xffb020;

// The edge connection classes of the link portal records: green for
// the field to field connections, blue for the water to water ones,
// teal for the shore pairs and gray for the links into tiles that did
// not resolve.
const CONNECTION_CLASSES = [
  [0.21, 0.84, 0.4],
  [0.3, 0.62, 1.0],
  [0.16, 0.8, 0.69],
  [0.55, 0.58, 0.62],
];
// The connection portals ride a small lift off the surface so the
// depth test never eats them at grazing angles.
const CONNECTION_LIFT = 10;

// The wall filler darkens the surface ramp so the vertical cracks it
// closes read as shaded cliff faces instead of navigable surface. The
// shade stays close to the surface: the ambient floor of the light rig
// already keeps the steep faces out of the black.
const WALL_SHADE = 0.88;

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
  routeStart: null,
  routeEnd: null,
  startMarker: null,
  endMarker: null,
  markerRadius: 24,
  waypointLabels: null,
  showWaypointCoords: true,
  showEdges: false,
  hover: {
    pointer: new THREE.Vector2(),
    hasPointer: false,
    cameraPosition: new THREE.Vector3(),
    cameraQuaternion: new THREE.Quaternion(),
    candidates: [],
    best: null,
    hitTile: null,
    swept: false,
  },
};

// init boots the viewer for the /api/config payload of the navmesh
// mode. It builds the full screen surface, loads the tile geometries
// of the initial selection and wires the double click search. The
// view state link of the copy button overrides the boot: its query
// parameters restore the camera pose, the tile selection, the
// filter, the height scale and the route pair - and the route runs
// again on its own, so the pasted link answers itself.
function init(config) {
  const navmesh = config.navmesh;
  const view = parseViewParams();
  buildSurface(navmesh);

  if (!navmesh || !navmesh.tiles || navmesh.tiles.length === 0) {
    showStatus("no tiles", "no navmesh tiles in " + (navmesh ? navmesh.dir : "") +
      " - build them with cmd/navmesh-build first");
    return;
  }

  let initial = navmesh.initialTiles && navmesh.initialTiles.length > 0
    ? navmesh.initialTiles
    : navmesh.tiles;
  if (view.tiles) {
    const known = new Set(navmesh.tiles.map(tileKey));
    const wanted = view.tiles.filter((key) => known.has(key));
    if (wanted.length > 0) {
      const wantedSet = new Set(wanted);
      initial = navmesh.tiles.filter((tile) => wantedSet.has(tileKey(tile)));
    }
  }
  const initialKeys = new Set(initial.map(tileKey));
  for (const tile of navmesh.tiles) {
    addTileRow(tile, initialKeys.has(tileKey(tile)));
  }
  syncAllBox();
  if (view.filter) {
    viewer.filter = view.filter;
    const select = document.getElementById("nmv-filter");
    if (select) {
      select.value = view.filter;
    }
  }
  if (view.scale) {
    setHeightScale(view.scale);
    const select = document.getElementById("nmv-height");
    if (select) {
      select.value = String(view.scale);
    }
  }
  if (view.cam) {
    applyCameraState(view.cam);
  } else {
    frameInitialTiles(initial);
  }
  for (const tile of initial) {
    void ensureTile(tile);
  }
  if (view.from && view.to) {
    viewer.routeStart = view.from;
    viewer.routeEnd = view.to;
    drawStartMarker(view.from);
    drawEndMarker(view.to);
    void requestRoute(view.from, view.to);
  } else if (view.from) {
    viewer.pendingStart = view.from;
    drawStartMarker(view.from);
    showStatus("start set", "double click the destination");
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
    <div class="nmv-cursor" id="nmv-cursor">
      <span class="nmv-cursor-tile" id="nmv-cursor-tile">&mdash;</span>
      <span id="nmv-cursor-pos">move the pointer over the mesh</span>
    </div>
    <div class="nmv-panel nmv-side">
      <div class="nmv-title">swarm &middot; navmesh viewer</div>
      <div class="nmv-sub" id="nmv-dir"></div>
      <div class="nmv-section">flight</div>
      <div class="nmv-row"><span>speed</span>
        <span class="nmv-tile-status" id="nmv-speed"></span></div>
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
      <div class="nmv-section">display</div>
      <label class="nmv-row"><input type="checkbox"
        id="nmv-waypoint-coords" checked>
        <span>waypoint coordinates</span></label>
      <label class="nmv-row"><input type="checkbox" id="nmv-edges">
        <span>edge connections</span></label>
      <div class="nmv-section">legend</div>
      <div class="nmv-row"><span class="nmv-swatch nmv-ground"></span>
        <span>ground (height ramp)</span></div>
      <div class="nmv-row"><span class="nmv-swatch nmv-water"></span>
        <span>water (depth ramp)</span></div>
      <div class="nmv-row"><span class="nmv-swatch nmv-conn-ground"></span>
        <span>field connections</span></div>
      <div class="nmv-row"><span class="nmv-swatch nmv-conn-water"></span>
        <span>water connections</span></div>
      <div class="nmv-row"><span class="nmv-swatch nmv-conn-shore"></span>
        <span>shore connections</span></div>
      <div class="nmv-section">share this view</div>
      <input type="text" id="nmv-link" class="nmv-link" readonly
        title="the camera, the route pair and the tile selection as one link">
      <button id="nmv-copy" class="btn nmv-copy">copy view link</button>
      <button id="nmv-clear" class="btn nmv-clear">clear route</button>
    </div>
    <div class="nmv-panel nmv-result" id="nmv-result">
      <div class="nmv-section">route search</div>
      <div class="nmv-status" id="nmv-status">double click the mesh</div>
      <div class="nmv-timer" id="nmv-timer"></div>
      <div class="nmv-timer-sub" id="nmv-timer-sub"></div>
      <div class="nmv-stats" id="nmv-stats"></div>
    </div>
    <div class="nmv-hint">wasd + q/e flies &middot; shift boosts &middot;
      drag looks &middot; wheel retunes the speed &middot; double click:
      first the start, then the destination &middot; the copy button
      shares this exact view</div>`;
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
      // The stitched world spans hundreds of thousands of units; the
      // log depth buffer keeps the stacked surfaces from z fighting
      // at every viewing distance.
      logarithmicDepthBuffer: true,
    });
  } catch (err) {
    showStatus("error", "WebGL is unavailable: " + err.message);
    return;
  }
  viewer.renderer.setPixelRatio(Math.min(window.devicePixelRatio, 2));
  viewer.renderer.setClearColor(0x0d1117, 1);

  viewer.scene = new THREE.Scene();
  viewer.camera = new THREE.PerspectiveCamera(
    60, 1, 16, 400000);
  // The balanced four light rig: the ambient floor keeps every face
  // readable (a steep quad tessellates into two triangles whose flat
  // normals face apart - without the floor the away-facing half falls
  // to black and reads as a hole, the "black triangles" of the solid
  // surface round), the hemisphere models the sky, the sun and the
  // counter fill keep the relief.
  const ambient = new THREE.AmbientLight(0xffffff, 0.42);
  viewer.scene.add(ambient);
  const hemi = new THREE.HemisphereLight(0xe8eef4, 0x8a8064, 0.6);
  viewer.scene.add(hemi);
  const sun = new THREE.DirectionalLight(0xffffff, 0.45);
  sun.position.set(0.45, 1, 0.25);
  viewer.scene.add(sun);
  const fill = new THREE.DirectionalLight(0xdfe7f0, 0.25);
  fill.position.set(-0.5, 0.4, -0.35);
  viewer.scene.add(fill);

  viewer.rig = createFlyRig(viewer.camera, canvas);
  canvas.addEventListener("dblclick", onDoubleClick);
  canvas.addEventListener("pointermove", onPointerMove);
  canvas.addEventListener("pointerleave", onPointerLeave);

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
  document.getElementById("nmv-copy").addEventListener("click", copyViewState);
  document.getElementById("nmv-waypoint-coords").addEventListener(
    "change", (e) => {
      viewer.showWaypointCoords = e.target.checked;
      if (viewer.waypointLabels) {
        viewer.waypointLabels.visible = e.target.checked;
      }
    });
  document.getElementById("nmv-edges").addEventListener("change", (e) => {
    setConnectionsVisible(e.target.checked);
  });

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

// renderLoop redraws the scene and drives the progressive cursor
// raycast: one candidate tile per frame, so the readout never blocks
// the orbit.
function renderLoop() {
  if (viewer.rig) {
    viewer.rig.update();
  }
  hoverStep();
  if (viewer.renderer) {
    viewer.renderer.render(viewer.scene, viewer.camera);
  }
  requestAnimationFrame(renderLoop);
}

// createFlyRig is the flight camera of the viewer: the pointer drag
// yaws and pitches the view, wasd flies along the full view vector
// (the airplane feel - W follows the pitch into the ground and out
// of it), q and e descend and climb, shift boosts 4x and the wheel
// retunes the cruise speed. No damping - the debug viewer prefers
// the direct response.
function createFlyRig(camera, dom) {
  // The YXZ order is fixed once here, before any framing touch:
  // the euler values then mean yaw/pitch/roll from the start.
  camera.rotation.order = "YXZ";
  const rig = {
    camera,
    yaw: Math.PI / 4,
    pitch: -0.6,
    speed: 2400,
    keys: new Set(),
    clock: new THREE.Clock(),
    update() {
      const dt = Math.min(this.clock.getDelta(), 0.1);
      // The full euler write (with the zero roll) keeps the camera
      // honest: a stale rotation.z from a lookAt elsewhere would roll
      // the horizon (the boot framing bug of the last round - the
      // euler order flip reinterpreted the lookAt angles as roll).
      camera.rotation.set(this.pitch, this.yaw, 0);
      // The movement basis: the full pitched forward vector and its
      // horizontal right wing.
      const forward = new THREE.Vector3(
        -Math.sin(this.yaw) * Math.cos(this.pitch),
        Math.sin(this.pitch),
        -Math.cos(this.yaw) * Math.cos(this.pitch));
      const right = new THREE.Vector3(-forward.z, 0, forward.x);
      const move = new THREE.Vector3();
      if (this.keys.has("KeyW") || this.keys.has("ArrowUp")) {
        move.add(forward);
      }
      if (this.keys.has("KeyS") || this.keys.has("ArrowDown")) {
        move.sub(forward);
      }
      if (this.keys.has("KeyD") || this.keys.has("ArrowRight")) {
        move.add(right);
      }
      if (this.keys.has("KeyA") || this.keys.has("ArrowLeft")) {
        move.sub(right);
      }
      if (this.keys.has("KeyE")) {
        move.y += 1;
      }
      if (this.keys.has("KeyQ")) {
        move.y -= 1;
      }
      if (move.lengthSq() > 0) {
        const boost = this.keys.has("ShiftLeft") ||
          this.keys.has("ShiftRight") ? 4 : 1;
        camera.position.addScaledVector(move.normalize(),
          this.speed * boost * dt);
      }
    },
  };

  let dragging = false;
  let lastX = 0, lastY = 0;
  dom.addEventListener("contextmenu", (e) => e.preventDefault());
  dom.addEventListener("pointerdown", (e) => {
    if (e.button !== 0) {
      return;
    }
    dragging = true;
    lastX = e.clientX;
    lastY = e.clientY;
    dom.setPointerCapture(e.pointerId);
  });
  dom.addEventListener("pointermove", (e) => {
    if (!dragging) {
      return;
    }
    const dx = e.clientX - lastX, dy = e.clientY - lastY;
    lastX = e.clientX;
    lastY = e.clientY;
    rig.yaw -= dx * 0.0042;
    rig.pitch = Math.min(1.55, Math.max(-1.55, rig.pitch - dy * 0.0042));
  });
  const release = () => { dragging = false; };
  dom.addEventListener("pointerup", release);
  dom.addEventListener("pointercancel", release);
  dom.addEventListener("wheel", (e) => {
    e.preventDefault();
    const factor = e.deltaY > 0 ? 1.25 : 1 / 1.25;
    rig.speed = Math.min(400000, Math.max(60, rig.speed * factor));
    showFlightSpeed(rig.speed);
  }, { passive: false });

  // The keys live on the window so the canvas focus never matters;
  // the form fields keep their own keys.
  const formTag = (target) => target instanceof Element &&
    (target.tagName === "INPUT" || target.tagName === "SELECT" ||
      target.tagName === "TEXTAREA" || target.tagName === "BUTTON");
  window.addEventListener("keydown", (e) => {
    if (formTag(e.target)) {
      return;
    }
    if (FLY_KEYS.has(e.code)) {
      rig.keys.add(e.code);
      e.preventDefault();
    }
  });
  window.addEventListener("keyup", (e) => {
    rig.keys.delete(e.code);
  });
  window.addEventListener("blur", () => {
    rig.keys.clear();
  });
  showFlightSpeed(rig.speed);

  return rig;
}

// FLY_KEYS are the flight controls the rig claims (the arrows ride
// along so the viewer answers without a wasd reach).
const FLY_KEYS = new Set([
  "KeyW", "KeyA", "KeyS", "KeyD", "KeyQ", "KeyE",
  "ArrowUp", "ArrowDown", "ArrowLeft", "ArrowRight",
  "ShiftLeft", "ShiftRight",
]);

// showFlightSpeed writes the cruise speed readout of the panel.
function showFlightSpeed(speed) {
  const cell = document.getElementById("nmv-speed");
  if (cell) {
    cell.textContent = Math.round(speed).toLocaleString() + " u/s";
  }
}

// frameInitialTiles aims the flight camera at the union of the
// initially selected tiles: a three quarter orbit offset above the
// center, the cruise speed scaled to the world span.
function frameInitialTiles(tiles) {
  if (!tiles || tiles.length === 0 || !viewer.rig || !viewer.camera) {
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
  const target = new THREE.Vector3(
    (minX + maxX) / 2, 0, (minY + maxY) / 2);
  viewer.camera.position.set(
    target.x + size * 0.75,
    Math.max(2500, size * 0.7),
    target.z + size * 0.75);
  // The framing pose is computed, never taken from lookAt: a lookAt
  // under the default XYZ euler order leaves a z angle behind that
  // the YXZ rig would read as roll (the tilted horizon of the last
  // round). The analytic route writes yaw and pitch directly.
  const dirX = target.x - viewer.camera.position.x;
  const dirZ = target.z - viewer.camera.position.z;
  const flat = Math.hypot(dirX, dirZ);
  viewer.rig.yaw = Math.atan2(-dirX, -dirZ);
  viewer.rig.pitch = Math.atan2(
    target.y - viewer.camera.position.y, flat);
  viewer.camera.rotation.set(viewer.rig.pitch, viewer.rig.yaw, 0);
  viewer.rig.speed = Math.max(400, size * 0.18);
  showFlightSpeed(viewer.rig.speed);
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
    connections: null,
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
    entry.mesh = buildTileMesh(key, buffer);
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
// (the ground height ramp or the water depth ramp - both flat per
// polygon so a warped quad reads as one surface) and the quad indices
// of the two tessellations: the surface quads are row-major on the
// wire (corner 2 sits diagonal to 1), so they split along the 1-2
// anti-diagonal - (0 2 1) (1 2 3), both triangles face up in the
// viewer mapping y = world height - while the wall quads are cyclic
// (emitter lo, emitter hi, target hi, target lo) and split along the
// 0-2 diagonal - (0 2 1) (0 2 3). The tile mesh carries the region
// grid outline and the lazily built edge overlay as children, so the
// height scale and the visibility apply to them too.
function buildTileMesh(key, buffer) {
  const view = new DataView(buffer);
  if (view.getUint32(0, true) !== GEO_MAGIC) {
    throw new Error("bad geometry magic");
  }
  const worldMinX = view.getInt32(8, true);
  const worldMinY = view.getInt32(12, true);
  const minH = view.getInt16(16, true);
  const maxH = view.getInt16(18, true);
  const polyCount = view.getUint32(20, true);
  const linkCount = view.getUint32(24, true);
  const wallCount = view.getUint32(28, true);
  const corners = new Int16Array(buffer, GEO_HEADER, polyCount * 12);
  const areas = new Uint8Array(
    buffer, GEO_HEADER + polyCount * 24, polyCount);
  // The link portal block follows the padded area tail; the wall
  // block closes the payload. Every record is sixteen bytes and the
  // blocks stay four byte aligned, so the u16 views read them whole.
  const linksBase = align4(GEO_HEADER + polyCount * 25);
  const wallsBase = linksBase + linkCount * 16;
  const links = new Uint16Array(buffer, linksBase, linkCount * 8);
  const walls = new Uint16Array(buffer, wallsBase, wallCount * 8);
  const wallMeta = new Uint8Array(buffer, wallsBase, wallCount * 16);

  const quadCount = polyCount + wallCount;
  const positions = new Float32Array(quadCount * 12);
  const colors = new Uint8Array(quadCount * 12);
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
    }
    // One flat color per polygon reads better than per corner
    // gradients over a warped quad: the mean corner height drives the
    // ramp (the ground height ramp or the water depth ramp).
    writeQuadColor(colors, poly,
      surfaceRamp(water, sumH / 4, minH, span));
  }

  // The height step walls: vertical filler quads along the shared
  // edges of adjacent rectangles. The record carries the fixed edge
  // coordinate and the span bounds as tile local world offsets, plus
  // the two surface height profiles at the span ends.
  for (let wall = 0; wall < wallCount; wall++) {
    const src = wall * 8;
    const meta = wall * 16;
    const horizontal = wallMeta[meta + 15] === 1;
    const fixed = (horizontal ? worldMinY : worldMinX) + walls[src];
    const lo = (horizontal ? worldMinX : worldMinY) + walls[src + 1];
    const hi = (horizontal ? worldMinX : worldMinY) + walls[src + 2];
    const hA0 = signedShort(walls[src + 3]);
    const hA1 = signedShort(walls[src + 4]);
    const hB0 = signedShort(walls[src + 5]);
    const hB1 = signedShort(walls[src + 6]);
    const water = wallMeta[meta + 14] === 1;
    const base = (polyCount + wall) * 12;
    // Corner order: the emitter profile at lo and hi, then the
    // target profile back through hi and lo - one closed quad.
    writeWallCorner(positions, base, horizontal, fixed, lo, hA0);
    writeWallCorner(positions, base + 3, horizontal, fixed, hi, hA1);
    writeWallCorner(positions, base + 6, horizontal, fixed, hi, hB1);
    writeWallCorner(positions, base + 9, horizontal, fixed, lo, hB0);
    writeWallColor(colors, polyCount + wall, water,
      hA0, hA1, hB0, hB1, minH, span);
  }

  // The surface tessellation: the wire corners are row-major (0 is
  // the min corner, 1 the +x one, 2 the +y one, 3 the opposite), so
  // the two triangles split along the 1-2 anti-diagonal: (0,2,1)
  // covers the lower-left half, (1,2,3) the upper-right one. The
  // previous (0,2,1)+(0,2,3) pair anchored both triangles on the
  // shared 0-2 edge and left the right quarter of every polygon
  // unpainted - the see-through triangles of the owner reports, one
  // missing wedge per rectangle scaling with its size.
  const indices = new Uint32Array(quadCount * 6);
  for (let poly = 0; poly < polyCount; poly++) {
    const base = poly * 4;
    const at = poly * 6;
    indices[at] = base;
    indices[at + 1] = base + 2;
    indices[at + 2] = base + 1;
    indices[at + 3] = base + 1;
    indices[at + 4] = base + 2;
    indices[at + 5] = base + 3;
  }
  // The wall quads carry their corners in cyclic order (emitter lo,
  // emitter hi, target hi, target lo), so the 0-2 diagonal split
  // covers the full quad.
  for (let wall = 0; wall < wallCount; wall++) {
    const base = (polyCount + wall) * 4;
    const at = (polyCount + wall) * 6;
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
    // The surface steps back a hair so the lifted connection overlay
    // of the edges toggle wins the depth test.
    polygonOffset: true,
    polygonOffsetFactor: 1,
    polygonOffsetUnits: 1,
  });
  const mesh = new THREE.Mesh(geometry, material);
  mesh.userData.polys = polyCount;
  mesh.userData.walls = wallCount;
  mesh.userData.key = key;
  mesh.userData.links = {
    view: links,
    meta: new Uint8Array(buffer, linksBase, linkCount * 16),
    count: linkCount,
    worldMinX,
    worldMinY,
  };
  mesh.scale.y = viewer.heightScale;
  mesh.add(buildTileOutline(worldMinX, worldMinY, maxH));

  return mesh;
}

// align4 rounds a byte offset up to the four byte block boundary.
function align4(offset) {
  return (offset + 3) & ~3;
}

// signedShort reads one two's complement height out of the raw u16
// view of a record block.
function signedShort(value) {
  return value > 32767 ? value - 65536 : value;
}

// surfaceRamp colors one surface height: the ground height ramp or
// the water depth ramp of the legend.
function surfaceRamp(water, height, minH, span) {
  const t = water
    ? Math.min(1, Math.max(0, (WATER_LEVEL - height) / WATER_DEEP_RANGE))
    : Math.min(1, Math.max(0, (height - minH) / span));
  const low = water ? WATER_SHALLOW : GROUND_LOW;
  const high = water ? WATER_DEEP : GROUND_HIGH;

  return [
    low.r + (high.r - low.r) * t,
    low.g + (high.g - low.g) * t,
    low.b + (high.b - low.b) * t,
  ];
}

// writeQuadColor paints all four corners of one surface quad with
// one flat color (0..255 channels).
function writeQuadColor(colors, quad, color) {
  for (let corner = 0; corner < 4; corner++) {
    const dst = (quad * 4 + corner) * 3;
    colors[dst] = color[0];
    colors[dst + 1] = color[1];
    colors[dst + 2] = color[2];
  }
}

// writeWallCorner places one wall corner: the fixed edge coordinate,
// the position along the span and the height of the owning profile.
function writeWallCorner(positions, dst, horizontal, fixed, along, height) {
  if (horizontal) {
    positions[dst] = along;
    positions[dst + 1] = height;
    positions[dst + 2] = fixed;
  } else {
    positions[dst] = fixed;
    positions[dst + 1] = height;
    positions[dst + 2] = along;
  }
}

// writeWallColor paints one wall quad with the surface ramp of its
// area at the profile heights, darkened so the filler reads as a
// shaded cliff face instead of navigable surface.
function writeWallColor(colors, quad, water, hA0, hA1, hB0, hB1,
  minH, span,
) {
  const heights = [hA0, hA1, hB1, hB0];
  for (let corner = 0; corner < 4; corner++) {
    const color = surfaceRamp(water, heights[corner], minH, span);
    const dst = (quad * 4 + corner) * 3;
    colors[dst] = color[0] * WALL_SHADE;
    colors[dst + 1] = color[1] * WALL_SHADE;
    colors[dst + 2] = color[2] * WALL_SHADE;
  }
}

// buildTileOutline draws the region grid rectangle of one tile above
// its highest geometry, the persistent answer to "which square am I
// looking at".
function buildTileOutline(worldMinX, worldMinY, maxH) {
  const size = 32768;
  const lift = maxH + 24;
  const points = [
    worldMinX, lift, worldMinY,
    worldMinX + size, lift, worldMinY,
    worldMinX + size, lift, worldMinY + size,
    worldMinX, lift, worldMinY + size,
    worldMinX, lift, worldMinY,
  ];
  const geometry = new THREE.BufferGeometry();
  geometry.setAttribute("position",
    new THREE.BufferAttribute(new Float32Array(points), 3));
  const outline = new THREE.Line(geometry, new THREE.LineBasicMaterial({
    color: TILE_OUTLINE_COLOR,
    transparent: true,
    opacity: 0.8,
  }));
  outline.userData.outline = true;

  return outline;
}

// buildTileConnections builds the edge connections overlay of one
// tile: every link portal of the mesh as its open world span, one
// line segment per connection, colored by the area pair of the
// polygons it joins - green for the field to field connections, blue
// for the water to water ones, teal for the shore pairs. The spans
// are the honest answer of "where may a route cross this edge": the
// NSWE walls of the geodata clip them to the open cell runs, so a
// long shared edge between two rectangles shows its gates, not its
// full length.
function buildTileConnections(mesh) {
  const links = mesh.userData.links;
  if (!links || links.count === 0) {
    return null;
  }
  const positions = new Float32Array(links.count * 6);
  const colors = new Float32Array(links.count * 6);
  for (let i = 0; i < links.count; i++) {
    const src = i * 8;
    const record = i * 16;
    positions[i * 6] = links.worldMinX + links.view[src];
    positions[i * 6 + 1] =
      signedShort(links.view[src + 2]) + CONNECTION_LIFT;
    positions[i * 6 + 2] = links.worldMinY + links.view[src + 1];
    positions[i * 6 + 3] = links.worldMinX + links.view[src + 3];
    positions[i * 6 + 4] =
      signedShort(links.view[src + 5]) + CONNECTION_LIFT;
    positions[i * 6 + 5] = links.worldMinY + links.view[src + 4];
    const color = CONNECTION_CLASSES[links.meta[record + 12]] ||
      CONNECTION_CLASSES[3];
    colors[i * 6] = color[0];
    colors[i * 6 + 1] = color[1];
    colors[i * 6 + 2] = color[2];
    colors[i * 6 + 3] = color[0];
    colors[i * 6 + 4] = color[1];
    colors[i * 6 + 5] = color[2];
  }
  const geometry = new THREE.BufferGeometry();
  geometry.setAttribute("position",
    new THREE.BufferAttribute(positions, 3));
  geometry.setAttribute("color",
    new THREE.BufferAttribute(colors, 3));
  const overlay = new THREE.LineSegments(geometry,
    new THREE.LineBasicMaterial({
      vertexColors: true,
      transparent: true,
      opacity: 0.95,
    }));

  return overlay;
}

// setConnectionsVisible toggles the edge connections overlay of every
// loaded tile, building it lazily on the first enable.
function setConnectionsVisible(visible) {
  viewer.showEdges = visible;
  for (const entry of viewer.tiles.values()) {
    if (!entry.mesh) {
      continue;
    }
    if (visible && !entry.connections) {
      entry.connections = buildTileConnections(entry.mesh);
      if (entry.connections) {
        entry.mesh.add(entry.connections);
      }
    }
    if (entry.connections) {
      entry.connections.visible = visible;
    }
  }
}

// onPointerMove arms the progressive cursor raycast.
function onPointerMove(event) {
  if (!viewer.renderer) {
    return;
  }
  const rect = event.target.getBoundingClientRect();
  viewer.hover.pointer.set(
    ((event.clientX - rect.left) / rect.width) * 2 - 1,
    -((event.clientY - rect.top) / rect.height) * 2 + 1);
  viewer.hover.hasPointer = true;
  restartHoverSweep();
}

// onPointerLeave clears the cursor readout.
function onPointerLeave() {
  viewer.hover.hasPointer = false;
  viewer.hover.candidates = [];
  viewer.hover.best = null;
  viewer.hover.swept = false;
  setHoverTile(null);
  clearCursorReadout();
}

// clearCursorReadout resets the readout bar to the idle text.
function clearCursorReadout() {
  setHoverTile(null);
  const tile = document.getElementById("nmv-cursor-tile");
  const pos = document.getElementById("nmv-cursor-pos");
  if (tile) {
    tile.innerHTML = "&mdash;";
  }
  if (pos) {
    pos.textContent = "pointer over no tile";
  }
}

// restartHoverSweep re-runs the sphere pass: the visible tiles whose
// bounding sphere the cursor ray enters become the candidates,
// ordered by the entry distance so the readout is right after the
// first frames.
function restartHoverSweep() {
  const hover = viewer.hover;
  hover.candidates = [];
  hover.best = null;
  if (!viewer.camera || !hover.hasPointer) {
    return;
  }
  const raycaster = new THREE.Raycaster();
  raycaster.setFromCamera(hover.pointer, viewer.camera);
  const ray = raycaster.ray;
  const sphere = new THREE.Sphere();
  const entries = [];
  for (const entry of viewer.tiles.values()) {
    if (!entry.mesh || !entry.mesh.visible) {
      continue;
    }
    if (!entry.mesh.geometry.boundingSphere) {
      entry.mesh.geometry.computeBoundingSphere();
    }
    sphere.copy(entry.mesh.geometry.boundingSphere);
    sphere.applyMatrix4(entry.mesh.matrixWorld);
    const point = new THREE.Vector3();
    if (ray.intersectSphere(sphere, point)) {
      entries.push({
        mesh: entry.mesh,
        distance: ray.origin.distanceTo(point),
      });
    }
  }
  entries.sort((a, b) => a.distance - b.distance);
  hover.candidates = entries.map((e) => e.mesh);
  hover.swept = true;
}

// hoverStep advances the progressive raycast: one candidate tile per
// frame, the readout updating with the best hit so far. A camera move
// (orbit, pan, zoom) restarts the sweep so the answer tracks what the
// eye actually sees once it settles. A drained sweep without a hit
// clears the readout - the pointer rests over the tile gaps of the
// stitched world.
function hoverStep() {
  const hover = viewer.hover;
  if (!viewer.camera || !hover.hasPointer) {
    return;
  }
  const cameraMoved =
    !viewer.camera.position.equals(hover.cameraPosition) ||
    !viewer.camera.quaternion.equals(hover.cameraQuaternion);
  if (cameraMoved) {
    hover.cameraPosition.copy(viewer.camera.position);
    hover.cameraQuaternion.copy(viewer.camera.quaternion);
    restartHoverSweep();
    return;
  }
  if (hover.candidates.length === 0) {
    if (hover.swept && !hover.best) {
      hover.swept = false;
      clearCursorReadout();
    }
    return;
  }
  const mesh = hover.candidates.shift();
  const raycaster = new THREE.Raycaster();
  raycaster.setFromCamera(hover.pointer, viewer.camera);
  const hits = raycaster.intersectObject(mesh, false);
  if (hits.length > 0 &&
    (!hover.best || hits[0].distance < hover.best.distance)) {
    hover.best = hits[0];
    hover.hitTile = mesh.userData.key;
    updateCursorReadout(hover.best, mesh.userData.key);
    setHoverTile(mesh.userData.key);
  }
}

// updateCursorReadout writes the tile square, the region local cell
// and the world coordinates of the point under the pointer.
function updateCursorReadout(hit, key) {
  const tile = document.getElementById("nmv-cursor-tile");
  const pos = document.getElementById("nmv-cursor-pos");
  if (!tile || !pos) {
    return;
  }
  const worldX = hit.point.x;
  const worldY = hit.point.z;
  const worldZ = hit.point.y / viewer.heightScale;
  const localCellX = Math.floor(
    (worldX - Math.floor(worldX / 32768) * 32768) / CELL_SIZE);
  const localCellY = Math.floor(
    (worldY - Math.floor(worldY / 32768) * 32768) / CELL_SIZE);
  tile.textContent = key;
  pos.textContent =
    "cell " + localCellX + " " + localCellY +
    "  \u00b7  x " + Math.round(worldX) +
    "  y " + Math.round(worldY) +
    "  z " + Math.round(worldZ);
}

// setHoverTile lights the region outline of the hovered tile and dims
// the previous one.
function setHoverTile(key) {
  if (viewer.hover.tileKey === key) {
    return;
  }
  if (viewer.hover.tileKey) {
    const previous = viewer.tiles.get(viewer.hover.tileKey);
    if (previous && previous.mesh) {
      const outline = previous.mesh.children.find(
        (child) => child.userData.outline);
      if (outline) {
        outline.material.color.setHex(TILE_OUTLINE_COLOR);
      }
    }
  }
  viewer.hover.tileKey = key;
  if (key) {
    const entry = viewer.tiles.get(key);
    if (entry && entry.mesh) {
      const outline = entry.mesh.children.find(
        (child) => child.userData.outline);
      if (outline) {
        outline.material.color.setHex(TILE_HOVER_COLOR);
      }
    }
  }
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
  viewer.routeStart = start;
  viewer.routeEnd = end;
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
// panel: the status, the measured construction time, the search
// statistics and the from/to coordinates of the clicked pair.
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
  const rows = [];
  if (viewer.routeStart && viewer.routeEnd) {
    rows.push(["from", formatCoordRow(viewer.routeStart)]);
    rows.push(["to", formatCoordRow(viewer.routeEnd)]);
    rows.push(["spacer", ""]);
  }
  rows.push(
    ["waypoints", String(waypoints.length)],
    ["corridor polys", String(answer.corridor)],
    ["explored polys", String(answer.explored)],
    ["path length", Math.round(routeLength(waypoints)).toLocaleString() +
      " units"],
    ["filter", answer.filter]);
  stats.innerHTML = rows.map(([name, value]) =>
    name === "spacer"
      ? `<div class="nmv-stat nmv-stat-gap"></div>`
      : `<div class="nmv-stat"><span>${name}</span><b>${value}</b></div>`)
    .join("");
}

// formatCoordRow renders one clicked endpoint with its tile square.
function formatCoordRow(point) {
  const key = String(Math.floor(point.x / 32768) + 20) + "_" +
    String(Math.floor(point.y / 32768) + 18);

  return key + " &middot; " + Math.round(point.x) + ", " +
    Math.round(point.y) + ", " + Math.round(point.z);
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

// drawRouteOverlays rebuilds the path line, the waypoint dots, the
// coordinate labels and the endpoint markers from the last answer
// (the height scale change re-renders through it).
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

  const labels = new THREE.Group();
  labels.visible = viewer.showWaypointCoords;
  for (let i = 0; i < waypoints.length; i++) {
    labels.add(makeWaypointLabel(i, waypoints.length, waypoints[i]));
  }
  viewer.scene.add(labels);
  viewer.waypointLabels = labels;

  drawStartMarker(waypoints[0]);
  drawEndMarker(waypoints[waypoints.length - 1]);
}

// makeWaypointLabel builds one screen fixed sprite carrying the
// waypoint index and its world coordinates.
function makeWaypointLabel(index, total, waypoint) {
  const text = (index + 1) + "/" + total + "  " +
    Math.round(waypoint.x) + ", " + Math.round(waypoint.y) + ", " +
    Math.round(waypoint.z);
  const scale = 2;
  const canvas = document.createElement("canvas");
  let ctx = canvas.getContext("2d");
  const font = "26px ui-monospace, monospace";
  ctx.font = font;
  const width = Math.ceil(ctx.measureText(text).width) + 28;
  canvas.width = width * scale;
  canvas.height = 44 * scale;
  ctx = canvas.getContext("2d");
  ctx.scale(scale, scale);
  ctx.font = font;
  ctx.fillStyle = "rgba(13, 17, 23, 0.78)";
  ctx.fillRect(0, 0, width, 44);
  ctx.strokeStyle = "rgba(240, 246, 252, 0.25)";
  ctx.strokeRect(0.5, 0.5, width - 1, 43);
  ctx.fillStyle = index === 0
    ? "#3fd66a"
    : index === total - 1 ? "#ff5c5c" : "#e6edf3";
  ctx.textBaseline = "middle";
  ctx.fillText(text, 14, 22);

  const texture = new THREE.CanvasTexture(canvas);
  texture.minFilter = THREE.LinearFilter;
  const sprite = new THREE.Sprite(new THREE.SpriteMaterial({
    map: texture,
    sizeAttenuation: false,
    depthTest: false,
    transparent: true,
  }));
  sprite.renderOrder = 12;
  // The screen fixed scale: the label height occupies ~4.4% of the
  // viewport height at any zoom.
  const heightFraction = 0.044;
  sprite.scale.set(
    heightFraction * (width / 44), heightFraction, 1);
  sprite.position.set(
    waypoint.x,
    waypoint.z * viewer.heightScale + viewer.markerRadius * 1.6,
    waypoint.y);

  return sprite;
}

// drawStartMarker places the green start sphere.
function drawStartMarker(world) {
  if (viewer.startMarker) {
    viewer.scene.remove(viewer.startMarker);
    viewer.startMarker = null;
  }
  viewer.startMarker = makeMarker(START_COLOR, true);
  placeMarker(viewer.startMarker, world);
}

// drawEndMarker places the red destination sphere.
function drawEndMarker(world) {
  if (viewer.endMarker) {
    viewer.scene.remove(viewer.endMarker);
    viewer.endMarker = null;
  }
  viewer.endMarker = makeMarker(END_COLOR, false);
  placeMarker(viewer.endMarker, world);
}

// makeMarker builds one screen fixed endpoint dot (a sprite: the
// from/to endpoints stay visible at every zoom - the world scaled
// sphere of the first round vanished into sub pixels at the world
// view) with a ring for the start and a solid disc for the end.
function makeMarker(color, ring) {
  const scale = 4;
  const canvas = document.createElement("canvas");
  canvas.width = 32 * scale;
  canvas.height = 32 * scale;
  const ctx = canvas.getContext("2d");
  ctx.scale(scale, scale);
  const gradient = ctx.createRadialGradient(16, 16, 2, 16, 16, 15);
  gradient.addColorStop(0, "rgba(255,255,255,0.95)");
  gradient.addColorStop(0.25, "rgba(255,255,255,0.0)");
  gradient.addColorStop(1, "rgba(255,255,255,0)");
  ctx.fillStyle = gradient;
  ctx.fillRect(0, 0, 32, 32);
  ctx.lineWidth = 3.2;
  ctx.strokeStyle = "#" + color.toString(16).padStart(6, "0");
  ctx.beginPath();
  ctx.arc(16, 16, 12, 0, Math.PI * 2);
  if (ring) {
    ctx.stroke();
  } else {
    ctx.fillStyle = ctx.strokeStyle;
    ctx.fill();
    ctx.stroke();
  }
  const texture = new THREE.CanvasTexture(canvas);
  texture.minFilter = THREE.LinearFilter;
  const sprite = new THREE.Sprite(new THREE.SpriteMaterial({
    map: texture,
    sizeAttenuation: false,
    depthTest: false,
    transparent: true,
  }));
  sprite.renderOrder = 11;
  // The screen fixed size: ~5% of the viewport height.
  sprite.scale.set(0.05, 0.05, 1);

  return sprite;
}

// placeMarker positions one endpoint dot over the world position.
function placeMarker(marker, world) {
  marker.position.set(
    world.x, world.z * viewer.heightScale, world.y);
  viewer.scene.add(marker);
}

// clearRoute drops the whole search state (the button and every new
// first click run through it).
function clearRoute() {
  viewer.pendingStart = null;
  viewer.path = null;
  viewer.routeStart = null;
  viewer.routeEnd = null;
  clearRouteOverlays();
  showStatus("idle", "double click the mesh");
  document.getElementById("nmv-timer").textContent = "";
  document.getElementById("nmv-timer-sub").textContent = "";
  document.getElementById("nmv-stats").innerHTML = "";
}

// clearRouteOverlays removes the line, the dots, the labels and the
// markers.
function clearRouteOverlays() {
  for (const key of ["routeLine", "routeDots", "waypointLabels",
    "startMarker", "endMarker"]) {
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

// The view state link is the feedback channel of the viewer (the
// owner request: reproduce the exact view locally and let the route
// answer itself). The query parameters:
//   cam=x,y,z,yaw,pitch  the flight camera over the GAME WORLD axes
//                         (x, y horizontal, z height - the same
//                         triple the cursor readout shows)
//   from=x,y,z           the route start (world axes)
//   to=x,y,z             the route destination
//   tiles=21_19,22_19    the visible tile selection (omitted = all)
//   filter=swim|dry      the route filter
//   scale=1|2|4          the height exaggeration
// The world axes mapping matters: the viewer renders three y as the
// height, so a pasted link reads the same at every height scale (the
// restore re-applies the scale of the link itself).

// parseViewParams reads the boot state off the page URL.
function parseViewParams() {
  const search = new URLSearchParams(window.location.search);
  const view = {
    cam: parseCameraParam(search.get("cam")),
    from: parsePointParam(search.get("from")),
    to: parsePointParam(search.get("to")),
    tiles: null,
    filter: null,
    scale: null,
  };
  const tiles = search.get("tiles");
  if (tiles) {
    view.tiles = tiles.split(",")
      .map((key) => key.trim()).filter((key) => key !== "");
  }
  const filter = search.get("filter");
  if (filter === "swim" || filter === "dry") {
    view.filter = filter;
  }
  const scale = Number(search.get("scale"));
  if (scale === 1 || scale === 2 || scale === 4) {
    view.scale = scale;
  }

  return view;
}

// parsePointParam reads one world x,y,z triple.
function parsePointParam(text) {
  if (!text) {
    return null;
  }
  const parts = text.split(",").map(Number);
  if (parts.length !== 3 || parts.some((n) => !Number.isFinite(n))) {
    return null;
  }

  return { x: parts[0], y: parts[1], z: parts[2] };
}

// parseCameraParam reads the camera pose (the world position plus
// the yaw and the pitch).
function parseCameraParam(text) {
  if (!text) {
    return null;
  }
  const parts = text.split(",").map(Number);
  if (parts.length !== 5 || parts.some((n) => !Number.isFinite(n))) {
    return null;
  }

  return {
    x: parts[0], y: parts[1], z: parts[2],
    yaw: parts[3], pitch: parts[4],
  };
}

// applyCameraState restores the flight camera from the link pose.
function applyCameraState(cam) {
  if (!viewer.rig || !viewer.camera) {
    return;
  }
  viewer.camera.position.set(
    cam.x, cam.z * viewer.heightScale, cam.y);
  viewer.rig.yaw = cam.yaw;
  viewer.rig.pitch = cam.pitch;
}

// buildViewStateUrl freezes the current view into the link: the
// camera pose, the armed or answered route pair, the visible tiles,
// the filter and the height scale.
function buildViewStateUrl() {
  const params = new URLSearchParams();
  if (viewer.rig && viewer.camera) {
    params.set("cam", [
      Math.round(viewer.camera.position.x),
      Math.round(viewer.camera.position.z),
      Math.round(viewer.camera.position.y / viewer.heightScale),
      viewer.rig.yaw.toFixed(4),
      viewer.rig.pitch.toFixed(4),
    ].join(","));
  }
  const start = viewer.routeStart || viewer.pendingStart;
  if (start) {
    params.set("from", formatPointParam(start));
  }
  if (viewer.routeEnd) {
    params.set("to", formatPointParam(viewer.routeEnd));
  }
  const visible = [];
  for (const [key, entry] of viewer.tiles) {
    if (entry.visible) {
      visible.push(key);
    }
  }
  if (visible.length > 0 && visible.length < viewer.tiles.size) {
    params.set("tiles", visible.join(","));
  }
  params.set("filter", viewer.filter);
  params.set("scale", String(viewer.heightScale));

  return window.location.origin + window.location.pathname + "?" +
    params.toString();
}

// formatPointParam renders one world triple for the link.
function formatPointParam(point) {
  return Math.round(point.x) + "," + Math.round(point.y) + "," +
    Math.round(point.z);
}

// copyViewState fills the link field and copies it to the clipboard;
// the legacy selection fallback keeps the headless and plain http
// environments working (the async clipboard API needs a secure
// context, the sandbox browsers often are not one).
async function copyViewState() {
  const url = buildViewStateUrl();
  const field = document.getElementById("nmv-link");
  if (field) {
    field.value = url;
  }
  const button = document.getElementById("nmv-copy");
  let copied = false;
  try {
    await navigator.clipboard.writeText(url);
    copied = true;
  } catch (err) {
    if (field) {
      field.focus();
      field.select();
      copied = document.execCommand("copy");
    }
  }
  if (button) {
    const label = button.textContent;
    button.textContent = copied ? "copied" : "select and copy";
    window.setTimeout(() => {
      button.textContent = label;
    }, 1600);
  }
}

// window.__nmv exposes the viewer state for the browser console and
// the acceptance probes of the round (read only).
if (typeof window !== "undefined") {
  window.__nmv = viewer;
}

export { init };
