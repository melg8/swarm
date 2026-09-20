package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/klauspost/compress/zstd"
	"github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
)

func main() {
	dir := "data/navmesh"
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".nm" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			panic(err)
		}
		if len(data) >= 4 && data[0] == 0x28 && data[1] == 0xB5 && data[2] == 0x2F && data[3] == 0xFD {
			dec, _ := zstd.NewReader(nil)
			raw, err := dec.DecodeAll(data, nil)
			if err != nil {
				panic(err)
			}
			data = raw
		}
		tile, err := navmesh.DecodeTile(data)
		if err != nil {
			fmt.Printf("%s: DECODE ERROR %v\n", entry.Name(), err)

			continue
		}
		g := tile.Grid
		if g == nil {
			fmt.Printf("%s: no grid (bv tree %d)\n", entry.Name(), len(tile.BVTree))

			continue
		}
		sum := uint64(0)
		max := uint32(0)
		for _, e := range g.Entries {
			sum += uint64(e)
			if e > max {
				max = e
			}
		}
		fmt.Printf("%s: polys=%d entries=%d sum=%d max=%d rawLen=%d\n",
			entry.Name(), len(tile.Polys), len(g.Entries), sum, max, len(data))
	}
}
