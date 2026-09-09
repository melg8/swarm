// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package pathfind

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestClosestHeight pins the destination deck resolution the zone
// return relies on: the layer of the cell whose height is closest to
// the reference z - the way the server resolves a destination, not
// the deck the walker stands on. The live case behind it: a zone
// center on the ground under a floating city deck - a walk planned
// with the walker height at the goal put the 3D approach target mid
// air and the search could never reach it.
func TestClosestHeight(t *testing.T) {
	spec := &regionSpec{}
	spec.setFlat(-3000)
	// A deck rides 600 above the ground on one square of the world,
	// like the floating elven city over the lake shore.
	for lx := 250; lx <= 260; lx++ {
		for ly := 250; ly <= 260; ly++ {
			spec.setMultilayer(lx, ly, []Layer{
				{Height: -3000, NSWE: nsweAll},
				{Height: -2400, NSWE: nsweAll},
			})
		}
	}
	engine := newTestEngine(t, spec)

	// A destination named with the deck height lands on the deck.
	height, err := engine.ClosestHeight(
		worldOf(252, 252, 0).X, worldOf(252, 252, 0).Y, -2400)
	require.NoError(t, err)
	require.EqualValues(t, -2400, height)

	// The same cell named with the ground height lands on the ground.
	height, err = engine.ClosestHeight(
		worldOf(252, 252, 0).X, worldOf(252, 252, 0).Y, -3000)
	require.NoError(t, err)
	require.EqualValues(t, -3000, height)

	// A reference height between the layers picks the closer deck.
	height, err = engine.ClosestHeight(
		worldOf(252, 252, 0).X, worldOf(252, 252, 0).Y, -2600)
	require.NoError(t, err)
	require.EqualValues(t, -2400, height,
		"the closer layer wins the resolution")

	// The flat ground outside the deck resolves to its single layer.
	height, err = engine.ClosestHeight(
		worldOf(150, 150, 0).X, worldOf(150, 150, 0).Y, -2400)
	require.NoError(t, err)
	require.EqualValues(t, -3000, height,
		"a single layer cell answers its own height")
}

// TestClosestHeightMissingGeodata: a position without a region file
// under it answers the region load error, the zone return keeps the
// walker height then.
func TestClosestHeightMissingGeodata(t *testing.T) {
	empty := NewEngine(t.TempDir())
	_, err := empty.ClosestHeight(100000, 140000, 0)
	require.Error(t, err)
}
