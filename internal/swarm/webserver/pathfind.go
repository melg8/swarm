// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/melg8/swarm/internal/swarm/pathfind"
)

// maxRawPathPoints caps the raw A* cell path of the JSON response: the
// full debug path of a long route can hold tens of thousands of points
// and the browser only needs the shape.
const maxRawPathPoints = 4000

// configResponse tells the web UI which mode the server runs in.
type configResponse struct {
	Mode     string            `json:"mode"`
	Geodata  *pathfind.Stats   `json:"geodata,omitempty"`
	MaxSteps uint16            `json:"maxPassableHeight"`
	Defaults *pathfindDefaults `json:"defaults,omitempty"`
}

// pathfindDefaults carries the suggested initial map view of the
// pathfind test.
type pathfindDefaults struct {
	Center pathfind.Vec3 `json:"center"`
	Scale  float64       `json:"scale"`
}

// PathfindOptions tunes the pathfind test server.
type PathfindOptions struct {
	// ViewCenter is the initial map camera position; nil falls back to
	// the center of the available geodata. The bot passes its hunting
	// zone so the test opens on the area the bot actually walks.
	ViewCenter *pathfind.Vec3
}

// newPathfindServer creates the web server of the pathfind test mode:
// the map with an interactive path search and no bot behind it.
func NewPathfindServer(
	engine *pathfind.Engine, address string, logger *log.Logger,
	options PathfindOptions,
) *Server {
	server := newServer(address, logger)
	server.pathfinder = engine
	server.pathfindView = options.ViewCenter

	mux := server.httpServer.Handler.(*http.ServeMux)
	mux.HandleFunc("GET /api/config", server.handleConfig)
	mux.HandleFunc("POST /api/pathfind", server.handlePathfind)
	mux.HandleFunc("GET /api/geodata/tile/{level}/{name}",
		server.handleGeodataTile)

	return server
}

// pathfindPoint is one world position of the pathfind API.
type pathfindPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// pathfindRequest is the request body of POST /api/pathfind.
type pathfindRequest struct {
	Start pathfindPoint `json:"start"`
	End   pathfindPoint `json:"end"`
}

// pathfindResponse is the reply of POST /api/pathfind.
type pathfindResponse struct {
	Found         bool            `json:"found"`
	Aborted       bool            `json:"aborted"`
	Error         string          `json:"error,omitempty"`
	Start         pathfindPoint   `json:"start"`
	End           pathfindPoint   `json:"end"`
	Waypoints     []pathfindPoint `json:"waypoints"`
	Raw           []pathfindPoint `json:"raw"`
	DurationMs    float64         `json:"durationMs"`
	Explored      int             `json:"explored"`
	OpenLeft      int             `json:"openLeft"`
	Length        float64         `json:"length"`
	RegionsLoaded int             `json:"regionsLoaded"`
}

// handleConfig responds with the server mode and the geodata summary.
func (s *Server) handleConfig(w http.ResponseWriter, _ *http.Request) {
	stats := s.pathfinder.Stats()
	center := stats.Center
	if s.pathfindView != nil {
		center = *s.pathfindView
	}
	response := configResponse{
		Mode:     modePathfind,
		Geodata:  &stats,
		MaxSteps: s.pathfinder.MaxPassableHeight(),
		Defaults: &pathfindDefaults{
			Center: center,
			Scale:  defaultPathfindScale,
		},
	}
	writeJSON(w, s.logger, response)
}

// handlePathfind runs one path search for the markers of the map.
func (s *Server) handlePathfind(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
	if err != nil {
		http.Error(w, "failed to read the request body", http.StatusBadRequest)

		return
	}
	var request pathfindRequest
	if err := json.Unmarshal(body, &request); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)

		return
	}

	result, err := s.pathfinder.FindPath(
		pathfind.Vec3(request.Start), pathfind.Vec3(request.End),
		s.pathfinder.MaxPassableHeight())
	response := pathfindResponse{
		Found:         false,
		Aborted:       false,
		Error:         "",
		Start:         request.Start,
		End:           request.End,
		Waypoints:     []pathfindPoint{},
		Raw:           []pathfindPoint{},
		DurationMs:    0,
		Explored:      0,
		OpenLeft:      0,
		Length:        0,
		RegionsLoaded: s.pathfinder.Stats().LoadedRegions,
	}
	if err != nil {
		response.Error = err.Error()
		writeJSON(w, s.logger, response)

		return
	}
	if result != nil {
		response.Found = result.Found
		response.Aborted = result.Aborted
		response.Waypoints = toResponsePoints(result.Waypoints)
		response.Raw = downsample(toResponsePoints(result.RawPath),
			maxRawPathPoints)
		response.DurationMs = float64(result.Duration.Nanoseconds()) / 1e6
		response.Explored = result.Explored
		response.OpenLeft = result.OpenLeft
		response.Length = result.Length
	}
	writeJSON(w, s.logger, response)
}

// toResponsePoints converts world positions to the JSON shape.
func toResponsePoints(world []pathfind.Vec3) []pathfindPoint {
	points := make([]pathfindPoint, len(world))
	for i, position := range world {
		points[i] = pathfindPoint{
			X: position.X,
			Y: position.Y,
			Z: position.Z,
		}
	}

	return points
}

// downsample keeps every k-th point of an oversized path so the JSON
// stays small while the shape survives.
func downsample(points []pathfindPoint, limit int) []pathfindPoint {
	if len(points) <= limit {
		return points
	}
	stride := len(points) / limit
	kept := make([]pathfindPoint, 0, limit+1)
	for i := 0; i < len(points); i += stride {
		kept = append(kept, points[i])
	}
	kept = append(kept, points[len(points)-1])

	return kept
}
