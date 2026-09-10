// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/melg8/swarm/internal/swarm/npcdata"
)

// AppendSnapshotJSON appends the JSON encoding of the whole current
// bot state to dst and returns the grown slice. The event stream and
// the state endpoint call it on their reusable payload buffers: the
// encoder walks the live state under the read lock and materializes
// no intermediate Snapshot - the per element view structs live on the
// stack of this call, so the steady state of a watched stream
// allocates nothing per event (the snapshot copy used to pay the
// object, combat, event and chat slice allocations on every version
// change, roughly 26 KB of garbage per event on a 100 npc bot). The
// output is byte identical to Snapshot().AppendJSON, pinned by
// TestAppendSnapshotJSONMatchesSnapshot and the reflection golden
// suite of snapshot_json_test.go.
func (b *Bot) AppendSnapshotJSON(dst []byte) []byte {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.appendSnapshotJSONLocked(dst, time.Now())
}

// appendSnapshotJSONLocked is the direct live state walk of the
// snapshot encoding. The caller must hold a lock; the arrays keep the
// exact nil semantics of the Snapshot view (the always present
// collections encode as empty arrays, only the optional walk plan and
// hunting zone fall back to null).
func (b *Bot) appendSnapshotJSONLocked(dst []byte, now time.Time) []byte {
	if dst == nil {
		dst = make([]byte, 0, b.snapshotJSONSizeLocked())
	}

	adena, inventorySlots := b.inventoryTotalsLocked()
	dst = append(dst, `{"id":`...)
	dst = appendJSONString(dst, b.id)
	dst = append(dst, `,"status":`...)
	dst = appendJSONString(dst, string(b.status))
	dst = append(dst, `,"phase":`...)
	dst = appendJSONString(dst, b.phase)
	dst = append(dst, `,"character":`...)
	dst = appendCharacterJSON(dst, b.characterSnapshotLocked(now, adena,
		inventorySlots))
	dst = append(dst, `,"inventory":[`...)
	dst = b.appendLiveInventoryJSON(dst)
	dst = append(dst, `],"objects":[`...)
	dst = b.appendLiveObjectsJSON(dst, now)
	dst = append(dst, `],"events":[`...)
	dst = b.appendLiveEventsJSON(dst)
	dst = append(dst, `],"chat":[`...)
	dst = b.appendLiveChatJSON(dst)
	dst = append(dst, `],"walkPath":`...)
	dst = b.appendLiveWalkPathJSON(dst)
	dst = append(dst, `,"shopping":`...)
	dst = b.appendLiveShoppingJSON(dst, now)
	dst = append(dst, `,"skills":`...)
	dst = b.appendLiveSkillsJSON(dst)
	dst = append(dst, `,"skillPlan":`...)
	dst = b.appendLiveSkillPlanJSON(dst)
	dst = append(dst, `,"combatEvents":[`...)
	dst = b.appendLiveCombatJSON(dst, now)
	dst = append(dst, `],"huntingZone":`...)
	dst = appendZoneJSON(dst, b.zone)
	dst = append(dst, `,"huntingZones":[`...)
	dst = b.appendLiveZoneViewsJSON(dst)
	dst = append(dst, `],"packets":`...)
	dst = strconv.AppendInt(dst, b.packets, 10)
	dst = append(dst, `,"version":`...)
	dst = strconv.AppendUint(dst, b.version, 10)
	dst = append(dst, `,"serverTimeMs":`...)
	dst = strconv.AppendInt(dst, now.UnixMilli(), 10)
	dst = append(dst, `,"startedAt":`...)
	dst = appendJSONTime(dst, b.started)
	dst = append(dst, `,"updatedAt":`...)
	dst = appendJSONTime(dst, b.updated)

	return append(dst, '}')
}

