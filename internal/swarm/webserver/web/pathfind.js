/*

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT

*/

// PathfindUI drives the bot less pathfind test mode: draggable A and B
// markers on the map, a path request per change and the search
// statistics of the sidebar panel. The map camera and the drawing run
// through the regular MapView, the mode only feeds it markers and a
// result overlay.
const PathfindUI = {
  config: null,
  requestTimer: null,
  requestSeq: 0,

  init(config) {
    this.config = config;
    const geo = config.geodata || {};
    const defaults = config.defaults || {};
    const center = defaults.center || geo.center || { x: 0, y: 0 };
    MapView.scale = defaults.scale || 0.06;
    MapView.panAnchor = { x: center.x, y: center.y };
    MapView.enablePathfind({
      start: { x: center.x - 1500, y: center.y - 1000 },
      end: { x: center.x + 1500, y: center.y + 1000 },
      onMarkersChanged: (immediate) => this.markersChanged(immediate),
      onPlaceModeChanged: (key) => this.syncPlaceButtons(key)
    });

    const dirText = geo.dir
      ? geo.dir + " · " + geo.regionFiles + " region files"
      : "not configured, pass -geodata";
    document.getElementById("pf-geodata").textContent = "geodata: " + dirText;

    document.getElementById("set-a")
      .addEventListener("click", () => this.armPlace("start"));
    document.getElementById("set-b")
      .addEventListener("click", () => this.armPlace("end"));
    document.getElementById("show-raw")
      .addEventListener("change", () => MapView.draw());
    // The geodata visualization mode: switching swaps the tile urls,
    // the tile cache is keyed by mode so both stay warm.
    MapView.pathfind.geoMode =
      document.getElementById("geo-mode").value || "height";
    document.getElementById("geo-mode")
      .addEventListener("change", (event) => {
        MapView.pathfind.geoMode = event.target.value;
        MapView.draw();
      });

    document.getElementById("foot-status").textContent =
      "mode: pathfind test";
    this.updateMarkerLabels();
    this.markersChanged(true);
  },

  // armPlace toggles the click to place mode of one marker: the next
  // map click moves it instead of panning.
  armPlace(key) {
    MapView.setPlaceMode(MapView.pathfind.placeMode === key ? null : key);
  },

  // syncPlaceButtons mirrors the armed placement on the set buttons.
  syncPlaceButtons(key) {
    document.getElementById("set-a")
      .classList.toggle("armed", key === "start");
    document.getElementById("set-b")
      .classList.toggle("armed", key === "end");
  },

  // markersChanged reacts to a marker move: immediate after a drop or a
  // placement, debounced while the marker is dragged.
  markersChanged(immediate) {
    this.updateMarkerLabels();
    if (this.requestTimer) {
      clearTimeout(this.requestTimer);
      this.requestTimer = null;
    }
    if (immediate) {
      this.requestPath();

      return;
    }
    this.requestTimer = setTimeout(() => {
      this.requestTimer = null;
      this.requestPath();
    }, 250);
  },

  updateMarkerLabels() {
    const format = (marker) => Math.round(marker.x).toLocaleString("en-US")
      + ", " + Math.round(marker.y).toLocaleString("en-US");
    document.getElementById("pf-a").textContent =
      format(MapView.pathfind.start);
    document.getElementById("pf-b").textContent =
      format(MapView.pathfind.end);
  },

  // requestPath asks the server for the path between the markers. Out of
  // order replies are dropped through a sequence counter so a slow
  // search never overwrites a newer one.
  async requestPath() {
    const pf = MapView.pathfind;
    const status = document.getElementById("pf-status");
    const seq = ++this.requestSeq;
    status.textContent = "searching…";
    status.className = "pf-status pending";
    try {
      const response = await fetch("/api/pathfind", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          start: { x: pf.start.x, y: pf.start.y },
          end: { x: pf.end.x, y: pf.end.y }
        })
      });
      if (!response.ok) {
        throw new Error("http " + response.status);
      }
      const result = await response.json();
      if (seq !== this.requestSeq) {
        return;
      }
      pf.result = result;
      if (this.config.geodata) {
        this.config.geodata.loadedRegions = result.regionsLoaded;
      }
      this.renderStats(result);
    } catch (err) {
      if (seq !== this.requestSeq) {
        return;
      }
      status.textContent = "request failed: " + err.message;
      status.className = "pf-status error";
    }
    MapView.draw();
  },

  renderStats(result) {
    const status = document.getElementById("pf-status");
    if (result.error) {
      status.textContent = result.error;
      status.className = "pf-status error";
    } else if (!result.found) {
      status.textContent = result.aborted
        ? "search aborted: too many nodes explored"
        : "no path found";
      status.className = "pf-status error";
    } else {
      status.textContent = "path found";
      status.className = "pf-status ok";
    }
    const number = (value) => Number(value).toLocaleString("en-US");
    document.getElementById("pf-time").textContent =
      result.durationMs >= 1
        ? result.durationMs.toFixed(1) + " ms"
        : Math.max(1, Math.round(result.durationMs * 1000)) + " µs";
    document.getElementById("pf-explored").textContent =
      number(result.explored);
    document.getElementById("pf-waypoints").textContent =
      number(result.waypoints.length);
    document.getElementById("pf-length").textContent =
      number(Math.round(result.length)) + " units";
    document.getElementById("pf-regions").textContent =
      number(result.regionsLoaded);
  }
};
