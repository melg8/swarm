// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
    "strconv"
)

// The Voronoi hunt mesh of the cell hunting: the static partition
// payload (the registry the hunt loops rotate through) and the live
// record of the cell the bot holds. The mesh is the SAME for every
// bot of the process (the registry installs once), so the web layer
// serves it from a dedicated endpoint keyed by the version - it
// never rides the per second live snapshot. The live record (the
// active cell, its respawn clock, its measured income) changes every
// second and stays in the snapshot.

// CellMeshView is one cell of the hunting mesh wire form: the
// registry identity, the level band, the focus (the patrol
// destination), the expected mass and the convex polygon as a flat
// counter-clockwise x,y pair list (the map draws the edges, the hit
// tests resolve the hovered cell).
type CellMeshView struct {
    ID       string  `json:"id"`
    Name     string  `json:"name"`
    MinLevel int32   `json:"minLevel"`
    MaxLevel int32   `json:"maxLevel"`
    FocusX   int32   `json:"focusX"`
    FocusY   int32   `json:"focusY"`
    Mass     int32   `json:"mass"`
    Verts    []int32 `json:"verts"`
}

// CellLiveView is the live economy record of the cell the bot hunts:
// the identity, the state ("farming" the bot works the ground,
// "moving" the bot walks to it), the respawn clock, the measured
// income, the death heat, the fleet occupancy and the kill centroid
// EMA. This is the MINIMAL live payload of the map view - the mesh
// geometry rides the version keyed endpoint.
type CellLiveView struct {
    ID       string `json:"id"`
    Name     string `json:"name"`
    State    string `json:"state"`
    MinLevel int32  `json:"minLevel"`
    MaxLevel int32  `json:"maxLevel"`
    // SpawnMass is the expected population of the cell ground.
    SpawnMass int32 `json:"spawnMass"`
    // RespawnMinSec and RespawnMaxSec bound the respawn window.
    RespawnMinSec int32 `json:"respawnMinSec"`
    RespawnMaxSec int32 `json:"respawnMaxSec"`
    // NextRespawnSec is the predicted ETA of the earliest respawn of
    // the overlay, -1 when no prediction is pending.
    NextRespawnSec int32 `json:"nextRespawnSec"`
    // AdenaPerMin is the measured income rate of the session at the
    // ground.
    AdenaPerMin float64 `json:"adenaPerMin"`
    // DeathHeat is the decayed death heat of the cell.
    DeathHeat float64 `json:"deathHeat"`
    // Occupancy is the number of hunters of the fleet holding the
    // cell right now.
    Occupancy int32 `json:"occupancy"`
    // KillX and KillY are the kill centroid EMA (the live focus the
    // walk-to-cell segment uses), 0 when unknown.
    KillX int32 `json:"killX"`
    KillY int32 `json:"killY"`
}

// SetHuntingCells installs the hunting mesh of the deployment
// together with its version: the web layer serves the encoded
// payload from the version keyed endpoint (see HuntMesh), the live
// snapshot only carries the version string so a registry change is
// detectable without paying the mesh bytes per second. A repeated
// install of the same version keeps the cached payload as is. The
// mesh survives the session resets like the zone policy: it is the
// registry, not the observed world.
func (b *Bot) SetHuntingCells(version string, cells []CellMeshView) {
    payload := appendHuntMeshJSON(nil, version, cells)
    b.mu.Lock()
    b.huntMeshVersion = version
    b.huntMeshJSON = payload
    b.mu.Unlock()
}

// SetHuntingCellLive publishes the live record of the cell the loop
// hunts. A zero view (an empty id) clears the record - the map draws
// no highlighted cell.
func (b *Bot) SetHuntingCellLive(view CellLiveView) {
    b.mu.Lock()
    b.huntCell = view
    b.mu.Unlock()
}

// HuntMesh returns the encoded hunting mesh payload and its version
// for the mesh endpoint: the cached bytes of the last
// SetHuntingCells install. An empty version means no mesh.
func (b *Bot) HuntMesh() (string, []byte) {
    b.mu.RLock()
    version, payload := b.huntMeshVersion, b.huntMeshJSON
    b.mu.RUnlock()

    return version, payload
}

