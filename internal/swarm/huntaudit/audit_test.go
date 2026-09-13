// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package huntaudit

import (
    "encoding/json"
    "os"
    "path/filepath"
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// newCollectBot builds a tracker whose knownlist holds the observed
// population of the spot-54 ground: the spider mass west of the leash
// border, a dryad inside the square and a friendly villager.
func newCollectBot(t *testing.T) *state.Bot {
    t.Helper()
    bot := state.NewBot("probe")
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 9001, TemplateID: 1000460, Attackable: true,
        X: 14703, Y: 51897, Z: -3552,
    })
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 9002, TemplateID: 1000019, Attackable: true,
        X: 15600, Y: 52000, Z: -3600,
    })
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 9003, TemplateID: 1003016, Attackable: false,
        X: 15800, Y: 52100, Z: -3600,
    })

    return bot
}

func TestCollectNpcsVerdictsUseThePolygonLeash(t *testing.T) {
    bot := newCollectBot(t)
    // A cell polygon around the observation field: the western
    // spiders fall inside, the eastern dryad outside.
    leash := state.NewCellZone([]state.ZoneVertex{
        {X: 14600, Y: 51300},
        {X: 14900, Y: 51300},
        {X: 14900, Y: 51950},
        {X: 14600, Y: 51950},
    })
    npcs := collectNpcs(bot, 14750, 51625, leash)
    // The friendly villager is not attackable: it never records.
    require.Len(t, npcs, 2)
    require.True(t, npcs[0].InLeash)
    require.False(t, npcs[1].InLeash)
    require.InDelta(t, 276.0, npcs[0].Distance, 1.0)
}

func TestAuditFileRoundTripAndResume(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "audit.json")
    audit := &AuditFile{Account: "probe", WaitSec: 12}
    audit.Spots = append(audit.Spots, SpotRecord{
        ID: "elven-spot-53", Name: "Kaboo Orc Fighter Leader W",
        AnchorX: 16884, AnchorY: 52856, Radius: 2048,
        LeashHalf: 1448, Attackable: 3, InLeashCount: 1,
        Npcs: []NpcRecord{{
            ObjectID: 268439506, Name: "Kaboo Orc Fighter Leader",
            WireID: 1000472, Level: 12, X: 19053, Y: 53166, Z: -3520,
            Distance: 2191, InLeash: false, Attackable: true, Alive: true,
        }},
    })
    require.NoError(t, writeAudit(path, "probe", 12*time.Second, audit))
    loaded, err := loadAudit(path, false)
    require.NoError(t, err)
    require.Equal(t, "probe", loaded.Account)
    require.Len(t, loaded.Spots, 1)
    require.Equal(t, "elven-spot-53", loaded.Spots[0].ID)
    require.Equal(t, int32(19053), loaded.Spots[0].Npcs[0].X)
    // The fresh flag ignores the resume state.
    fresh, err := loadAudit(path, true)
    require.NoError(t, err)
    require.Empty(t, fresh.Spots)
    // A missing file starts an empty audit.
    missing, err := loadAudit(filepath.Join(dir, "none.json"), false)
    require.NoError(t, err)
    require.Empty(t, missing.Spots)
}

func TestLoadAnchors(t *testing.T) {
    dir := t.TempDir()
    // No path: no overrides.
    anchors, err := loadAnchors("")
    require.NoError(t, err)
    require.Empty(t, anchors)
    // The verification pass file: spot ids map onto positions.
    path := filepath.Join(dir, "suggested.json")
    payload := map[string]map[string]map[string]int32{
        "spots": {
            "elven-spot-54": {"x": 14650, "y": 51500, "z": -3620, "radius": 1656},
        },
    }
    data, err := json.Marshal(payload)
    require.NoError(t, err)
    require.NoError(t, os.WriteFile(path, data, 0o600))
    anchors, err = loadAnchors(path)
    require.NoError(t, err)
    require.Len(t, anchors, 1)
    override := anchors["elven-spot-54"]
    require.Equal(t, int32(14650), override.X)
    require.Equal(t, int32(51500), override.Y)
    require.Equal(t, int32(-3620), override.Z)
    require.Equal(t, int32(1656), override.Radius)
    // A broken file fails honestly.
    require.NoError(t, os.WriteFile(path, []byte("{not json"), 0o600))
    _, err = loadAnchors(path)
    require.Error(t, err)
}

func TestCollectNpcsFiltersAndVerdicts(t *testing.T) {
    tracker := newCollectBot(t)
    // The spot-54 anchor with its square leash inscribed in the
    // 1656 radius: the western spider mass falls outside, the
    // eastern dryad inside.
    half := int32(1171)
    leash := state.NewCellZone([]state.ZoneVertex{
        {X: 15936 - half, Y: 52218 - half},
        {X: 15936 + half, Y: 52218 - half},
        {X: 15936 + half, Y: 52218 + half},
        {X: 15936 - half, Y: 52218 + half},
    })
    npcs := collectNpcs(tracker, 15936, 52218, leash)
    // The friendly villager never lands in the dump, the attackable
    // mob does with its leash verdict.
    require.Len(t, npcs, 2)
    byID := map[int32]NpcRecord{}
    for _, npc := range npcs {
        byID[npc.ObjectID] = npc
    }
    spider := byID[9001]
    require.Equal(t, "Crimson Spider", spider.Name)
    require.InDelta(t, 1274.0, spider.Distance, 1.0)
    require.False(t, spider.InLeash)
    require.True(t, spider.Attackable)
    require.True(t, spider.Alive)
    require.Equal(t, int32(15), spider.Level)
    dryad := byID[9002]
    require.True(t, dryad.InLeash)
    require.InDelta(t, 400.0, dryad.Distance, 1.0)
}
