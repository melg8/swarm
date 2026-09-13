// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
)

// The view layer of the cell hunting: the STATIC mesh (the whole
// registry with its polygons, served once per version through the
// dedicated endpoint - it never rides the per second snapshot) and
// the LIVE record of the held cell (the identity, the respawn clock,
// the measured income - a couple hundred bytes per second). The map
// draws the mesh edges and highlights exactly ONE element: the cell
// the bot holds or walks to (the minimal information set of the
// user order; the rest of the map stays outline-only).

// cellLiveStateFarming marks the live record of a bot working its
// ground (inside the polygon), cellLiveStateMoving the record of a
// bot walking to its ground (outside it - the return leg or the
// fresh rotation).
const (
    cellLiveStateFarming = "farming"
    cellLiveStateMoving  = "moving"
)

// publishMesh installs the static registry payload of the tracker
// exactly once per hunter (the mesh is the same for every bot of
// the process; a relogin republishes the identical version and the
// tracker keeps its cache).
func (h *cellHunter) publishMesh(l *Loop) {
    if h.meshPublished || l.tracker == nil {
        return
    }
    h.meshPublished = true
    cells := make([]state.CellMeshView, 0, len(h.cells))
    for index := range h.cells {
        cell := &h.cells[index]
        verts := make([]int32, 0, len(cell.Vertices)*2)
        for v := range cell.Vertices {
            verts = append(verts, cell.Vertices[v].X, cell.Vertices[v].Y)
        }
        cells = append(cells, state.CellMeshView{
            ID:       cell.ID,
            Name:     cell.Name,
            MinLevel: cell.MinLevel,
            MaxLevel: cell.MaxLevel,
            FocusX:   cell.FocusX,
            FocusY:   cell.FocusY,
            Mass:     int32(cell.Mass),
            Verts:    verts,
        })
    }
    l.tracker.SetHuntingCells(huntMeshVersion, cells)
}

// publishView pushes the live record of the held cell to the tracker
// snapshot: the identity and the state (farming or moving - the map
// highlights the cell the bot is heading to), the respawn clock of
// the overlay, the measured income, the death heat and the kill
// centroid EMA. The kill ring rides along: the positions of the
// recent kills feed the fleet wide cross layer of the map (every
// kill of every bot, independent of the observed bot).
func (h *cellHunter) publishView(l *Loop, now time.Time) {
    if l.tracker == nil {
        return
    }
    if !h.viewAt.IsZero() && now.Sub(h.viewAt) < cellViewPeriod {
        return
    }
    h.viewAt = now
    h.pruneKills(now)
    h.publishMesh(l)
    if h.picked < 0 {
        //nolint:exhaustruct_v5 // the zero view clears the record
        l.tracker.SetHuntingCellLive(state.CellLiveView{})
        l.tracker.SetKillMarks(nil)

        return
    }
    cell := h.cells[h.picked]
    metric := &h.metrics[h.picked]
    next := int32(-1)
    if eta, _, _, ok := h.earliestPendingRespawn(cell.ID, now); ok {
        next = int32(eta.Seconds())
    }
    // The state marker: the character inside the polygon farms the
    // ground, outside it the return leg (or the fresh rotation)
    // walks there - the map answers "which zone is the bot going
    // to" through exactly this element.
    liveState := cellLiveStateMoving
    if h.selfKnown && h.leash.Contains(h.selfX, h.selfY) {
        liveState = cellLiveStateFarming
    }
    //nolint:exhaustruct_v5 // KillX/KillY set below
    view := state.CellLiveView{
        ID:             cell.ID,
        Name:           cell.Name,
        State:          liveState,
        MinLevel:       cell.MinLevel,
        MaxLevel:       cell.MaxLevel,
        SpawnMass:      int32(cell.Mass),
        RespawnMinSec:  cell.RespawnMin,
        RespawnMaxSec:  cell.RespawnMax,
        NextRespawnSec: next,
        AdenaPerMin:    metric.adenaPerMin(),
        DeathHeat:      h.deathHeat(metric, now),
        Occupancy:      int32(h.hub.occupancy(cell.ID)),
    }
    if metric.killPosKnown {
        view.KillX = int32(metric.killX)
        view.KillY = int32(metric.killY)
    }
    l.tracker.SetHuntingCellLive(view)

    // The kill ring: the fleet wide cross layer of the map.
    marks := make([]state.KillMarkView, 0, len(h.kills))
    for index := range h.kills {
        kill := &h.kills[index]
        //nolint:exhaustruct_v5 // BotID stays 0
        marks = append(marks, state.KillMarkView{
            X: kill.x, Y: kill.y, AtMs: kill.at.UnixMilli(),
        })
    }
    l.tracker.SetKillMarks(marks)
}