// HuntCell returns the live record of the cell the loop holds (the
// zero view when none is published): the map highlight and the
// status line of the web UI read it through the snapshot, the tests
// read it directly.
func (b *Bot) HuntCell() CellLiveView {
    b.mu.RLock()
    view := b.huntCell
    b.mu.RUnlock()

    return view
}

// appendHuntMeshJSON encodes the mesh payload:
// {"version":"...","cells":[...]} - one allocation, the flat vertex
// pairs stream as numbers without intermediate objects.
func appendHuntMeshJSON(
    dst []byte, version string, cells []CellMeshView,
) []byte {
    dst = append(dst, `{"version":`...)
    dst = appendJSONString(dst, version)
    dst = append(dst, `,"cells":[`...)
    for i := range cells {
        cell := &cells[i]
        if i > 0 {
            dst = append(dst, ',')
        }
        dst = append(dst, `{"id":`...)
        dst = appendJSONString(dst, cell.ID)
        dst = append(dst, `,"name":`...)
        dst = appendJSONString(dst, cell.Name)
        dst = append(dst, `,"minLevel":`...)
        dst = strconv.AppendInt(dst, int64(cell.MinLevel), 10)
        dst = append(dst, `,"maxLevel":`...)
        dst = strconv.AppendInt(dst, int64(cell.MaxLevel), 10)
        dst = append(dst, `,"focusX":`...)
        dst = strconv.AppendInt(dst, int64(cell.FocusX), 10)
        dst = append(dst, `,"focusY":`...)
        dst = strconv.AppendInt(dst, int64(cell.FocusY), 10)
        dst = append(dst, `,"mass":`...)
        dst = strconv.AppendInt(dst, int64(cell.Mass), 10)
        dst = append(dst, `,"verts":[`...)
        for v, value := range cell.Verts {
            if v > 0 {
                dst = append(dst, ',')
            }
            dst = strconv.AppendInt(dst, int64(value), 10)
        }
        dst = append(dst, `]}`...)
    }

    return append(dst, `]}`...)
}

// appendCellLiveJSON writes the live record of the active cell. The
// zero view (no id) encodes as null: the map draws no highlight.
func appendCellLiveJSON(dst []byte, view CellLiveView) []byte {
    if view.ID == "" {
        return append(dst, `null`...)
    }
    dst = append(dst, `{"id":`...)
    dst = appendJSONString(dst, view.ID)
    dst = append(dst, `,"name":`...)
    dst = appendJSONString(dst, view.Name)
    dst = append(dst, `,"state":`...)
    dst = appendJSONString(dst, view.State)
    dst = append(dst, `,"minLevel":`...)
    dst = strconv.AppendInt(dst, int64(view.MinLevel), 10)
    dst = append(dst, `,"maxLevel":`...)
    dst = strconv.AppendInt(dst, int64(view.MaxLevel), 10)
    dst = append(dst, `,"spawnMass":`...)
    dst = strconv.AppendInt(dst, int64(view.SpawnMass), 10)
    dst = append(dst, `,"respawnMinSec":`...)
    dst = strconv.AppendInt(dst, int64(view.RespawnMinSec), 10)
    dst = append(dst, `,"respawnMaxSec":`...)
    dst = strconv.AppendInt(dst, int64(view.RespawnMaxSec), 10)
    dst = append(dst, `,"nextRespawnSec":`...)
    dst = strconv.AppendInt(dst, int64(view.NextRespawnSec), 10)
    dst = append(dst, `,"adenaPerMin":`...)
    dst = appendJSONFloat(dst, view.AdenaPerMin)
    dst = append(dst, `,"deathHeat":`...)
    dst = appendJSONFloat(dst, view.DeathHeat)
    dst = append(dst, `,"occupancy":`...)
    dst = strconv.AppendInt(dst, int64(view.Occupancy), 10)
    dst = append(dst, `,"killX":`...)
    dst = strconv.AppendInt(dst, int64(view.KillX), 10)
    dst = append(dst, `,"killY":`...)
    dst = strconv.AppendInt(dst, int64(view.KillY), 10)

    return append(dst, '}')
}

// appendHuntMeshVersionJSON writes the mesh version marker of the
// snapshot: the string the web client compares against its cached
// mesh to decide the refetch (empty when no cell-mode registry is
// installed).
func appendHuntMeshVersionJSON(dst []byte, version string) []byte {
    dst = append(dst, `,"huntMesh":`...)

    return appendJSONString(dst, version)
}
