// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
    "errors"
    "net/http"
    "strconv"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// The dual pack view endpoints (issue #59, the visual counterpart of
// the navpack-verify corpus): the second pack of NavmeshOptions
// renders side by side with the primary one, the per tile polygon
// diff highlights what moved or vanished, and the compare pathfind
// rides the regular path endpoint (the handler answers both meshes
// when the compare mesh is armed).

// navmeshDiffRect is the identity of one polygon for the tile diff:
// the rect bounds and the four corner heights - two polygons match
// only when every wire field matches.
type navmeshDiffRect struct {
    X0  int32 `json:"x0"`
    Y0  int32 `json:"y0"`
    X1  int32 `json:"x1"`
    Y1  int32 `json:"y1"`
    H00 int16 `json:"h00"`
    H10 int16 `json:"h10"`
    H01 int16 `json:"h01"`
    H11 int16 `json:"h11"`
}

// navmeshCompareDiffResponse answers one tile of the compare diff:
// the polygon counts of both packs and the rect level delta - the
// polygons the reduced pack lost (vanished) and the ones it gained
// (added). The viewer highlights the tile by these lists.
type navmeshCompareDiffResponse struct {
    Col      int16             `json:"col"`
    Row      int16             `json:"row"`
    PolysA   int               `json:"polysA"`
    PolysB   int               `json:"polysB"`
    Vanished []navmeshDiffRect `json:"vanished"`
    Added    []navmeshDiffRect `json:"added"`
}

// handleNavmeshCompareGeometry serves the binary polygon soup of one
// tile of the COMPARE pack - the same NMV2 contract as the primary
// geometry endpoint, the payload of the second mesh.
func (s *Server) handleNavmeshCompareGeometry(
    w http.ResponseWriter, r *http.Request,
) {
    if s.navmeshCompare == nil {
        http.Error(w, "the compare pack is not armed",
            http.StatusNotImplemented)

        return
    }
    key, ok := navmeshKeyOf(r.PathValue("key"))
    if !ok {
        http.Error(w, "bad tile key, expected col_row like 21_19",
            http.StatusBadRequest)

        return
    }
    payload, etag, err := s.navmeshCompareGeometry(key)
    if err != nil {
        if errors.Is(err, navmesh.ErrTileAbsent) {
            http.Error(w, "no such navmesh tile in the compare pack",
                http.StatusNotFound)

            return
        }
        http.Error(w, err.Error(), http.StatusInternalServerError)

        return
    }
    if r.Header.Get("If-None-Match") == etag {
        w.WriteHeader(http.StatusNotModified)

        return
    }
    w.Header().Set("Content-Type", "application/octet-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("ETag", etag)
    w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
    if r.Method == http.MethodHead {
        w.WriteHeader(http.StatusOK)

        return
    }
    //nolint:gosec // the binary geometry of the local tile pack, an
    // application/octet-stream answer without any html context.
    _, _ = w.Write(payload)
}

// navmeshCompareGeometry returns the cached binary geometry payload
// of one tile of the compare pack together with its ETag (the same
// cache contract as the primary geometry).
func (s *Server) navmeshCompareGeometry(key navmesh.RegionKey,
) ([]byte, string, error) {
    return s.navmeshGeometryCached(s.navmeshCompare, s.navmeshCompareGeo,
        key, "compare ")
}

// handleNavmeshCompareDiff answers the polygon diff of one tile
// between the primary and the compare pack: the counts of both sides
// and the rect level lists of the vanished and the added polygons. A
// tile absent on one side diffs against an empty set - the whole
// tile vanished (or appeared).
func (s *Server) handleNavmeshCompareDiff(
    w http.ResponseWriter, r *http.Request,
) {
    if s.navmeshCompare == nil {
        http.Error(w, "the compare pack is not armed",
            http.StatusNotImplemented)

        return
    }
    key, ok := navmeshKeyOf(r.PathValue("key"))
    if !ok {
        http.Error(w, "bad tile key, expected col_row like 21_19",
            http.StatusBadRequest)

        return
    }
    tileA, errA := s.navmeshMesh.Tile(key)
    if errA != nil && !errors.Is(errA, navmesh.ErrTileAbsent) {
        http.Error(w, errA.Error(), http.StatusInternalServerError)

        return
    }
    tileB, errB := s.navmeshCompare.Tile(key)
    if errB != nil && !errors.Is(errB, navmesh.ErrTileAbsent) {
        http.Error(w, errB.Error(), http.StatusInternalServerError)

        return
    }
    if tileA == nil && tileB == nil {
        http.Error(w, "no such navmesh tile in either pack",
            http.StatusNotFound)

        return
    }

    countA, setA := 0, map[navmeshDiffRect]int{}
    if tileA != nil {
        countA = len(tileA.Polys)
        setA = polyRectSet(tileA.Polys)
    }
    countB, setB := 0, map[navmeshDiffRect]int{}
    if tileB != nil {
        countB = len(tileB.Polys)
        setB = polyRectSet(tileB.Polys)
    }

    answer := navmeshCompareDiffResponse{
        Col:      key.Col,
        Row:      key.Row,
        PolysA:   countA,
        PolysB:   countB,
        Vanished: []navmeshDiffRect{},
        Added:    []navmeshDiffRect{},
    }
    for rect, count := range setA {
        for i := count - setB[rect]; i > 0; i-- {
            answer.Vanished = append(answer.Vanished, rect)
        }
    }
    for rect, count := range setB {
        for i := count - setA[rect]; i > 0; i-- {
            answer.Added = append(answer.Added, rect)
        }
    }
    writeJSON(w, s.logger, answer)
}

// polyRectSet counts the polygon identities of one tile: the rect
// bounds and the corner heights are the identity (the decompositions
// of the same geodata share every polygon they agree on).
func polyRectSet(polys []navmesh.Poly) map[navmeshDiffRect]int {
    set := make(map[navmeshDiffRect]int, len(polys))
    for i := range polys {
        poly := &polys[i]
        set[navmeshDiffRect{
            X0: poly.X0, Y0: poly.Y0, X1: poly.X1, Y1: poly.Y1,
            H00: poly.H00, H10: poly.H10, H01: poly.H01, H11: poly.H11,
        }]++
    }

    return set
}