// appendLiveInventoryJSON writes the equipment widget array opened
// by the caller (the live view never falls back to null - the
// snapshot copy always materialized the collection). The caller must
// hold a lock.
func (b *Bot) appendLiveInventoryJSON(dst []byte) []byte {
	for i := range b.inventory.items {
		if i > 0 {
			dst = append(dst, ',')
		}
		item := b.inventory.items[i]
		stats, hasStats := npcdata.ItemGearStats(item.ItemID)
		itemType := stats.Type
		if !hasStats {
			itemType = npcdata.ItemType(item.ItemID)
		}
		dst = appendInventoryItemJSON(dst, InventoryItemSnapshot{
			ObjectID:    item.ObjectID,
			ItemID:      item.ItemID,
			Count:       item.Count,
			Type2:       item.Type2,
			Equipped:    item.Equipped,
			BodyPart:    item.BodyPart,
			Enchant:     item.Enchant,
			Name:        npcdata.ItemName(item.ItemID),
			Icon:        npcdata.ItemIcon(item.ItemID),
			Type:        itemType,
			WeaponType:  stats.WeaponType,
			ArmorType:   stats.ArmorType,
			BodyPartKey: stats.BodyPart,
			PAtk:        stats.PAtk,
			MAtk:        stats.MAtk,
			PDef:        stats.PDef,
			MDef:        stats.MDef,
			SDef:        stats.SDef,
			RShld:       stats.RShld,
			PAtkSpd:     stats.PAtkSpd,
			SoulShots:   stats.SoulShots,
			SpiritShots: stats.SpiritShots,
			Weight:      npcdata.ItemWeight(item.ItemID),
			Price:       npcdata.ItemPrice(item.ItemID),
		})
	}

	return dst
}

// appendLiveObjectsJSON writes the world object array opened by the
// caller. The caller must hold a lock.
func (b *Bot) appendLiveObjectsJSON(dst []byte, now time.Time) []byte {
	nowNano := now.UnixNano()
	for i := range b.world.hot {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = appendObjectJSON(dst, b.objectSnapshotLocked(i, nowNano))
	}

	return dst
}

// appendLiveEventsJSON writes the newest snapshotEvents rolling log
// entries opened by the caller, in chronological order exactly like
// eventLog.appendNewest. The caller must hold a lock.
func (b *Bot) appendLiveEventsJSON(dst []byte) []byte {
	count := min(b.log.length, snapshotEvents)
	for i := count; i > 0; i-- {
		index := (b.log.head - i + eventCapacity) % eventCapacity
		if i != count {
			dst = append(dst, ',')
		}
		dst = appendEventJSON(dst, b.log.ring[index])
	}

	return dst
}

// appendLiveChatJSON writes the whole chat window opened by the
// caller, in chronological order exactly like chatLog.appendAll. The
// caller must hold a lock.
func (b *Bot) appendLiveChatJSON(dst []byte) []byte {
	for i := range b.chat.length {
		if i > 0 {
			dst = append(dst, ',')
		}
		index := (b.chat.head - b.chat.length + i + chatCapacity) %
			chatCapacity
		dst = appendChatEventJSON(dst, b.chat.ring[index])
	}

	return dst
}

// appendLiveWalkPathJSON writes the published walk plan (null when
// none is fresh) exactly like the Snapshot view. The caller must hold
// a lock.
func (b *Bot) appendLiveWalkPathJSON(dst []byte) []byte {
	if b.walkPath == nil || time.Since(b.walkPathAt) > walkPlanTTL {
		return append(dst, `null`...)
	}
	dst = append(dst, '[')
	for i := range b.walkPath {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = appendWalkPointJSON(dst, b.walkPath[i])
	}

	return append(dst, ']')
}

// appendLiveShoppingJSON writes the published shopping plan (null
// when none is fresh) exactly like the Snapshot view. The caller
// must hold a lock.
func (b *Bot) appendLiveShoppingJSON(dst []byte, now time.Time) []byte {
	if !b.shoppingPlanLive(now) {
		return append(dst, `null`...)
	}

	return appendShoppingPlanJSON(dst, b.shopping)
}

// appendLiveSkillsJSON writes the learned skill list (null when the
// character knows no skills, the nil semantics of the Snapshot view).
// The caller must hold a lock.
func (b *Bot) appendLiveSkillsJSON(dst []byte) []byte {
	if len(b.skills) == 0 {
		return append(dst, `null`...)
	}
	dst = append(dst, '[')
	first := true
	for _, id := range b.sortedSkillIDsLocked() {
		if !first {
			dst = append(dst, ',')
		}
		first = false
		skill := b.skills[id]
		snapshot := SkillSnapshot{
			SkillID: id,
			Level:   skill.level,
			Passive: skill.passive,
			Name:    fmt.Sprintf("skill #%d", id),
		}
		if info, ok := npcdata.SkillInfoOf(id); ok {
			snapshot.Name = info.Name
			snapshot.Icon = info.Icon
			snapshot.Passive = info.Passive
		}
		dst = appendSkillSnapshotJSON(dst, snapshot)
	}

	return append(dst, ']')
}

