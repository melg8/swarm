// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package npcdata

import "sort"

// NPCCombat is the combat and adena profile of one npc template: the hp
// prices the fight time at the measured damage rate, the physical attack the
// incoming damage and the mean adena the drop curve of the ground economy
// (npc_combat_table.go holds the generated rows).
type NPCCombat struct {
    // TemplateID is the npc stats (CT0 xml) template id, the key the hunt
    // registries carry.
    TemplateID int32
    // Level is the npc level of the stats.
    Level int32
    // HP is the base hp of the template.
    HP float64
    // PAtk and PDef are the physical attack and defence of the template.
    PAtk float64
    PDef float64
    // Exp is the experience reward of one kill at the full rate.
    Exp int32
    // Adena is the mean adena of one kill at the full drop: the group chance
    // times the amount range of the adena item.
    Adena float64
}

// noCombat is the not-found answer (exhaustruct wants the plain var).
var noCombat NPCCombat

// NPCCombatProfile resolves the combat profile by the npc stats (CT0 xml)
// template id, false for an unknown template. The hunt registries and the
// cell mob lists carry these ids; an NpcInfo wire id carries the 1000000
// display offset instead (NPCWireTemplateID maps the other way).
func NPCCombatProfile(templateID int32) (NPCCombat, bool) {
    index := sort.Search(len(npcCombatTable), func(index int) bool {
        return npcCombatTable[index].TemplateID >= templateID
    })
    if index == len(npcCombatTable) ||
        npcCombatTable[index].TemplateID != templateID {
        return noCombat, false
    }

    return npcCombatTable[index], true
}
