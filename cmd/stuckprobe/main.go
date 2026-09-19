// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// stuck points verdict probe: the exact RouteApproach answers for the
// two reported frozen bot positions after the pocket bound fix.
package main

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/pathfind"
	"github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

func main() {
	mesh := navmesh.NewMesh("data/navmesh")

	points := []struct {
		name    string
		x, y, z float64
	}{
		{"A (heading 21963)", 43632, 50560, -2960},
		{"B (heading 26712)", 41920, 52128, -3000},
	}
	dests := []pathfind.Vec3{
		{X: 25500, Y: 51095, Z: -3408},
		{X: 43032, Y: 50408, Z: -2992},
		{X: 45478, Y: 49730, Z: -3056},
	}

	for _, p := range points {
		start := navmesh.Pos{X: p.x, Y: p.y, Z: p.z}
		fmt.Printf("\n=== point %s ===\n", p.name)
		ref, _, ok := mesh.FindNearestPoly(start)
		if !ok {
			fmt.Println("no start poly")

			continue
		}
		tile, poly := mesh.PolyOf(ref)
		x0, y0, x1, y1 := tile.WorldRect(poly)
		fmt.Printf("start poly box %.0f %.0f .. %.0f %.0f\n", x0, y0, x1, y1)

		for _, d := range dests {
			route, err := mesh.RouteApproach(start,
				navmesh.Pos{X: d.X, Y: d.Y, Z: d.Z}, 0,
				navmesh.DefaultFilter())
			if err != nil {
				fmt.Printf("  -> %.0f %.0f: ERR %v\n", d.X, d.Y, err)

				continue
			}
			fmt.Printf("  -> %.0f %.0f: found=%v partial=%v pocket=%v "+
				"wps=%d\n", d.X, d.Y, route.Found, route.Partial,
				route.PocketEscape, len(route.Waypoints))
			for i, wp := range route.Waypoints {
				fmt.Printf("     wp[%d] %.0f %.0f %.0f\n", i, wp.X, wp.Y,
					wp.Z)
			}
		}
	}
}
