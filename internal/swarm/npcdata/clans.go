// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package npcdata

import (
	"sort"
	"strings"
)

// clanAllName is the special clan that matches every clan in the
// Mobius assist call.
const clanAllName = "ALL"

// ClanMaskAll is the marker bit of the ALL clan: an npc that carries
// it matches every clan in the assist check of the tracker (the
// Mobius AttackableAI clan call semantics).
const ClanMaskAll = uint64(1) << 63

// npcClanSets carries the clan lists pre-split per display id: the
// split of the joined dictionary value ran once at package init
// instead of allocating per NpcInfo packet. The lists are shared and
// read-only - callers must never mutate them.
var npcClanSets = buildNpcClanSets()

// npcClanMasks carries the clan bitmask per display id: bit i stands
// for the i-th clan of the sorted clan alphabet (44 clans of the pack
// fit into the low 44 bits), ClanMaskAll marks the ALL clan. A zero
// mask means the npc belongs to no clan.
var npcClanMasks = buildNpcClanMasks()

// NPCClanMask resolves the clan bitmask of an npc by the raw NpcInfo
// template id (display id + 1000000): two npcs whose masks share a
// bit belong to the same clan, a mask carrying ClanMaskAll matches
// every clan, and a zero mask belongs to no clan. The tracker uses
// the mask for the social pull check of the target search without
// any string work.
func NPCClanMask(templateID int32) uint64 {
	if templateID <= npcTemplateOffset {
		return 0
	}

	return npcClanMasks[templateID-npcTemplateOffset]
}

// buildNpcClanSets splits the joined clan strings of the generated
// dictionary once at package init.
func buildNpcClanSets() map[int32][]string {
	sets := make(map[int32][]string, len(npcClans))
	for id, joined := range npcClans {
		if joined == "" {
			continue
		}
		sets[id] = strings.Split(joined, " ")
	}

	return sets
}

// buildNpcClanMasks assigns one bit of a uint64 to every clan name of
// the pack (deterministic: the sorted alphabet) and folds the clan
// list of every npc into its mask.
func buildNpcClanMasks() map[int32]uint64 {
	names := make([]string, 0, 64)
	seen := make(map[string]struct{}, 64)
	for _, clans := range npcClanSets {
		for _, clan := range clans {
			if _, dup := seen[clan]; dup {
				continue
			}
			seen[clan] = struct{}{}
			names = append(names, clan)
		}
	}
	sort.Strings(names)
	bits := make(map[string]uint64, len(names))
	for i, clan := range names {
		if clan == clanAllName {
			bits[clan] = ClanMaskAll

			continue
		}
		bits[clan] = uint64(1) << uint(i)
	}
	masks := make(map[int32]uint64, len(npcClanSets))
	for id, clans := range npcClanSets {
		var mask uint64
		for _, clan := range clans {
			mask |= bits[clan]
		}
		masks[id] = mask
	}

	return masks
}
