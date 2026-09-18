// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// The navmesh-export command bridges the built tile pack to the
// external pathfinding harnesses. It dumps the polygons, the links
// and the portal segments of the requested regions into a flat
// little endian binary the raasta and the condor benchmark ports
// read, so the third party algorithms answer the same queries over
// the same graph the game mesh answers. The optional -probe flag
// times the production Route over the same pack and endpoints, which
// puts the flat numbers next to the external ones in one run.
//
// The dump layout (little endian):
//
//	header: magic u32 'L2MB', version u32, polyCount u32,
//	        linkCount u32, skippedLinks u32,
//	        start xyz f64, goal xyz f64 (6 doubles total),
//	        regionCount u32, region entries col i16 row i16 base u32
//	polys:  x0 y0 x1 y1 f32 (the world rect), area u8, 7 pad bytes
//	links:  from u32, to u32 (the global poly indices),
//	        ax ay bx by f32 (the portal segment endpoints)
//
// The links whose external target region is not in the -regions set
// are skipped and counted in the header (the harnesses see exactly
// the closed subgraph).
package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

// dumpMagic tags the binary ('L2MB' little endian).
const dumpMagic = 0x424D324C

// dumpVersion is the dump format version this tool writes.
const dumpVersion = 1

// dumpLink is one polygon-to-polygon connection in world form: the
// global endpoints plus the portal segment the funnel may cross.
type dumpLink struct {
	From, To       uint32
	AX, AY, BX, BY float32
}

func main() {
	dir := flag.String("navmesh", "data/navmesh", "the tile pack directory")
	regions := flag.String("regions", "20_19,20_20,21_19,21_20",
		"the comma separated region keys the dump covers")
	from := flag.String("from", "12338,42444,-3640", "the route start x,y,z")
	to := flag.String("to", "58630,91061,-3696", "the route goal x,y,z")
	out := flag.String("out", "/tmp/l2mesh.bin", "the dump output path")
	probe := flag.Bool("probe", false, "time the production Route over the pack")
	flag.Parse()

	start, end, err := parseEndpoints(*from, *to)
	if err != nil {
		fail("the endpoints: %v", err)
	}

	keys, err := parseRegions(*regions)
	if err != nil {
		fail("the regions: %v", err)
	}

	if *probe {
		runProbe(*dir, keys, start, end)
	}

	if err := runDump(*dir, keys, start, end, *out); err != nil {
		fail("the dump: %v", err)
	}
}

// runProbe times the production Route three times over a fresh mesh:
// the first answer is the user visible cold path (the tile decode,
// the sidecar loads, the coarse search and the refinement hops), the
// second reuses the decoded tiles and the loaded abstracts (the
// warm path), the third pins the steady state. The decode share of
// the cold answer falls out of the difference between the first two.
func runProbe(dir string, keys []navmesh.RegionKey, start, end navmesh.Pos) {
	mesh := navmesh.NewMesh(dir)
	for round := 1; round <= 3; round++ {
		roundStart := time.Now()
		route, err := mesh.Route(start, end, navmesh.DefaultFilter())
		roundTime := time.Since(roundStart)
		if err != nil {
			fmt.Printf("route %d: error %v after %s\n", round, err, roundTime)
			return
		}
		fmt.Printf("route %d: found=%t partial=%t explored=%d corridor=%d "+
			"waypoints=%d time=%s\n", round, route.Found, route.Partial,
			route.Explored, len(route.Corridor), len(route.Waypoints), roundTime)
	}
}