// sortedSkillIDsLocked returns the learned skill ids in ascending
// order (the Snapshot view order). The caller must hold a lock.
func (b *Bot) sortedSkillIDsLocked() []int32 {
	ids := make([]int32, 0, len(b.skills))
	for id := range b.skills {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	return ids
}

// appendLiveSkillPlanJSON writes the learning queue (null when the
// class is unknown or nothing is left to learn) exactly like the
// Snapshot view. The affordability flag is computed inline against
// the SP read under the same lock - the stored queue never carries a
// valid flag. The caller must hold a lock.
func (b *Bot) appendLiveSkillPlanJSON(dst []byte) []byte {
	b.ensureSkillQueueLocked()
	if len(b.skillQueue) == 0 {
		return append(dst, `null`...)
	}
	sp := int64(b.char.Sp)
	total, missing := skillTotals(b.skillQueue, sp)
	dst = append(dst, `{"sp":`...)
	dst = strconv.AppendInt(dst, sp, 10)
	dst = append(dst, `,"total":`...)
	dst = strconv.AppendInt(dst, total, 10)
	dst = append(dst, `,"missing":`...)
	dst = strconv.AppendInt(dst, missing, 10)
	dst = append(dst, `,"entries":[`...)
	for i := range b.skillQueue {
		if i > 0 {
			dst = append(dst, ',')
		}
		entry := b.skillQueue[i]
		entry.Affordable = sp >= int64(entry.SpCost)
		dst = appendSkillPlanEntryJSON(dst, entry)
	}
	dst = append(dst, `]}`...)

	return dst
}

// appendLiveCombatJSON writes the combat animation beats of the TTL
// window opened by the caller, in chronological order exactly like
// combatFeed.appendView. The caller must hold a lock.
func (b *Bot) appendLiveCombatJSON(dst []byte, now time.Time) []byte {
	cut := now.Add(-combatEventTTL)
	first := true
	for _, ev := range b.combat.events {
		if ev.At.Before(cut) {
			continue
		}
		if !first {
			dst = append(dst, ',')
		}
		first = false
		dst = appendCombatEventViewJSON(dst, CombatEventView{
			Seq:        ev.Seq,
			Kind:       ev.Kind,
			AttackerID: ev.AttackerID,
			TargetID:   ev.TargetID,
			Amount:     ev.Amount,
			AtMs:       ev.At.UnixMilli(),
			X:          ev.X,
			Y:          ev.Y,
			TargetX:    ev.TargetX,
			TargetY:    ev.TargetY,
		})
	}

	return dst
}

// appendLiveZoneViewsJSON writes the zone registry array opened by
// the caller. The caller must hold a lock.
func (b *Bot) appendLiveZoneViewsJSON(dst []byte) []byte {
	for i := range b.zoneViews {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = appendZoneViewJSON(dst, b.zoneViews[i])
	}

	return dst
}

// characterSnapshotLocked builds the character view of the snapshot
// from the live state. The value stays on the stack of the caller
// (the append functions read it by value). The caller must hold a
// lock.
func (b *Bot) characterSnapshotLocked(
	now time.Time, adena int32, inventorySlots int,
) CharacterSnapshot {
	return CharacterSnapshot{
		ObjectID:        b.selfID,
		Name:            b.char.Name,
		TargetID:        b.char.TargetID,
		Moving:          b.char.Moving,
		DestX:           b.char.DestX,
		DestY:           b.char.DestY,
		DestZ:           b.char.DestZ,
		Speed:           b.char.RunSpeed,
		CollisionRadius: b.char.CollisionRadius,
		SocialUntilMs:   b.char.SocialUntil.UnixMilli(),
		MoveAtMs:        b.char.MoveAt.UnixMilli(),
		Level:           b.char.Level,
		Race:            b.char.Race,
		ClassID:         b.char.ClassID,
		X:               b.char.X,
		Y:               b.char.Y,
		Z:               b.char.Z,
		Heading:         b.char.Heading,
		CurHP:           b.char.CurHP,
		MaxHP:           b.char.MaxHP,
		CurMP:           b.char.CurMP,
		MaxMP:           b.char.MaxMP,
		Sitting:         b.char.Sitting,
		STR:             b.char.STR,
		DEX:             b.char.DEX,
		CON:             b.char.CON,
		INT:             b.char.INT,
		WIT:             b.char.WIT,
		MEN:             b.char.MEN,
		Exp:             b.char.Exp,
		ExpPercent:      ExpPercent(b.char.Level, int64(b.char.Exp)),
		Sp:              b.char.Sp,
		InCombat:        b.char.inCombat(now),
		CurrentLoad:     b.char.CurrentLoad,
		MaxLoad:         b.char.MaxLoad,
		InventorySlots:  inventorySlots,
		InventoryMax:    inventorySlotLimit,
		Adena:           adena,
	}
}

// objectSnapshotLocked builds the object view of the snapshot from
// the live hot and cold records. The value stays on the stack of the
// caller. The caller must hold a lock.
func (b *Bot) objectSnapshotLocked(slot int, nowNano int64) ObjectSnapshot {
	hot := &b.world.hot[slot]
	cold := &b.world.cold[slot]

	return ObjectSnapshot{
		ObjectID:        hot.ObjectID,
		Kind:            kindString(hot.Kind),
		Name:            cold.Name,
		Title:           cold.Title,
		TemplateID:      cold.TemplateID,
		Attackable:      hot.Attackable,
		Aggressive:      hot.Aggressive,
		AggroRange:      cold.AggroRange,
		Level:           hot.Level,
		TargetID:        hot.TargetID,
		InCombat:        hot.inCombat(nowNano),
		Dead:            hot.Dead,
		Moving:          hot.Moving,
		Running:         hot.Running,
		Speed:           hot.effectiveSpeed(),
		CollisionRadius: cold.CollisionRadius,
		SocialUntilMs:   unixMilliFromNano(cold.SocialUntil),
		Count:           cold.Count,
		X:               hot.X,
		Y:               hot.Y,
		Z:               hot.Z,
		Heading:         cold.Heading,
		DestX:           hot.DestX,
		DestY:           hot.DestY,
		DestZ:           hot.DestZ,
		MoveAtMs:        unixMilliFromNano(hot.MoveAt),
		CurHP:           cold.CurHP,
		MaxHP:           cold.MaxHP,
		CurMP:           cold.CurMP,
		MaxMP:           cold.MaxMP,
	}
}

// inventoryTotalsLocked returns the adena sum and the slot count of
// the dense inventory store: the two aggregate fields of the
// character view. The caller must hold a lock.
func (b *Bot) inventoryTotalsLocked() (adena int32, slots int) {
	for _, item := range b.inventory.items {
		if item.Type2 == itemType2Adena {
			adena += item.Count
		}
	}

	return adena, len(b.inventory.items)
}

// snapshotJSONSizeLocked estimates the encoded length of the live
// state (the same rough per entry costs as snapshotJSONSize), so a
// one shot encode keeps everything in one allocation. The caller must
// hold a lock.
func (b *Bot) snapshotJSONSizeLocked() int {
	size := 640 + len(b.id) + len(b.phase) + len(b.status) +
		len(b.char.Name)
	size += 768 * len(b.inventory.items)
	size += 384 * len(b.world.hot)
	count := min(b.log.length, snapshotEvents)
	size += 96 * count
	size += 96 * b.chat.length
	size += 24 * len(b.walkPath)
	if b.shopping != nil {
		size += 256 * len(b.shopping.Entries)
	}
	size += 64 * len(b.skills)
	size += 160 * len(b.skillQueue)
	size += 160 * len(b.combat.events)
	size += 160 * len(b.zoneViews)
	for i := count; i > 0; i-- {
		index := (b.log.head - i + eventCapacity) % eventCapacity
		size += len(b.log.ring[index].Message)
	}
	for i := range b.chat.length {
		index := (b.chat.head - b.chat.length + i + chatCapacity) %
			chatCapacity
		size += len(b.chat.ring[index].Text) +
			len(b.chat.ring[index].Kind)
	}

	return size
}
