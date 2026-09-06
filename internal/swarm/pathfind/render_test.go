// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/require"
)

// pixelOf reads the RGBA color of an image pixel as uint8 channels.
func pixelOf(img *image.RGBA, x, y int) color.RGBA {
	return color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
}

// TestParseRenderMode checks the request parameter mapping.
func TestParseRenderMode(t *testing.T) {
	require.Equal(t, RenderHeight, ParseRenderMode("height"))
	require.Equal(t, RenderWalls, ParseRenderMode("walls"))
	require.Equal(t, RenderLayers, ParseRenderMode("layers"))
	require.Equal(t, RenderHeight, ParseRenderMode("nonsense"))
	require.Equal(t, RenderHeight, ParseRenderMode(""))
}

// TestRenderRegionHeightMode renders a synthetic region at full cell
// resolution and checks the palette: open terrain is grayscale by
// height, walled cells turn red, underwater cells turn blue and
// multilayer cells get the deck tint.
func TestRenderRegionHeightMode(t *testing.T) {
	spec := &regionSpec{}
	spec.setFlat(-104)
	// A walled cell (a building block).
	spec.setCell(5, 3, Layer{Height: -3536, NSWE: 0})
	// A cell below the typical sea level.
	spec.setCell(600, 1300, Layer{Height: -4008, NSWE: nsweAll})
	// A multilayer cell (a bridge deck over the ground).
	spec.setMultilayer(100, 200, []Layer{
		{Height: -48, NSWE: nsweAll},
		{Height: 496, NSWE: nsweAll},
	})
	engine := newTestEngine(t, spec)
	key := RegionKey{Col: testRegionCol, Row: testRegionRow}

	img, err := engine.RenderRegion(key, cellsPerRegionSide, RenderHeight)
	require.NoError(t, err)
	require.Equal(t, cellsPerRegionSide, img.Rect.Dx())

	// Open land: pure grayscale of the height (truncated ramp value).
	open := pixelOf(img, 1000, 1000)
	require.Equal(t, uint8(107), open.R)
	require.Equal(t, open.R, open.G)
	require.Equal(t, open.G, open.B)

	// The walled cell is red dominant.
	wall := pixelOf(img, 5, 3)
	require.Greater(t, wall.R, uint8(150))
	require.Less(t, wall.G, uint8(90))
	require.Less(t, wall.B, uint8(90))

	// The underwater cell is blue dominant.
	deep := pixelOf(img, 600, 1300)
	require.Greater(t, deep.B, deep.R)

	// The multilayer cell carries the deck tint.
	deck := pixelOf(img, 100, 200)
	require.Greater(t, deck.G, deck.R)
	require.Greater(t, deck.G, deck.B)
}

// TestRenderRegionWallsMode checks the pure connectivity view: open
// cells are near white, walled cells red.
func TestRenderRegionWallsMode(t *testing.T) {
	spec := &regionSpec{}
	spec.setFlat(0)
	spec.setCell(5, 3, Layer{Height: 0, NSWE: 0})
	engine := newTestEngine(t, spec)
	key := RegionKey{Col: testRegionCol, Row: testRegionRow}

	img, err := engine.RenderRegion(key, cellsPerRegionSide, RenderWalls)
	require.NoError(t, err)

	open := pixelOf(img, 1000, 1000)
	require.Equal(t, uint8(230), open.R)
	wall := pixelOf(img, 5, 3)
	require.Equal(t, uint8(214), wall.R)
}

// TestRenderRegionLayersMode checks the layer count view.
func TestRenderRegionLayersMode(t *testing.T) {
	spec := &regionSpec{}
	spec.setFlat(0)
	spec.setMultilayer(100, 200, []Layer{
		{Height: 0, NSWE: nsweAll},
		{Height: 496, NSWE: nsweAll},
		{Height: 1000, NSWE: nsweAll},
	})
	engine := newTestEngine(t, spec)
	key := RegionKey{Col: testRegionCol, Row: testRegionRow}

	img, err := engine.RenderRegion(key, cellsPerRegionSide, RenderLayers)
	require.NoError(t, err)

	single := pixelOf(img, 1000, 1000)
	require.Equal(t, uint8(210), single.R)
	triple := pixelOf(img, 100, 200)
	require.Equal(t, uint8(240), triple.R)
	require.Equal(t, uint8(140), triple.G)
}

// TestRenderRegionDownscale checks that a walled cell stays visible
// when a pixel covers a block of cells (the walls survive downscaling).
func TestRenderRegionDownscale(t *testing.T) {
	spec := &regionSpec{}
	spec.setFlat(0)
	spec.setCell(5, 3, Layer{Height: 0, NSWE: 0})
	engine := newTestEngine(t, spec)
	key := RegionKey{Col: testRegionCol, Row: testRegionRow}

	img, err := engine.RenderRegion(key, cellsPerRegionSide/2, RenderWalls)
	require.NoError(t, err)

	// The pixel covering the walled cell leans toward red.
	pixel := pixelOf(img, 2, 1)
	require.Greater(t, int(pixel.R), int(pixel.G))
}

// TestRenderRegionMissing checks the error for a region without file.
func TestRenderRegionMissing(t *testing.T) {
	engine := NewEngine(t.TempDir())
	_, err := engine.RenderRegion(RegionKey{Col: 30, Row: 30},
		cellsPerRegionSide, RenderHeight)
	require.Error(t, err)
}
