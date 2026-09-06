// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
	"bytes"
	"image/png"
	"testing"
)

// BenchmarkRenderRegionGiran measures the full resolution render of the
// real Giran region (one geodata cell per pixel).
func BenchmarkRenderRegionGiran(b *testing.B) {
	engine := giranEngine()
	key := RegionKey{Col: 22, Row: 22}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.RenderRegion(key, 2048, RenderHeight)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRenderRegionGiranPNG measures the render plus the PNG
// encoding of the tile, the cost the web server actually pays.
func BenchmarkRenderRegionGiranPNG(b *testing.B) {
	engine := giranEngine()
	key := RegionKey{Col: 22, Row: 22}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		img, err := engine.RenderRegion(key, 2048, RenderHeight)
		if err != nil {
			b.Fatal(err)
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			b.Fatal(err)
		}
	}
}
