// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"testing"

	"github.com/melg8/swarm/internal/swarm/npcdata"
	"github.com/stretchr/testify/require"
)

// The quest chain data pins: every value below was read from the
// Mobius C1 quest scripts (Q00406, Q00407, ElfHumanFighterChange1),
// the npc stats xml (the mob names and levels), the spawn files
// (the stations and the kill grounds) and the quest page htm files
// (the link texts). The tests pin the data against the npcdata
// maps - a wrong display id or item id fails the name resolution.

// TestQuestNpcIdsResolveToNames pins the npc stations: the display
// ids resolve to the npc names of the npcdata map (the cross-check
// of the CT0_to_C4 mapping against the generated data).
func TestQuestNpcIdsResolveToNames(t *testing.T) {
	stations := map[string]QuestNpc{
		"Sorius":  soriusNPC(),
		"Kluto":   klutoNPC(),
		"Reisa":   reisaNPC(),
		"Moretti": morettiNPC(),
		"Prias":   priasNPC(),
		"Rains":   rainsNPC(),
	}
	for wantName, npc := range stations {
		got := npcdata.NPCName(npc.TemplateID + npcDisplayOffset)
		require.Equal(t, wantName, got,
			"the %s station display id resolves through npcdata",
			wantName)
	}
}

// TestQuestKillMobsResolveToNames pins the kill sets: every mob
// display id of both chains resolves to the mob name the script
// kills (the Ol Mahum Patrol correction included - 53 is the
// patrol, not the bugbear).
func TestQuestKillMobsResolveToNames(t *testing.T) {
	expected := map[int32]string{
		35:   "Tracker Skeleton",
		42:   "Tracker Skeleton Leader",
		45:   "Skeleton Scout",
		51:   "Skeleton Bowman",
		54:   "Ruin Spartoi",
		60:   "Raging Spartoi",
		53:   "Ol Mahum Patrol",
		782:  "Ol Mahum Novice",
		5031: "Ol Mahum Sentry",
	}
	for id, want := range expected {
		require.Equal(t, want, npcdata.NPCName(id+npcDisplayOffset),
			"the kill mob %d resolves through npcdata", id)
	}
}

// TestQuestItemsResolveToNames pins the quest item ids: the drops
// and the proof items resolve through the npcdata item names.
func TestQuestItemsResolveToNames(t *testing.T) {
	expected := map[int32]string{
		1202: "Sorius's Letter 1",
		1203: "Kluto Box",
		1204: "Elven Knight Brooch",
		1205: "Topaz Piece",
		1206: "Emerald Piece",
		1276: "Kluto's Memo",
		1293: "Rusted Key",
	}
	for id, want := range expected {
		require.Equal(t, want, npcdata.ItemName(id),
			"the quest item %d resolves through npcdata", id)
	}
}

// TestElvenKnightChainPinsTheScriptFacts pins the Q00406 ladder:
// the ids, the cond sequence, the kill economy (the chances and
// the targets read from the onKill branches), the stations and the
// accept route of the datapack pages.
func TestElvenKnightChainPinsTheScriptFacts(t *testing.T) {
	chain := ElvenKnightChain()
	require.Equal(t, int32(406), chain.QuestID)
	require.Equal(t, "Q00406_PathOfTheElvenKnight", chain.Script)
	require.Equal(t, int32(19), chain.MinLevel)
	require.Equal(t, int32(18), chain.ClassID)
	require.Equal(t, "Sorius", chain.Start.Name)
	require.Equal(t, int32(1204), chain.ProofItem)
	require.Equal(t, int64(3200), chain.RewardXP)
	require.Equal(t, int64(2280), chain.RewardSP)

	// The accept route: the 30327-01 link, then the 05 challenge
	// link (the 06 event starts the quest).
	require.Len(t, chain.Accept, 2)
	require.Equal(t, "Say you want to be an Elven Knight",
		chain.Accept[0].LinkText)
	require.Equal(t, "Challenge the test", chain.Accept[1].LinkText)

	// The cond ladder: 1..6, every stage exactly one action.
	require.Len(t, chain.Stages, 6)
	for i, stage := range chain.Stages {
		require.Equal(t, int32(i+1), stage.Cond,
			"the stages run cond 1..6 in order")
		isTalk := stage.TalkNpc.TemplateID != 0
		isKill := len(stage.Kill.Mobs) > 0
		require.NotEqual(t, isTalk, isKill,
			"stage %d is a talk or a kill stage, never both", stage.Cond)
	}

	// The skeleton hunt: the six Ruins of Agony species, 70
	// percent topaz, the 20th piece advances.
	s1, ok := QuestStageByCond(chain, 1)
	require.True(t, ok)
	require.Equal(t, []int32{35, 42, 45, 51, 54, 60}, s1.Kill.Mobs)
	require.Equal(t, []int32{1205}, s1.Kill.ItemIDs)
	require.Equal(t, 70, s1.Kill.DropPercent)
	require.Equal(t, int32(20), s1.Kill.Target)

	// The Kluto favor link at cond 3 (the bypass advance).
	s3, ok := QuestStageByCond(chain, 3)
	require.True(t, ok)
	require.Equal(t, "Kluto", s3.TalkNpc.Name)
	require.Len(t, s3.Links, 1)
	require.Equal(t, "Ask about the favor", s3.Links[0].LinkText)

	// The Ol Mahum Novice hunt: 50 percent emerald, 20 pieces.
	s4, ok := QuestStageByCond(chain, 4)
	require.True(t, ok)
	require.Equal(t, []int32{782}, s4.Kill.Mobs)
	require.Equal(t, 50, s4.Kill.DropPercent)
	require.Equal(t, int32(20), s4.Kill.Target)

	// The closing talk at Sorius.
	s6, ok := QuestStageByCond(chain, 6)
	require.True(t, ok)
	require.Equal(t, "Sorius", s6.TalkNpc.Name)
	require.Empty(t, s6.Links, "the talk itself closes the quest")

	_, ok = QuestStageByCond(chain, 7)
	require.False(t, ok, "the ladder ends at cond 6")
}

