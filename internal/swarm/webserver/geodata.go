// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
	"bytes"
	"image/png"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/melg8/swarm/internal/swarm/pathfind"
)

// Geodata tile levels of the visualization pyramid: level 0 renders one
// geodata cell per pixel (2048x2048 per region tile) and every next
// level halves the resolution, mirroring the map tile pyramid.
var geodataTilePixels = map[int]int{0: 2048, 1: 1024, 2: 512, 3: 256, 4: 128}

// geodataTileCacheMax bounds how many encoded PNG tiles stay in memory
// (~1 MB each at the full resolution, less further down the pyramid).
const geodataTileCacheMax = 24

// geodataTileKey identifies one encoded tile of the visualization.
type geodataTileKey struct {
	mode  string
	level int
	col   int
	row   int
}

// geodataTileCache is a small LRU of encoded PNG tiles.
type geodataTileCache struct {
	mu    sync.Mutex
	tiles map[geodataTileKey][]byte
	order []geodataTileKey
}

func newGeodataTileCache() *geodataTileCache {
	return &geodataTileCache{
		tiles: make(map[geodataTileKey][]byte),
		order: make([]geodataTileKey, 0, geodataTileCacheMax),
	}
}

// get returns the cached tile and moves it to the back of the LRU.
func (c *geodataTileCache) get(key geodataTileKey) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	tile, ok := c.tiles[key]
	if !ok {
		return nil, false
	}
	for i, candidate := range c.order {
		if candidate == key {
			c.order = append(c.order[:i], c.order[i+1:]...)
			c.order = append(c.order, key)

			break
		}
	}

	return tile, true
}

// put stores a tile, evicting the least recently used one on overflow.
func (c *geodataTileCache) put(key geodataTileKey, tile []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.tiles[key]; ok {
		return
	}
	c.tiles[key] = tile
	c.order = append(c.order, key)
	for len(c.order) > geodataTileCacheMax {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.tiles, oldest)
	}
}

// handleGeodataTile serves one rendered geodata tile as PNG. The tile
// name is "{bx}_{by}.png" (the region file coordinates); the tiles are
// immutable per (mode, level, region), so the browser is told to cache
// them aggressively.
func (s *Server) handleGeodataTile(w http.ResponseWriter, r *http.Request) {
	level, err := strconv.Atoi(r.PathValue("level"))
	if err != nil {
		http.Error(w, "invalid tile level", http.StatusBadRequest)

		return
	}
	size, ok := geodataTilePixels[level]
	if !ok {
		http.Error(w, "unknown tile level", http.StatusBadRequest)

		return
	}
	name := r.PathValue("name")
	if !strings.HasSuffix(name, ".png") {
		http.Error(w, "tiles are served as .png", http.StatusBadRequest)

		return
	}
	colText, rowText, found := strings.Cut(strings.TrimSuffix(name, ".png"), "_")
	if !found {
		http.Error(w, "invalid tile name", http.StatusBadRequest)

		return
	}
	col, err := strconv.Atoi(colText)
	if err != nil {
		http.Error(w, "invalid tile column", http.StatusBadRequest)

		return
	}
	row, err := strconv.Atoi(rowText)
	if err != nil {
		http.Error(w, "invalid tile row", http.StatusBadRequest)

		return
	}
	mode := string(pathfind.ParseRenderMode(r.URL.Query().Get("mode")))

	key := geodataTileKey{mode: mode, level: level, col: col, row: row}
	if tile, ok := s.geodataTiles.get(key); ok {
		writeGeodataTile(w, tile)

		return
	}

	img, err := s.pathfinder.RenderRegion(pathfind.RegionKey{
		Col: int16(col),
		Row: int16(row),
	}, size, pathfind.RenderMode(mode))
	if err != nil {
		s.logger.Printf("Geodata tile %d_%d unavailable: %v", col, row, err)
		http.Error(w, "geodata region unavailable", http.StatusNotFound)

		return
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		http.Error(w, "tile encoding failed", http.StatusInternalServerError)

		return
	}
	tile := encoded.Bytes()
	s.geodataTiles.put(key, tile)
	writeGeodataTile(w, tile)
}

// writeGeodataTile responds with one PNG tile.
func writeGeodataTile(w http.ResponseWriter, tile []byte) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if _, err := w.Write(tile); err != nil {
		log.Printf("Error writing geodata tile: %v", err)
	}
}
