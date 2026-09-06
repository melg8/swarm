// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
	"image"
	"sync"
)

// RenderMode selects what the geodata visualization paints per cell.
type RenderMode string

// Render modes of the geodata tiles:
//   - height: grayscale terrain with a blue tint below the typical sea
//     level, red over cells whose walk surface has a closed wall and a
//     green tint on multilayer cells (bridges, interiors);
//   - walls: white for open cells, red for walled ones - the pure
//     connectivity view;
//   - layers: the count of walkable layers per cell, one gray, two
//     yellow, three orange, four or more red.
const (
	RenderHeight RenderMode = "height"
	RenderWalls  RenderMode = "walls"
	RenderLayers RenderMode = "layers"
)

// ParseRenderMode maps a request parameter to a render mode, defaulting
// to the height view.
func ParseRenderMode(value string) RenderMode {
	switch RenderMode(value) {
	case RenderWalls:
		return RenderWalls
	case RenderLayers:
		return RenderLayers
	default:
		return RenderHeight
	}
}

// Height window of the grayscale ramp and the typical C1 sea level used
// for the underwater tint.
const (
	heightMin = -10240
	heightMax = 14336
	seaLevel  = -3840
)

// Palette colors of the overlays.
var (
	wallColor  = [3]uint8{214, 44, 44}
	waterColor = [3]uint8{36, 96, 200}
	deckColor  = [3]uint8{56, 178, 80}
)

// heightLUT maps every possible cell height to a packed RGB color, so
// the render loop stays a slice lookup per cell.
var (
	heightLUTOnce sync.Once
	heightLUT     [65536]uint32
)

func buildHeightLUT() {
	span := float64(heightMax - heightMin)
	for i := range heightLUT {
		world := int16(i) // the LUT is indexed by the raw int16 height
		v := (float64(world) - float64(heightMin)) / span
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		gray := uint32(26 + v*198)
		r, g, b := gray, gray, gray
		if int(world) < seaLevel {
			r, g, b = mix(r, g, b, waterColor, 0.4)
		}
		heightLUT[i] = r | g<<8 | b<<16
	}
}

// mix blends a base color toward the target by the fraction, clamping
// every channel into the byte range.
func mix(r, g, b uint32, target [3]uint8, fraction float64) (uint32, uint32, uint32) {
	channel := func(c uint32, t uint8) uint32 {
		v := int(c) + int((float64(t)-float64(c))*fraction)
		if v < 0 {
			v = 0
		}
		if v > 255 {
			v = 255
		}

		return uint32(v)
	}

	return channel(r, target[0]), channel(g, target[1]), channel(b, target[2])
}

// RenderRegion renders one geodata region into a square image of size x
// size pixels (size must divide the 2048 cells of a region side, so
// every pixel covers an exact block of cells). The rendered view follows
// the walk surface the pathfinder sees at the default height: the layer
// closest to zero.
func (e *Engine) RenderRegion(
	key RegionKey, size int, mode RenderMode,
) (*image.RGBA, error) {
	entry, err := e.entry(key)
	if err != nil {
		return nil, err
	}

	return entry.region.render(size, mode), nil
}

// render paints the region. Every pixel aggregates the cpp x cpp cells
// it covers: walls stay visible at every zoom level and the height of
// the open cells is averaged.
func (r *Region) render(size int, mode RenderMode) *image.RGBA {
	heightLUTOnce.Do(buildHeightLUT)

	const channelShift = 8
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	cpp := cellsPerRegionSide / size
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var sumR, sumG, sumB, open, walled, maxLayers int
			for ly := py * cpp; ly < (py+1)*cpp; ly++ {
				for lx := px * cpp; lx < (px+1)*cpp; lx++ {
					top, count, wall := r.topLayer(Point{
						X: int32(lx),
						Y: int32(ly),
					})
					if count > maxLayers {
						maxLayers = count
					}
					if wall {
						walled++
						continue
					}
					lut := heightLUT[uint16(top.Height)]
					sumR += int(lut & 0xFF)
					sumG += int(lut >> channelShift & 0xFF)
					sumB += int(lut >> 16 & 0xFF)
					open++
				}
			}
			color := pixelColor(mode, open, walled, maxLayers,
				sumR, sumG, sumB)
			offset := (py*size + px) * 4
			img.Pix[offset] = color[0]
			img.Pix[offset+1] = color[1]
			img.Pix[offset+2] = color[2]
			img.Pix[offset+3] = 255
		}
	}

	return img
}

// pixelColor folds the per pixel cell statistics into one color.
func pixelColor(
	mode RenderMode, open, walled, maxLayers int,
	sumR, sumG, sumB int,
) [3]uint8 {
	switch mode {
	case RenderLayers:
		switch {
		case maxLayers >= 4:
			return [3]uint8{220, 60, 40}
		case maxLayers == 3:
			return [3]uint8{240, 140, 40}
		case maxLayers == 2:
			return [3]uint8{240, 200, 60}
		default:
			return [3]uint8{210, 210, 210}
		}
	case RenderWalls:
		fraction := float64(walled) / float64(walled+open)
		r, g, b := mix(230, 230, 230, wallColor, fraction)

		return [3]uint8{uint8(r), uint8(g), uint8(b)}
	default:
		var r, g, b uint32
		if walled > 0 {
			// Walls dominate the pixel but keep a hint of the terrain
			// height under them.
			share := float64(walled) / float64(walled+open)
			terrain := uint32(120)
			if open > 0 {
				terrain = uint32((sumR + sumG + sumB) / (3 * open))
			}
			r, g, b = mix(terrain, terrain, terrain, wallColor, 0.45+0.4*share)
		} else {
			r = uint32(sumR / open)
			g = uint32(sumG / open)
			b = uint32(sumB / open)
		}
		if maxLayers >= 2 {
			r, g, b = mix(r, g, b, deckColor, 0.35)
		}

		return [3]uint8{uint8(r), uint8(g), uint8(b)}
	}
}

// topLayer returns the walk surface of a cell (the layer with the
// highest height, which is what the pathfinder picks at the default
// height of zero for negative world heights), the layer count and
// whether the surface has any closed wall.
func (r *Region) topLayer(local Point) (top Layer, count int, wall bool) {
	span := r.spans[cellSpanIndex(blockIndex(local), cellIndexInBlock(local))]
	offset, layers := unpackSpan(span)
	top = r.pool.get(r.refs[offset])
	for i := 1; i < layers; i++ {
		candidate := r.pool.get(r.refs[offset+i])
		if candidate.Height > top.Height {
			top = candidate
		}
	}

	return top, layers, !top.IsCompletelyOpen()
}
