// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
    "math"
    "testing"

    "github.com/stretchr/testify/require"
)

// The refused click root cause evidence (the owner question of the
// 2026-09-19 round: WHY does the server refuse the ordinary ground
// clicks in the village plaza zone, and the same at the shop and the
// teacher hall entries).
//
// The elven village geodata is a LAYER SANDWICH. The server resolves
// the layer of every click target (and of every line step of the
// getValidLocation walk) by the NEAREST height to the z the request
// carries (GeoEngine.getNearestZ / MultilayerBlock), so a click z
// that disagrees with the validating pack by more than HALF the local
// layer gap flips the resolved layer: the line check then runs
// against the roof band or the water floor instead of the deck, the
// NSWE of the wrong layer refuses the step, the destination collapses
// onto the walker (getValidLocation), and an older Mobius build
// answers the collapsed or no-path click with ActionFailed - the
// wholesale refused clicks of the 2026-09-14 dumps, the refusals the
// official client's own mouse clicks met on the same cell.
//
// The measured stacks of the pack this repository ships (the deck
// cells differ INSIDE one pack already: the plaza deck sits at -3056,
// the southwest gate corridor deck at -2992 - a 64 unit step):
//
//   - the village deck cells: two layers, the open deck top (-3056)
//     over the open water floor (-3928): an 872 unit gap, a 436 unit
//     flip margin;
//   - the teacher hall block: THREE layers, the roof band (-2464),
//     the interior floor (-2792) and the water floor (-3928): the
//     interior flip margins shrink to 164 (roof) and 568 (water) - a
//     pack vintage disagreement of 165 units of z flips a click into
//     the hall onto the roof layer;
//   - the shop interior cells: partially walled in the pack (the
//     nswe mask 7 - the east wall closed): the line check against
//     the interior layer refuses where the deck layer walks.
//
// The test pins the measured stacks so a geodata refresh that moves
// the sandwich renews the numbers in the refused click docs instead
// of silently invalidating them.

// sandwichLayers resolves the layer stack of a world cell in the
// village region of the real pack.
func sandwichLayers(t *testing.T, x, y int32) []Layer {
    t.Helper()
    engine := stuckSpotEngine(t)
    cell := WorldToCell(float64(x), float64(y))
    entry, err := engine.entry(CellToRegion(cell))
    require.NoError(t, err, "the village region must load")
    require.NotNil(t, entry.region)

    return entry.region.Layers(LocalCell(cell))
}

func TestVillageLayerSandwichPlazaDeck(t *testing.T) {
    layers := sandwichLayers(t, 45768, 49848)
    require.Len(t, layers, 2,
        "the plaza cell is a two layer stack: the deck over the water")
    require.Equal(t, int16(-3056), layers[0].Height,
        "the deck top layer height")
    require.True(t, layers[0].IsCompletelyOpen(),
        "the deck top walks open in every direction")
    require.Equal(t, int16(-3928), layers[1].Height,
        "the water floor layer height")
    // The flip margin: half the measured gap between the two layers.
    // A click z more than this far off the deck height resolves the
    // water floor and the line check runs against the wrong world.
    require.InDelta(t, 436.0,
        math.Abs(float64(layers[1].Height-layers[0].Height))/2, 0.5)
}

func TestVillageLayerSandwichTeacherHall(t *testing.T) {
    layers := sandwichLayers(t, 45725, 52105)
    require.GreaterOrEqual(t, len(layers), 3,
        "the teacher hall block is a three layer stack: the roof "+
            "band, the interior floor and the water floor")
    require.Equal(t, int16(-2464), layers[0].Height,
        "the roof band layer height")
    require.Equal(t, int16(-2792), layers[1].Height,
        "the interior floor layer height")
    // The flip margin toward the roof: half the measured gap between
    // the interior floor and the roof band - a click into the hall
    // whose z sits 165 units off the interior floor resolves the roof
    // band and refuses.
    require.InDelta(t, 164.0,
        float64(layers[0].Height-layers[1].Height)/2, 0.5)
}

func TestVillageLayerSandwichShopInterior(t *testing.T) {
    layers := sandwichLayers(t, 44995, 51706)
    require.GreaterOrEqual(t, len(layers), 2,
        "the shop interior cell stacks the interior over the water")
    require.Equal(t, int16(-2800), layers[0].Height,
        "the shop interior floor layer height")
    require.False(t, layers[0].IsCompletelyOpen(),
        "the shop interior is partially walled in the pack - the "+
            "line check against the interior layer refuses where "+
            "the deck layer walks")
    require.Equal(t, uint8(7), layers[0].NSWE,
        "the measured interior wall mask (north, south and west "+
            "open, the east wall closed)")
}

func TestVillageLayerSandwichDeckIsNotFlat(t *testing.T) {
    plaza := sandwichLayers(t, 45768, 49848)
    corridor := sandwichLayers(t, 43512, 50504)
    require.Equal(t, int16(-3056), plaza[0].Height,
        "the plaza deck height")
    require.Equal(t, int16(-2992), corridor[0].Height,
        "the gate corridor deck height - the deck steps 64 units "+
            "INSIDE one pack, so interpolated click z values between "+
            "the steps sit on layer boundaries the validating pack "+
            "resolves its own way")
}
