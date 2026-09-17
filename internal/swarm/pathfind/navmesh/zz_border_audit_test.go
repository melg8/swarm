// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package navmesh

import (
	"testing"
)

// TestPackBorderAudit counts the external links per ordered region
// pair across the whole pack: the stitch writes both directions, so
// an asymmetry or a zero pair names the border the pack build lost.
// The test skips when the pack is absent and only reports.
func TestPackBorderAudit(t *testing.T) {
	dir := "../../../../data/navmesh"
	probe := NewMesh(dir)
	files := probe.TileFiles()
	if len(files) < 100 {
		t.Skip("the whole map pack is not present")
	}
	mesh := NewMesh(dir)
	mesh.SetCacheCapacity(1)

	type pair struct {
		a, b RegionKey
	}
	counts := make(map[pair]int)
	dead := make(map[pair]int)
	for _, key := range files {
		tile, err := mesh.Tile(key)
		if err != nil || tile == nil {
			t.Logf("tile %d_%d fails to load: %v", key.Col, key.Row, err)

			continue
		}
		for i := range tile.ExtLinks {
			ext := &tile.ExtLinks[i]
			p := pair{a: key,
				b: RegionKey{Col: int16(ext.Col), Row: int16(ext.Row)}}
			counts[p]++
			if ext.Poly == 0xFFFFFFFF {
				dead[p]++
			}
		}
	}
	// The asymmetries: A->B links but no B->A.
	asymmetric := 0
	for p, n := range counts {
		back := counts[pair{a: p.b, b: p.a}]
		if back == 0 {
			asymmetric++
			if asymmetric <= 20 {
				t.Logf("asymmetric border: %d_%d -> %d_%d (%d links,"+
					" %d dead), no way back", p.a.Col, p.a.Row, p.b.Col,
					p.b.Row, n, dead[p])
			}
		}
		if dead[p] > 0 && dead[p] == n {
			t.Logf("all-dead border: %d_%d -> %d_%d (%d links)",
				p.a.Col, p.a.Row, p.b.Col, p.b.Row, n)
		}
	}
	t.Logf("pack border audit: %d ordered pairs with links, %d"+
		" asymmetric", len(counts), asymmetric)
}