// runDump writes the closed subgraph of the requested regions.
func runDump(dir string, keys []navmesh.RegionKey, start, end navmesh.Pos,
	outPath string) error {
	mesh := navmesh.NewMesh(dir)

	tiles := make([]*navmesh.Tile, 0, len(keys))
	for _, key := range keys {
		tile, err := mesh.Tile(key)
		if err != nil {
			return fmt.Errorf("region %d_%d: %w", key.Col, key.Row, err)
		}

		tiles = append(tiles, tile)
	}

	// The global poly index space: one base per loaded region.
	bases := make([]uint32, len(tiles))
	base := uint32(0)
	baseOfKey := make(map[navmesh.RegionKey]uint32, len(tiles))
	polyTotal := 0
	for i, tile := range tiles {
		bases[i] = base
		baseOfKey[keys[i]] = base
		base += uint32(len(tile.Polys))
		polyTotal += len(tile.Polys)
	}

	// The link pass: resolve every chain entry to the global target
	// index and the world portal segment; skip the links whose
	// external region is outside the dump.
	links := make([]dumpLink, 0, polyTotal)
	skipped := 0
	for i, tile := range tiles {
		for pi := range tile.Polys {
			poly := &tile.Polys[pi]
			for li := poly.FirstLink; li >= 0; li = tile.Links[li].Next {
				link := &tile.Links[li]
				var to uint32
				if link.To >= 0 {
					to = bases[i] + uint32(link.To)
				} else {
					ext := &tile.ExtLinks[-link.To-1]
					target, ok := baseOfKey[navmesh.RegionKey{
						Col: int16(ext.Col), Row: int16(ext.Row)}]
					if !ok {
						skipped++
						continue
					}

					to = target + ext.Poly
				}

				ax, ay, bx, by := tile.Portal(poly, link)
				links = append(links, dumpLink{
					From: bases[i] + uint32(pi), To: to,
					AX: float32(ax), AY: float32(ay),
					BX: float32(bx), BY: float32(by),
				})
			}
		}
	}

	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	writer := bufio.NewWriterSize(out, 1<<20)
	var header [72]byte
	binary.LittleEndian.PutUint32(header[0:4], dumpMagic)
	binary.LittleEndian.PutUint32(header[4:8], dumpVersion)
	binary.LittleEndian.PutUint32(header[8:12], uint32(polyTotal))
	binary.LittleEndian.PutUint32(header[12:16], uint32(len(links)))
	binary.LittleEndian.PutUint32(header[16:20], uint32(skipped))
	binary.LittleEndian.PutUint64(header[20:28], math.Float64bits(start.X))
	binary.LittleEndian.PutUint64(header[28:36], math.Float64bits(start.Y))
	binary.LittleEndian.PutUint64(header[36:44], math.Float64bits(start.Z))
	binary.LittleEndian.PutUint64(header[44:52], math.Float64bits(end.X))
	binary.LittleEndian.PutUint64(header[52:60], math.Float64bits(end.Y))
	binary.LittleEndian.PutUint64(header[60:68], math.Float64bits(end.Z))
	if _, err := writer.Write(header[:68]); err != nil {
		return err
	}

	binary.LittleEndian.PutUint32(header[0:4], uint32(len(keys)))
	if _, err := writer.Write(header[:4]); err != nil {
		return err
	}
	for i, key := range keys {
		binary.LittleEndian.PutUint16(header[0:2], uint16(key.Col))
		binary.LittleEndian.PutUint16(header[2:4], uint16(key.Row))
		binary.LittleEndian.PutUint32(header[4:8], bases[i])
		if _, err := writer.Write(header[:8]); err != nil {
			return err
		}
	}

	// The poly records: the world rect plus the area byte.
	buf := make([]byte, 17)
	for _, tile := range tiles {
		for pi := range tile.Polys {
			poly := &tile.Polys[pi]
			x0, y0, x1, y1 := tile.WorldRect(poly)
			binary.LittleEndian.PutUint32(buf[0:4], math.Float32bits(float32(x0)))
			binary.LittleEndian.PutUint32(buf[4:8], math.Float32bits(float32(y0)))
			binary.LittleEndian.PutUint32(buf[8:12], math.Float32bits(float32(x1)))
			binary.LittleEndian.PutUint32(buf[12:16], math.Float32bits(float32(y1)))
			buf[16] = poly.Area
			if _, err := writer.Write(buf); err != nil {
				return err
			}
		}
	}

	// The link records: the endpoints and the portal segment.
	lbuf := make([]byte, 24)
	for _, link := range links {
		binary.LittleEndian.PutUint32(lbuf[0:4], link.From)
		binary.LittleEndian.PutUint32(lbuf[4:8], link.To)
		binary.LittleEndian.PutUint32(lbuf[8:12], math.Float32bits(link.AX))
		binary.LittleEndian.PutUint32(lbuf[12:16], math.Float32bits(link.AY))
		binary.LittleEndian.PutUint32(lbuf[16:20], math.Float32bits(link.BX))
		binary.LittleEndian.PutUint32(lbuf[20:24], math.Float32bits(link.BY))
		if _, err := writer.Write(lbuf); err != nil {
			return err
		}
	}

	if err := writer.Flush(); err != nil {
		return err
	}

	fmt.Printf("dump: %d polys, %d links (%d skipped), %d regions -> %s\n",
		polyTotal, len(links), skipped, len(keys), outPath)
	return nil
}

// parseRegions reads the 20_19 style keys.
func parseRegions(spec string) ([]navmesh.RegionKey, error) {
	parts := strings.Split(spec, ",")
	keys := make([]navmesh.RegionKey, 0, len(parts))
	seen := make(map[navmesh.RegionKey]bool, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		halves := strings.SplitN(part, "_", 2)
		if len(halves) != 2 {
			return nil, fmt.Errorf("region %q is not the X_Y form", part)
		}

		col, err := strconv.ParseInt(halves[0], 10, 16)
		if err != nil {
			return nil, fmt.Errorf("region %q column: %w", part, err)
		}

		row, err := strconv.ParseInt(halves[1], 10, 16)
		if err != nil {
			return nil, fmt.Errorf("region %q row: %w", part, err)
		}

		key := navmesh.RegionKey{Col: int16(col), Row: int16(row)}
		if !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}

	if len(keys) == 0 {
		return nil, errors.New("no regions given")
	}

	return keys, nil
}

// parseEndpoints reads the x,y,z world triples.
func parseEndpoints(fromSpec, toSpec string) (navmesh.Pos, navmesh.Pos, error) {
	start, err := parsePos(fromSpec)
	if err != nil {
		return navmesh.Pos{}, navmesh.Pos{}, fmt.Errorf("from: %w", err)
	}

	end, err := parsePos(toSpec)
	if err != nil {
		return navmesh.Pos{}, navmesh.Pos{}, fmt.Errorf("to: %w", err)
	}

	return start, end, nil
}

func parsePos(spec string) (navmesh.Pos, error) {
	parts := strings.Split(spec, ",")
	if len(parts) != 3 {
		return navmesh.Pos{}, fmt.Errorf("%q is not the x,y,z form", spec)
	}

	var vals [3]float64
	for i, part := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return navmesh.Pos{}, fmt.Errorf("%q: %w", part, err)
		}

		vals[i] = v
	}

	return navmesh.Pos{X: vals[0], Y: vals[1], Z: vals[2]}, nil
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "navmesh-export: "+format+"\n", args...)
	os.Exit(1)
}
