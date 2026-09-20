// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "math"
    "testing"

    "github.com/melg8/swarm/internal/swarm/npcdata"
    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/stretchr/testify/require"
)

// TestWorldStandTableJoinsTheSpawnData pins the join of the generated
// world stand table against the generated spawn table: every row
// names a real world merchant spawn, every stand sits within the
// server interaction distance of its spawn (3D, the same gate the
// interaction machinery measures), every stand differs from its spawn
// (the rows the spawn serves stay out - the fallback answers them)
// and the curated counter round table keeps its own keys.
func TestWorldStandTableJoinsTheSpawnData(t *testing.T) {
    for id, stand := range merchantStandsWorld {
        spawn, ok := npcdata.MerchantSpawnOf(id)
        require.True(t, ok,
            "the world row %d must join a spawn row", id)
        _, curated := merchantStands[id]
        require.False(t, curated,
            "the curated row %d must stay out of the world table", id)
        spawnPoint := pathfind.Vec3{
            X: float64(spawn.X), Y: float64(spawn.Y),
            Z: float64(spawn.Z),
        }
        d2d := math.Hypot(stand.X-spawnPoint.X, stand.Y-spawnPoint.Y)
        require.LessOrEqual(t, d2d, 250.0,
            "the stand of %s must sit within the interaction "+
                "distance of its spawn", spawn.Name)
        dz := math.Abs(stand.Z - spawnPoint.Z)
        require.LessOrEqual(t, dz, 96.0,
            "the stand of %s must sit on the spawn's floor scale",
            spawn.Name)
        require.Greater(t, d2d, 0.0,
            "the stand of %s must differ from its spawn",
            spawn.Name)
    }
}

// TestMerchantStandPointResolvesEveryWorldMerchant pins the approach
// point resolution of every world merchant spawn: the world rows
// answer their table stand, the curated ids their curated row and the
// spawn served merchants their spawn - no world merchant resolves to
// a zero point, the walk planning always receives an approach point.
func TestMerchantStandPointResolvesEveryWorldMerchant(t *testing.T) {
    for _, id := range npcdata.MerchantTemplateIDs() {
        spawn, ok := npcdata.MerchantSpawnOf(id)
        require.True(t, ok)
        npc := townNpc{
            TemplateID: id,
            Name:       spawn.Name,
            X:          spawn.X, Y: spawn.Y, Z: spawn.Z,
        }
        stand := merchantStandPoint(npc)
        require.NotEqual(t, pathfind.Vec3{}, stand,
            "the merchant %s must resolve an approach point",
            spawn.Name)
        if world, ok := merchantStandsWorld[id]; ok {
            require.Equal(t, world, stand,
                "the world row of %s must answer as is",
                spawn.Name)

            continue
        }
        if curated, ok := merchantStands[id]; ok {
            require.Equal(t, curated, stand,
                "the curated row of %s must answer as is",
                spawn.Name)

            continue
        }
        require.InDelta(t, float64(spawn.X), stand.X, 0.001)
        require.InDelta(t, float64(spawn.Y), stand.Y, 0.001)
        require.InDelta(t, float64(spawn.Z), stand.Z, 0.001)
    }
}
