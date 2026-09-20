// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// The render mode of the counter probe: a zoomed top-down picture of
// the raw geodata around the shop merchants (one pixel per cell), the
// classification colors of the map mode plus the merchant and route
// markers, written as a PNG the shop quarter round reads.
package main

import (
    "fmt"
    "image"
    "image/color"
    "image/png"
    "os"
    "path/filepath"

    "github.com/melg8/swarm/internal/swarm/pathfind"
)

// renderCustom paints one custom square (the shop quarter overview):
// the classification colors, every merchant of the list that falls
// inside as a red pixel and their mesh plan ends as blue pixels.
func renderCustom(
    regions map[string]*pathfind.Region, geodata string, scale, radius int,
    center merchant, list []merchant, planEnds map[string]pathfind.Vec3,
) {
    region := regions[regionPath(geodata, center.x, center.y)]
    buf := make([]pathfind.Layer, 0, 8)
    c := pathfind.WorldToCell(center.x, center.y)
    side := 2*radius + 1
    img := image.NewRGBA(image.Rect(0, 0, side*scale, side*scale))
    for row := -radius; row <= radius; row++ {
        for col := -radius; col <= radius; col++ {
            cell := pathfind.Point{
                X: c.X + int32(col),
                Y: c.Y + int32(row),
            }
            stack := region.LayerStack(pathfind.LocalCell(cell), buf)
            fill(img, (col+radius)*scale, (row+radius)*scale, scale,
                classifyColor(stack, int(center.z)))
        }
    }
    mark := func(x, y float64, col color.RGBA) {
        px, py := worldToPixel(x, y, c, radius)
        if px < 0 || px > 2*radius || py < 0 || py > 2*radius {
            return
        }
        fill(img, px*scale, py*scale, scale, col)
    }
    for _, m := range list {
        mark(m.x, m.y, color.RGBA{255, 0, 0, 255})
        if end, ok := planEnds[m.name]; ok {
            mark(end.X, end.Y, color.RGBA{0, 128, 255, 255})
        }
    }
    path := filepath.Join(outDir, "counterprobe_custom.png")
    file, err := os.Create(path)
    if err != nil {
        fmt.Printf("create %s: %v\n", path, err)
        os.Exit(1)
    }
    if err := png.Encode(file, img); err != nil {
        fmt.Printf("encode %s: %v\n", path, err)
        os.Exit(1)
    }
    file.Close()
    fmt.Printf("wrote %s\n", path)
}

// renderZoom paints the cell square around each merchant into one PNG
// per merchant (one pixel per cell, scale factor applied). planEnds
// carries the mesh route end per merchant name (nil when the route
// answered nothing) drawn as the blue marker.
func renderZoom(
    regions map[string]*pathfind.Region, geodata string, scale int,
    planEnds map[string]pathfind.Vec3,
) {
    for _, m := range merchants {
        region := regions[regionPath(geodata, m.x, m.y)]
        buf := make([]pathfind.Layer, 0, 8)
        center := pathfind.WorldToCell(m.x, m.y)
        const radius = 24 // a 49x49 cell square
        side := 2*radius + 1
        img := image.NewRGBA(image.Rect(0, 0, side*scale, side*scale))
        for row := -radius; row <= radius; row++ {
            for col := -radius; col <= radius; col++ {
                cell := pathfind.Point{
                    X: center.X + int32(col),
                    Y: center.Y + int32(row),
                }
                stack := region.LayerStack(pathfind.LocalCell(cell), buf)
                c := classifyColor(stack, int(m.z))
                fill(img, (col+radius)*scale, (row+radius)*scale,
                    scale, c)
            }
        }
        // The merchant spawn marker (red, center).
        fill(img, radius*scale, radius*scale, scale,
            color.RGBA{255, 0, 0, 255})
        // The mesh plan end marker (blue).
        if end, ok := planEnds[m.name]; ok {
            ex, ey := worldToPixel(end.X, end.Y, center, radius)
            fill(img, ex*scale, ey*scale, scale,
                color.RGBA{0, 128, 255, 255})
        }
        // The heading arrow pixel: one cell toward the facing.
        hx, hy := headingPixel(m.heading)
        fill(img, (radius+hx)*scale, (radius+hy)*scale, scale,
            color.RGBA{255, 220, 0, 255})

        name := fmt.Sprintf("counterprobe_%s.png", m.name)
        path := filepath.Join(outDir, name)
        file, err := os.Create(path)
        if err != nil {
            fmt.Printf("create %s: %v\n", path, err)
            os.Exit(1)
        }
        if err := png.Encode(file, img); err != nil {
            fmt.Printf("encode %s: %v\n", path, err)
            os.Exit(1)
        }
        file.Close()
        fmt.Printf("wrote %s\n", path)
    }
}

