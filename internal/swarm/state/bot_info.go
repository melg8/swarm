// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
    "strconv"

    "github.com/melg8/swarm/internal/swarm/npcdata"
)

// Bot kind constants: the role of a bot in the process. The empty kind (the
// default of NewBot) reads as a regular long-running bot; KindLongRunning lets
// the main entry tag the fleet bots and KindAcceptance lets the acceptance
// manager tag its temp bots, so the sidebar splits them into two groups.
const (
    KindLongRunning = "long-running"
    KindAcceptance  = "acceptance"
)

// BotInfo is the compact JSON view used by the bot list endpoint. The
// vitals and the combat flag let the sidebar show the mini HP/MP/XP bars
// and the fighting state of every session at a glance.
type BotInfo struct {
    ID     string `json:"id"`
    Name   string `json:"name"`
    Status Status `json:"status"`
    // Phase mirrors the hunt loop phase so the sidebar bot row can
    // show the same activity banner as the map HUD. Empty for the
    // manual only sessions and the pre-world sessions.
    Phase string `json:"phase"`
    // Kind is the role of the bot in the process: KindLongRunning
    // for the 24/7 fleet bots, KindAcceptance for the temp bots of
    // the acceptance test manager. The empty string is the default
    // of an untagged bot and reads as long-running.
    Kind  string `json:"kind"`
    Level int32  `json:"level"`
    // DelevelTarget is the level the running deleveling drops the character to
    // (non zero only while the delevel phase runs): the sidebar row renders it
    // in the compact activity banner ("deleveling -> lv 9").
    DelevelTarget int32   `json:"delevelTarget"`
    CurHP         float64 `json:"curHp"`
    MaxHP         float64 `json:"maxHp"`
    CurMP         float64 `json:"curMp"`
    MaxMP         float64 `json:"maxMp"`
    ExpPercent    float64 `json:"expPercent"`
    InCombat      bool    `json:"inCombat"`
    Sitting       bool    `json:"sitting"`
}

// delevelTarget reads the published deleveling target of the hunt
// diagnostics: non zero only while the delevel phase runs (the loop
// clears the delevel block of the diagnostics outside the phase).
func (b *Bot) delevelTarget() int32 {
    if !b.hunt.DelevelActive {
        return 0
    }

    return b.hunt.DelevelTarget
}

func (b *Bot) Info() BotInfo {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return BotInfo{
        ID:            b.id,
        Name:          b.char.Name,
        Status:        b.status,
        Phase:         b.phase,
        Kind:          b.kind,
        Level:         b.char.Level,
        DelevelTarget: b.delevelTarget(),
        CurHP:         b.char.CurHP,
        MaxHP:         b.char.MaxHP,
        CurMP:         b.char.CurMP,
        MaxMP:         b.char.MaxMP,
        ExpPercent:    ExpPercent(b.char.Level, int64(b.char.Exp)),
        InCombat:      b.char.inCombat(b.now()),
        Sitting:       b.char.Sitting,
    }
}

// bowWeaponType is the weapon family of a bow in the item stats dictionary.
const bowWeaponType = "BOW"

// npcBowWielding resolves whether the NpcInfo right hand item id is
// a bow: the equipment int carries the template rhand item id and
// the generated item stats name the weapon family. An unknown or
// zero id (a natural weapon monster) is not a bow.
func npcBowWielding(rightHandItem int32) bool {
    if rightHandItem <= 0 {
        return false
    }
    stats, ok := npcdata.ItemGearStats(rightHandItem)

    return ok && stats.WeaponType == bowWeaponType
}

// resolveNpcName prefers the server side name and falls back to the
// generated npc dictionary for client side names.
func resolveNpcName(name string, templateID int32) string {
    if name != "" {
        return name
    }
    resolved := npcdata.NPCName(templateID)
    if resolved != "" {
        return resolved
    }

    return "npc #" + strconv.Itoa(int(templateID-npcdata.NPCDisplayOffset))
}

// itemName formats the item name with the display id fallback.
func itemName(name string, displayID int32) string {
    if name != "" {
        return name
    }

    return "#" + strconv.Itoa(int(displayID))
}