// TestElvenScoutChainPinsTheScriptFacts pins the Q00407 ladder:
// the one-link accept (the 05 event starts AND gives the letter),
// the cond sequence 1..8, the torn letter set (the four ids, the
// unconditional drops) and the sentry key hunt.
func TestElvenScoutChainPinsTheScriptFacts(t *testing.T) {
	chain := ElvenScoutChain()
	require.Equal(t, int32(407), chain.QuestID)
	require.Equal(t, "Q00407_PathOfTheElvenScout", chain.Script)
	require.Equal(t, int32(19), chain.MinLevel)
	require.Equal(t, int32(1217), chain.ProofItem)
	require.Equal(t, int64(3200), chain.RewardXP)
	require.Equal(t, int64(1000), chain.RewardSP)
	require.Equal(t, "Reisa", chain.Start.Name)

	// The accept route is ONE link: the 30328-05 event does the
	// whole start (startQuest + Reisa's Letter).
	require.Len(t, chain.Accept, 1)
	require.Equal(t, "Say you will accept the task",
		chain.Accept[0].LinkText)

	// The cond ladder: 1..8.
	require.Len(t, chain.Stages, 8)
	for i, stage := range chain.Stages {
		require.Equal(t, int32(i+1), stage.Cond,
			"the stages run cond 1..8 in order")
		isTalk := stage.TalkNpc.TemplateID != 0
		isKill := len(stage.Kill.Mobs) > 0
		require.NotEqual(t, isTalk, isKill,
			"stage %d is a talk or a kill stage, never both", stage.Cond)
	}

	// The Moretti briefing: the two link walk to the search
	// decision.
	s1, ok := QuestStageByCond(chain, 1)
	require.True(t, ok)
	require.Equal(t, "Moretti", s1.TalkNpc.Name)
	require.Len(t, s1.Links, 2)
	require.Equal(t, "Listen to details", s1.Links[0].LinkText)
	require.Equal(t, "Say you will begin the search",
		s1.Links[1].LinkText)

	// The patrol hunt: the four torn letter pieces drop one per
	// kill (no chance gate), the 4th piece advances.
	s2, ok := QuestStageByCond(chain, 2)
	require.True(t, ok)
	require.Equal(t, []int32{53}, s2.Kill.Mobs)
	require.Equal(t, []int32{1208, 1209, 1210, 1211}, s2.Kill.ItemIDs)
	require.Equal(t, 100, s2.Kill.DropPercent)
	require.Equal(t, int32(4), s2.Kill.Target)

	// The Prias legs: the cond 4 talk, the cond 5 sentry key (60
	// percent, one key).
	s4, ok := QuestStageByCond(chain, 4)
	require.True(t, ok)
	require.Equal(t, "Prias", s4.TalkNpc.Name)
	s5, ok := QuestStageByCond(chain, 5)
	require.True(t, ok)
	require.Equal(t, []int32{5031}, s5.Kill.Mobs)
	require.Equal(t, 60, s5.Kill.DropPercent)
	require.Equal(t, int32(1), s5.Kill.Target)

	// The closing talk at Reisa.
	s8, ok := QuestStageByCond(chain, 8)
	require.True(t, ok)
	require.Equal(t, "Reisa", s8.TalkNpc.Name)

	_, ok = QuestStageByCond(chain, 9)
	require.False(t, ok, "the ladder ends at cond 8")
}

// TestClassChangePinsTheScriptFacts pins the two class change legs:
// the master, the target class ids, the proof items and the route
// texts of the Rains pages (30288.htm -> 11 -> 12/15 -> the change
// links).
func TestClassChangePinsTheScriptFacts(t *testing.T) {
	knight := ElvenKnightClassChange()
	require.Equal(t, "Rains", knight.Npc.Name)
	require.Equal(t, int32(19), knight.TargetClass)
	require.Equal(t, int32(1204), knight.ProofItem)
	require.Equal(t, int32(20), knight.MinLevel)
	require.Len(t, knight.Route, 3)
	require.Equal(t, "Listen to information about first class transfer",
		knight.Route[0].LinkText)
	require.Equal(t, "Elven Knight", knight.Route[1].LinkText)
	require.Equal(t, "Change profession to an Elven Knight",
		knight.Route[2].LinkText)

	scout := ElvenScoutClassChange()
	require.Equal(t, int32(22), scout.TargetClass)
	require.Equal(t, int32(1217), scout.ProofItem)
	require.Len(t, scout.Route, 3)
	require.Equal(t, "Elven Scout", scout.Route[1].LinkText)
	require.Equal(t, "Change profession to an Elven Scout",
		scout.Route[2].LinkText)
}

// TestQuestChainAccessorsReturnFreshValues pins the no-shared-state
// contract: two accessor calls carry independent stage data (a
// caller mutating one chain never corrupts the other).
func TestQuestChainAccessorsReturnFreshValues(t *testing.T) {
	first := ElvenKnightChain()
	second := ElvenKnightChain()
	stage, ok := QuestStageByCond(first, 1)
	require.True(t, ok)
	stage.Kill.Mobs[0] = 9999
	stage.TalkNpc.Name = "corrupted"

	fresh, ok := QuestStageByCond(second, 1)
	require.True(t, ok)
	require.Equal(t, int32(35), fresh.Kill.Mobs[0])
	require.Empty(t, fresh.TalkNpc.Name)
}