// classifyColor maps the layer stack to the picture color.
func classifyColor(stack []pathfind.Layer, refZ int) color.RGBA {
    best := color.RGBA{0, 0, 0, 255} // no layers: wall or void
    bestScore := 1 << 30
    for _, l := range stack {
        d := int(l.Height) - refZ
        score := abs(d)
        var c color.RGBA
        switch {
        case d >= -96 && d <= 24:
            c = color.RGBA{190, 190, 190, 255} // floor
        case d < -96 && d >= -200:
            c = color.RGBA{120, 120, 120, 255} // floor below
        case d > 24 && d <= 96:
            c = color.RGBA{255, 140, 0, 255} // low step: the counter
        case d > 96 && d <= 200:
            c = color.RGBA{160, 60, 60, 255} // high step
        case d > 200 && d <= 400:
            c = color.RGBA{60, 60, 110, 255} // deck/roof only
        default:
            c = color.RGBA{0, 0, 0, 255}
        }
        if d >= -96 && d <= 24 {
            return c
        }
        if score < bestScore {
            best, bestScore = c, score
        }
    }

    return best
}

// worldToPixel converts a world position to the pixel cell of the
// square around the center cell.
func worldToPixel(
    x, y float64, center pathfind.Point, radius int,
) (int, int) {
    col := int(worldCellCoord(x)-center.X) + radius
    row := int(worldCellCoord(y)-center.Y) + radius

    return col, row
}

// worldCellCoord computes the global cell coordinate of a world
// coordinate (the same mapping WorldToCell uses, split for the pixel
// math; +20 is the tileZeroCol anchor of the geometry).
func worldCellCoord(v float64) int32 {
    return int32((v/32768 + 20) * 2048)
}

// headingPixel converts the L2 heading to the one-cell arrow offset
// (0 = east, counterclockwise in the cell grid means +y is south, so
// the arrow goes (cos, sin) with the L2 convention y+ south).
func headingPixel(heading float64) (int, int) {
    angle := heading / 65536 * 2 * 3.14159265
    hx := int(cos(angle) + 0.5)
    hy := int(sin(angle) + 0.5)

    return hx, hy
}

// outDir is the PNG output directory.
var outDir = "."

// cos and sin are tiny local shorthands.
func cos(v float64) float64 {
    // Taylor at [0, 2pi] is enough for the arrow precision.
    res := 1.0
    term := 1.0
    for i := 1; i <= 8; i++ {
        term *= -v * v / float64(2*i*(2*i-1))
        res += term
    }

    return res
}

func sin(v float64) float64 {
    res := v
    term := v
    for i := 1; i <= 8; i++ {
        term *= -v * v / float64(2*i*(2*i+1))
        res += term
    }

    return res
}

// fill paints a scale x scale pixel block.
func fill(img *image.RGBA, x, y, scale int, c color.RGBA) {
    for dy := range scale {
        for dx := range scale {
            img.Set(x+dx, y+dy, c)
        }
    }
}
