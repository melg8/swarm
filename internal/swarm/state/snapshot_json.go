// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import "strconv"

// The snapshot encoding is hand rolled: the event stream marshals the
// whole world view on every state version change (effectively on every
// received packet while a web client watches), and the reflection walk
// of encoding/json dominated that path (a 200 npc snapshot spent
// 250 us and 104 allocations in the reflector). The append functions
// below write the exact same bytes encoding/json produces for the
// snapshot types - field order, number formatting, string escaping
// (HTML escaping included) and time formatting match the reflection
// output, pinned by TestSnapshotJSONMatchesReflection.

// MarshalJSON encodes the snapshot with the direct writer. The json
// package calls it for every json.Marshal/json.Encode of a Snapshot.
func (s Snapshot) MarshalJSON() ([]byte, error) {
	buf := make([]byte, 0, snapshotJSONSize(s))

	return appendSnapshotJSON(buf, s), nil
}

// AppendJSON appends the snapshot encoding to dst - the streaming
// paths call it directly on a pre-sized buffer and skip the compacting
// copy json.Marshal runs over the MarshalJSON result. A nil dst gets
// the estimated output size allocated up front (one allocation for
// the whole document).
func (s Snapshot) AppendJSON(dst []byte) []byte {
	if dst == nil {
		dst = make([]byte, 0, snapshotJSONSize(s))
	}

	return appendSnapshotJSON(dst, s)
}

// snapshotJSONSize estimates the encoded length: the fixed frame plus
// rough per entry costs and the raw string lengths, so the append
// writer keeps everything in one allocation.
func snapshotJSONSize(s Snapshot) int {
	size := 640 + len(s.ID) + len(s.Phase) + len(s.Status) +
		len(s.Character.Name)
	size += 768 * len(s.Inventory)
	size += 384 * len(s.Objects)
	size += 96 * len(s.Events)
	size += 96 * len(s.Chat)
	size += 24 * len(s.WalkPath)
	if s.Shopping != nil {
		size += 256 * len(s.Shopping.Entries)
	}
	size += 160 * len(s.Skills)
	if s.SkillPlan != nil {
		size += 256 * len(s.SkillPlan.Entries)
	}
	size += 160 * len(s.CombatEvents)
	size += 160 * len(s.HuntingZones)
	for i := range s.Events {
		size += len(s.Events[i].Message)
	}
	for i := range s.Chat {
		size += len(s.Chat[i].Text) + len(s.Chat[i].Kind)
	}

	return size
}

// appendSnapshotJSON writes the snapshot object in the field order of
// the struct declaration.
func appendSnapshotJSON(dst []byte, s Snapshot) []byte {
	dst = append(dst, `{"id":`...)
	dst = appendJSONString(dst, s.ID)
	dst = append(dst, `,"status":`...)
	dst = appendJSONString(dst, string(s.Status))
	dst = append(dst, `,"phase":`...)
	dst = appendJSONString(dst, s.Phase)
	dst = append(dst, `,"character":`...)
	dst = appendCharacterJSON(dst, s.Character)
	dst = append(dst, `,"inventory":`...)
	dst = appendInventoryJSON(dst, s.Inventory)
	dst = append(dst, `,"objects":`...)
	dst = appendObjectsJSON(dst, s.Objects)
	dst = append(dst, `,"events":`...)
	dst = appendEventsJSON(dst, s.Events)
	dst = append(dst, `,"chat":`...)
	dst = appendChatJSON(dst, s.Chat)
	dst = append(dst, `,"walkPath":`...)
	dst = appendWalkPathJSON(dst, s.WalkPath)
	dst = append(dst, `,"shopping":`...)
	dst = appendShoppingPlanJSON(dst, s.Shopping)
	dst = append(dst, `,"skills":`...)
	dst = appendSkillsJSON(dst, s.Skills)
	dst = append(dst, `,"skillPlan":`...)
	dst = appendSkillPlanJSON(dst, s.SkillPlan)
	dst = append(dst, `,"combatEvents":`...)
	dst = appendCombatEventsJSON(dst, s.CombatEvents)
	dst = append(dst, `,"huntingZone":`...)
	dst = appendZoneJSON(dst, s.HuntingZone)
	dst = append(dst, `,"huntingZones":`...)
	dst = appendZoneViewsJSON(dst, s.HuntingZones)
	dst = append(dst, `,"packets":`...)
	dst = strconv.AppendInt(dst, s.Packets, 10)
	dst = append(dst, `,"version":`...)
	dst = strconv.AppendUint(dst, s.Version, 10)
	dst = append(dst, `,"serverTimeMs":`...)
	dst = strconv.AppendInt(dst, s.ServerTimeMs, 10)
	dst = append(dst, `,"startedAt":`...)
	dst = appendJSONTime(dst, s.StartedAt)
	dst = append(dst, `,"updatedAt":`...)
	dst = appendJSONTime(dst, s.UpdatedAt)
	dst = append(dst, '}')

	return dst
}

// appendCharacterJSON writes the character view object. Splitting
// the linear field walk only obscures the wire format; the field
// order mirrors the struct declaration like the reflection encoder.
//
//nolint:funlen // linear field order
func appendCharacterJSON(dst []byte, c CharacterSnapshot) []byte {
	dst = append(dst, `{"objectId":`...)
	dst = strconv.AppendInt(dst, int64(c.ObjectID), 10)
	dst = append(dst, `,"name":`...)
	dst = appendJSONString(dst, c.Name)
	dst = append(dst, `,"targetId":`...)
	dst = strconv.AppendInt(dst, int64(c.TargetID), 10)
	dst = append(dst, `,"moving":`...)
	dst = strconv.AppendBool(dst, c.Moving)
	dst = append(dst, `,"destX":`...)
	dst = strconv.AppendInt(dst, int64(c.DestX), 10)
	dst = append(dst, `,"destY":`...)
	dst = strconv.AppendInt(dst, int64(c.DestY), 10)
	dst = append(dst, `,"destZ":`...)
	dst = strconv.AppendInt(dst, int64(c.DestZ), 10)
	dst = append(dst, `,"speed":`...)
	dst = appendJSONFloat(dst, c.Speed)
	dst = append(dst, `,"collisionRadius":`...)
	dst = appendJSONFloat(dst, c.CollisionRadius)
	dst = append(dst, `,"socialUntilMs":`...)
	dst = strconv.AppendInt(dst, c.SocialUntilMs, 10)
	dst = append(dst, `,"moveAtMs":`...)
	dst = strconv.AppendInt(dst, c.MoveAtMs, 10)
	dst = append(dst, `,"level":`...)
	dst = strconv.AppendInt(dst, int64(c.Level), 10)
	dst = append(dst, `,"race":`...)
	dst = strconv.AppendInt(dst, int64(c.Race), 10)
	dst = append(dst, `,"classId":`...)
	dst = strconv.AppendInt(dst, int64(c.ClassID), 10)
	dst = append(dst, `,"x":`...)
	dst = strconv.AppendInt(dst, int64(c.X), 10)
	dst = append(dst, `,"y":`...)
	dst = strconv.AppendInt(dst, int64(c.Y), 10)
	dst = append(dst, `,"z":`...)
	dst = strconv.AppendInt(dst, int64(c.Z), 10)
	dst = append(dst, `,"heading":`...)
	dst = strconv.AppendInt(dst, int64(c.Heading), 10)
	dst = append(dst, `,"curHp":`...)
	dst = appendJSONFloat(dst, c.CurHP)
	dst = append(dst, `,"maxHp":`...)
	dst = appendJSONFloat(dst, c.MaxHP)
	dst = append(dst, `,"curMp":`...)
	dst = appendJSONFloat(dst, c.CurMP)
	dst = append(dst, `,"maxMp":`...)
	dst = appendJSONFloat(dst, c.MaxMP)
	dst = append(dst, `,"sitting":`...)
	dst = strconv.AppendBool(dst, c.Sitting)
	dst = append(dst, `,"str":`...)
	dst = strconv.AppendInt(dst, int64(c.STR), 10)
	dst = append(dst, `,"dex":`...)
	dst = strconv.AppendInt(dst, int64(c.DEX), 10)
	dst = append(dst, `,"con":`...)
	dst = strconv.AppendInt(dst, int64(c.CON), 10)
	dst = append(dst, `,"int":`...)
	dst = strconv.AppendInt(dst, int64(c.INT), 10)
	dst = append(dst, `,"wit":`...)
	dst = strconv.AppendInt(dst, int64(c.WIT), 10)
	dst = append(dst, `,"men":`...)
	dst = strconv.AppendInt(dst, int64(c.MEN), 10)
	dst = append(dst, `,"exp":`...)
	dst = strconv.AppendInt(dst, int64(c.Exp), 10)
	dst = append(dst, `,"expPercent":`...)
	dst = appendJSONFloat(dst, c.ExpPercent)
	dst = append(dst, `,"sp":`...)
	dst = strconv.AppendInt(dst, int64(c.Sp), 10)
	dst = append(dst, `,"inCombat":`...)
	dst = strconv.AppendBool(dst, c.InCombat)
	dst = append(dst, `,"load":`...)
	dst = strconv.AppendInt(dst, int64(c.CurrentLoad), 10)
	dst = append(dst, `,"maxLoad":`...)
	dst = strconv.AppendInt(dst, int64(c.MaxLoad), 10)
	dst = append(dst, `,"inventorySlots":`...)
	dst = strconv.AppendInt(dst, int64(c.InventorySlots), 10)
	dst = append(dst, `,"inventoryMax":`...)
	dst = strconv.AppendInt(dst, int64(c.InventoryMax), 10)
	dst = append(dst, `,"adena":`...)
	dst = strconv.AppendInt(dst, int64(c.Adena), 10)
	dst = append(dst, '}')

	return dst
}

// appendObjectsJSON writes the world object array (nil becomes null
// like the reflection encoder).
func appendObjectsJSON(dst []byte, objects []ObjectSnapshot) []byte {
	if objects == nil {
		return append(dst, `null`...)
	}
	dst = append(dst, '[')
	for i := range objects {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = appendObjectJSON(dst, objects[i])
	}
	dst = append(dst, ']')

	return dst
}

// appendObjectJSON writes one world object view. The linear field
// walk mirrors the struct declaration; splitting it only obscures
// the wire format.
//
//nolint:funlen // linear field order
func appendObjectJSON(dst []byte, o ObjectSnapshot) []byte {
	dst = append(dst, `{"objectId":`...)
	dst = strconv.AppendInt(dst, int64(o.ObjectID), 10)
	dst = append(dst, `,"kind":`...)
	dst = appendJSONString(dst, string(o.Kind))
	dst = append(dst, `,"name":`...)
	dst = appendJSONString(dst, o.Name)
	dst = append(dst, `,"title":`...)
	dst = appendJSONString(dst, o.Title)
	dst = append(dst, `,"templateId":`...)
	dst = strconv.AppendInt(dst, int64(o.TemplateID), 10)
	dst = append(dst, `,"attackable":`...)
	dst = strconv.AppendBool(dst, o.Attackable)
	dst = append(dst, `,"aggressive":`...)
	dst = strconv.AppendBool(dst, o.Aggressive)
	dst = append(dst, `,"aggroRange":`...)
	dst = strconv.AppendInt(dst, int64(o.AggroRange), 10)
	dst = append(dst, `,"level":`...)
	dst = strconv.AppendInt(dst, int64(o.Level), 10)
	dst = append(dst, `,"targetId":`...)
	dst = strconv.AppendInt(dst, int64(o.TargetID), 10)
	dst = append(dst, `,"inCombat":`...)
	dst = strconv.AppendBool(dst, o.InCombat)
	dst = append(dst, `,"dead":`...)
	dst = strconv.AppendBool(dst, o.Dead)
	dst = append(dst, `,"sitting":`...)
	dst = strconv.AppendBool(dst, o.Sitting)
	dst = append(dst, `,"moving":`...)
	dst = strconv.AppendBool(dst, o.Moving)
	dst = append(dst, `,"running":`...)
	dst = strconv.AppendBool(dst, o.Running)
	dst = append(dst, `,"speed":`...)
	dst = appendJSONFloat(dst, o.Speed)
	dst = append(dst, `,"collisionRadius":`...)
	dst = appendJSONFloat(dst, o.CollisionRadius)
	dst = append(dst, `,"socialUntilMs":`...)
	dst = strconv.AppendInt(dst, o.SocialUntilMs, 10)
	dst = append(dst, `,"count":`...)
	dst = strconv.AppendInt(dst, int64(o.Count), 10)
	dst = append(dst, `,"x":`...)
	dst = strconv.AppendInt(dst, int64(o.X), 10)
	dst = append(dst, `,"y":`...)
	dst = strconv.AppendInt(dst, int64(o.Y), 10)
	dst = append(dst, `,"z":`...)
	dst = strconv.AppendInt(dst, int64(o.Z), 10)
	dst = append(dst, `,"heading":`...)
	dst = strconv.AppendInt(dst, int64(o.Heading), 10)
	dst = append(dst, `,"destX":`...)
	dst = strconv.AppendInt(dst, int64(o.DestX), 10)
	dst = append(dst, `,"destY":`...)
	dst = strconv.AppendInt(dst, int64(o.DestY), 10)
	dst = append(dst, `,"destZ":`...)
	dst = strconv.AppendInt(dst, int64(o.DestZ), 10)
	dst = append(dst, `,"moveAtMs":`...)
	dst = strconv.AppendInt(dst, o.MoveAtMs, 10)
	dst = append(dst, `,"curHp":`...)
	dst = appendJSONFloat(dst, o.CurHP)
	dst = append(dst, `,"maxHp":`...)
	dst = appendJSONFloat(dst, o.MaxHP)
	dst = append(dst, `,"curMp":`...)
	dst = appendJSONFloat(dst, o.CurMP)
	dst = append(dst, `,"maxMp":`...)
	dst = appendJSONFloat(dst, o.MaxMP)
	dst = append(dst, '}')

	return dst
}

// appendInventoryJSON writes the equipment widget item array.
func appendInventoryJSON(dst []byte, items []InventoryItemSnapshot) []byte {
	if items == nil {
		return append(dst, `null`...)
	}
	dst = append(dst, '[')
	for i := range items {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = appendInventoryItemJSON(dst, items[i])
	}
	dst = append(dst, ']')

	return dst
}

// appendInventoryItemJSON writes one equipment widget item. The live
// state encoder reuses it with a stack allocated view, so the element
// encoding stays byte identical between the two paths.
//
//nolint:funlen // linear field order
func appendInventoryItemJSON(dst []byte, item InventoryItemSnapshot) []byte {
	dst = append(dst, `{"objectId":`...)
	dst = strconv.AppendInt(dst, int64(item.ObjectID), 10)
	dst = append(dst, `,"itemId":`...)
	dst = strconv.AppendInt(dst, int64(item.ItemID), 10)
	dst = append(dst, `,"count":`...)
	dst = strconv.AppendInt(dst, int64(item.Count), 10)
	dst = append(dst, `,"type2":`...)
	dst = strconv.AppendInt(dst, int64(item.Type2), 10)
	dst = append(dst, `,"equipped":`...)
	dst = strconv.AppendBool(dst, item.Equipped)
	dst = append(dst, `,"bodyPart":`...)
	dst = strconv.AppendInt(dst, int64(item.BodyPart), 10)
	dst = append(dst, `,"enchant":`...)
	dst = strconv.AppendInt(dst, int64(item.Enchant), 10)
	dst = append(dst, `,"name":`...)
	dst = appendJSONString(dst, item.Name)
	dst = append(dst, `,"icon":`...)
	dst = appendJSONString(dst, item.Icon)
	dst = append(dst, `,"type":`...)
	dst = appendJSONString(dst, item.Type)
	dst = append(dst, `,"weaponType":`...)
	dst = appendJSONString(dst, item.WeaponType)
	dst = append(dst, `,"armorType":`...)
	dst = appendJSONString(dst, item.ArmorType)
	dst = append(dst, `,"bodyPartKey":`...)
	dst = appendJSONString(dst, item.BodyPartKey)
	dst = append(dst, `,"pAtk":`...)
	dst = strconv.AppendInt(dst, int64(item.PAtk), 10)
	dst = append(dst, `,"mAtk":`...)
	dst = strconv.AppendInt(dst, int64(item.MAtk), 10)
	dst = append(dst, `,"pDef":`...)
	dst = strconv.AppendInt(dst, int64(item.PDef), 10)
	dst = append(dst, `,"mDef":`...)
	dst = strconv.AppendInt(dst, int64(item.MDef), 10)
	dst = append(dst, `,"sDef":`...)
	dst = strconv.AppendInt(dst, int64(item.SDef), 10)
	dst = append(dst, `,"rShld":`...)
	dst = strconv.AppendInt(dst, int64(item.RShld), 10)
	dst = append(dst, `,"pAtkSpd":`...)
	dst = strconv.AppendInt(dst, int64(item.PAtkSpd), 10)
	dst = append(dst, `,"soulShots":`...)
	dst = strconv.AppendInt(dst, int64(item.SoulShots), 10)
	dst = append(dst, `,"spiritShots":`...)
	dst = strconv.AppendInt(dst, int64(item.SpiritShots), 10)
	dst = append(dst, `,"weight":`...)
	dst = strconv.AppendInt(dst, int64(item.Weight), 10)
	dst = append(dst, `,"price":`...)
	dst = strconv.AppendInt(dst, item.Price, 10)

	return append(dst, '}')
}

// appendEventsJSON writes the rolling event log array.
func appendEventsJSON(dst []byte, events []Event) []byte {
	if events == nil {
		return append(dst, `null`...)
	}
	dst = append(dst, '[')
	for i := range events {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = appendEventJSON(dst, events[i])
	}
	dst = append(dst, ']')

	return dst
}

// appendEventJSON writes one rolling event log entry. The live state
// encoder reuses it with the ring record directly.
func appendEventJSON(dst []byte, event Event) []byte {
	dst = append(dst, `{"time":`...)
	dst = appendJSONTime(dst, event.Time)
	dst = append(dst, `,"message":`...)
	dst = appendJSONString(dst, event.Message)

	return append(dst, '}')
}

// appendChatJSON writes the chat window array.
func appendChatJSON(dst []byte, chat []ChatEvent) []byte {
	if chat == nil {
		return append(dst, `null`...)
	}
	dst = append(dst, '[')
	for i := range chat {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = appendChatEventJSON(dst, chat[i])
	}
	dst = append(dst, ']')

	return dst
}

// appendChatEventJSON writes one chat window line. The live state
// encoder reuses it with the ring record directly.
func appendChatEventJSON(dst []byte, line ChatEvent) []byte {
	dst = append(dst, `{"time":`...)
	dst = appendJSONTime(dst, line.Time)
	dst = append(dst, `,"kind":`...)
	dst = appendJSONString(dst, line.Kind)
	dst = append(dst, `,"text":`...)
	dst = appendJSONString(dst, line.Text)

	return append(dst, '}')
}

// appendWalkPathJSON writes the walk plan array.
func appendWalkPathJSON(dst []byte, points []WalkPoint) []byte {
	if points == nil {
		return append(dst, `null`...)
	}
	dst = append(dst, '[')
	for i := range points {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = appendWalkPointJSON(dst, points[i])
	}
	dst = append(dst, ']')

	return dst
}

// appendWalkPointJSON writes one walk plan waypoint. The live state
// encoder reuses it with the published plan directly.
func appendWalkPointJSON(dst []byte, point WalkPoint) []byte {
	dst = append(dst, `{"x":`...)
	dst = strconv.AppendInt(dst, int64(point.X), 10)
	dst = append(dst, `,"y":`...)
	dst = strconv.AppendInt(dst, int64(point.Y), 10)
	dst = append(dst, `,"z":`...)
	dst = strconv.AppendInt(dst, int64(point.Z), 10)

	return append(dst, '}')
}

// appendShoppingPlanJSON writes the published shopping plan object
// (null when none is published). The live state encoder reuses it
// with the published plan under the read lock; the entries array of
// a live plan is never nil (SetShoppingPlan clears empty plans).
func appendShoppingPlanJSON(dst []byte, plan *ShoppingPlanView) []byte {
	if plan == nil {
		return append(dst, `null`...)
	}
	if plan.Entries == nil {
		dst = append(dst, `{"entries":null`...)
	} else {
		dst = append(dst, `{"entries":[`...)
		for i := range plan.Entries {
			if i > 0 {
				dst = append(dst, ',')
			}
			dst = appendShoppingEntryJSON(dst, plan.Entries[i])
		}
		dst = append(dst, ']')
	}
	dst = append(dst, `,"adena":`...)
	dst = strconv.AppendInt(dst, plan.Adena, 10)
	dst = append(dst, `,"total":`...)
	dst = strconv.AppendInt(dst, plan.Total, 10)
	dst = append(dst, `,"trip":`...)
	dst = strconv.AppendBool(dst, plan.Trip)
	dst = append(dst, '}')

	return dst
}

// appendShoppingEntryJSON writes one planned purchase of the shopping
// plan. The field order mirrors the struct declaration like the
// reflection encoder.
//
//nolint:funlen // linear field order
func appendShoppingEntryJSON(dst []byte, entry ShoppingEntryView) []byte {
	dst = append(dst, `{"itemId":`...)
	dst = strconv.AppendInt(dst, int64(entry.ItemID), 10)
	dst = append(dst, `,"name":`...)
	dst = appendJSONString(dst, entry.Name)
	dst = append(dst, `,"icon":`...)
	dst = appendJSONString(dst, entry.Icon)
	dst = append(dst, `,"merchantId":`...)
	dst = strconv.AppendInt(dst, int64(entry.MerchantID), 10)
	dst = append(dst, `,"merchant":`...)
	dst = appendJSONString(dst, entry.Merchant)
	dst = append(dst, `,"type":`...)
	dst = appendJSONString(dst, entry.Type)
	dst = append(dst, `,"weaponType":`...)
	dst = appendJSONString(dst, entry.WeaponType)
	dst = append(dst, `,"armorType":`...)
	dst = appendJSONString(dst, entry.ArmorType)
	dst = append(dst, `,"bodyPartKey":`...)
	dst = appendJSONString(dst, entry.BodyPartKey)
	dst = append(dst, `,"pAtk":`...)
	dst = strconv.AppendInt(dst, int64(entry.PAtk), 10)
	dst = append(dst, `,"mAtk":`...)
	dst = strconv.AppendInt(dst, int64(entry.MAtk), 10)
	dst = append(dst, `,"pDef":`...)
	dst = strconv.AppendInt(dst, int64(entry.PDef), 10)
	dst = append(dst, `,"mDef":`...)
	dst = strconv.AppendInt(dst, int64(entry.MDef), 10)
	dst = append(dst, `,"sDef":`...)
	dst = strconv.AppendInt(dst, int64(entry.SDef), 10)
	dst = append(dst, `,"rShld":`...)
	dst = strconv.AppendInt(dst, int64(entry.RShld), 10)
	dst = append(dst, `,"pAtkSpd":`...)
	dst = strconv.AppendInt(dst, int64(entry.PAtkSpd), 10)
	dst = append(dst, `,"soulShots":`...)
	dst = strconv.AppendInt(dst, int64(entry.SoulShots), 10)
	dst = append(dst, `,"spiritShots":`...)
	dst = strconv.AppendInt(dst, int64(entry.SpiritShots), 10)
	dst = append(dst, `,"weight":`...)
	dst = strconv.AppendInt(dst, int64(entry.Weight), 10)
	dst = append(dst, `,"price":`...)
	dst = strconv.AppendInt(dst, entry.Price, 10)
	dst = append(dst, `,"sellCredit":`...)
	dst = strconv.AppendInt(dst, entry.SellCredit, 10)
	dst = append(dst, `,"missing":`...)
	dst = strconv.AppendInt(dst, entry.Missing, 10)
	dst = append(dst, `,"gain":`...)
	dst = appendJSONFloat(dst, entry.Gain)
	dst = append(dst, `,"affordable":`...)
	dst = strconv.AppendBool(dst, entry.Affordable)
	dst = append(dst, `,"buying":`...)
	dst = strconv.AppendBool(dst, entry.Buying)
	dst = append(dst, `,"reason":`...)
	dst = appendJSONString(dst, entry.Reason)
	dst = append(dst, '}')

	return dst
}

// appendSkillsJSON writes the learned skill array (null when the
// character knows no skills yet).
func appendSkillsJSON(dst []byte, skills []SkillSnapshot) []byte {
	if skills == nil {
		return append(dst, `null`...)
	}
	dst = append(dst, '[')
	for i := range skills {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = appendSkillSnapshotJSON(dst, skills[i])
	}

	return append(dst, ']')
}

// appendSkillSnapshotJSON writes one learned skill. The field order
// mirrors the struct declaration like the reflection encoder.
func appendSkillSnapshotJSON(dst []byte, skill SkillSnapshot) []byte {
	dst = append(dst, `{"skillId":`...)
	dst = strconv.AppendInt(dst, int64(skill.SkillID), 10)
	dst = append(dst, `,"level":`...)
	dst = strconv.AppendInt(dst, int64(skill.Level), 10)
	dst = append(dst, `,"passive":`...)
	dst = strconv.AppendBool(dst, skill.Passive)
	dst = append(dst, `,"name":`...)
	dst = appendJSONString(dst, skill.Name)
	dst = append(dst, `,"icon":`...)
	dst = appendJSONString(dst, skill.Icon)
	dst = append(dst, `,"desc":`...)
	dst = appendJSONString(dst, skill.Desc)
	dst = append(dst, '}')

	return dst
}

// appendSkillPlanJSON writes the learning queue (null when the class
// is unknown or nothing is left to learn).
func appendSkillPlanJSON(dst []byte, plan *SkillPlanView) []byte {
	if plan == nil {
		return append(dst, `null`...)
	}
	dst = append(dst, `{"sp":`...)
	dst = strconv.AppendInt(dst, plan.Sp, 10)
	dst = append(dst, `,"total":`...)
	dst = strconv.AppendInt(dst, plan.Total, 10)
	dst = append(dst, `,"missing":`...)
	dst = strconv.AppendInt(dst, plan.Missing, 10)
	if plan.Entries == nil {
		dst = append(dst, `,"entries":null`...)
	} else {
		dst = append(dst, `,"entries":[`...)
		for i := range plan.Entries {
			if i > 0 {
				dst = append(dst, ',')
			}
			dst = appendSkillPlanEntryJSON(dst, plan.Entries[i])
		}
		dst = append(dst, ']')
	}
	dst = append(dst, '}')

	return dst
}

// appendSkillPlanEntryJSON writes one queued lesson. The field order
// mirrors the struct declaration like the reflection encoder.
func appendSkillPlanEntryJSON(dst []byte, entry SkillPlanEntry) []byte {
	dst = append(dst, `{"skillId":`...)
	dst = strconv.AppendInt(dst, int64(entry.SkillID), 10)
	dst = append(dst, `,"name":`...)
	dst = appendJSONString(dst, entry.Name)
	dst = append(dst, `,"icon":`...)
	dst = appendJSONString(dst, entry.Icon)
	dst = append(dst, `,"desc":`...)
	dst = appendJSONString(dst, entry.Desc)
	dst = append(dst, `,"level":`...)
	dst = strconv.AppendInt(dst, int64(entry.Level), 10)
	dst = append(dst, `,"passive":`...)
	dst = strconv.AppendBool(dst, entry.Passive)
	dst = append(dst, `,"spCost":`...)
	dst = strconv.AppendInt(dst, int64(entry.SpCost), 10)
	dst = append(dst, `,"reqLevel":`...)
	dst = strconv.AppendInt(dst, int64(entry.ReqLevel), 10)
	dst = append(dst, `,"category":`...)
	dst = strconv.AppendInt(dst, int64(entry.Category), 10)
	dst = append(dst, `,"affordable":`...)
	dst = strconv.AppendBool(dst, entry.Affordable)
	dst = append(dst, '}')

	return dst
}

// appendCombatEventsJSON writes the combat animation feed array.
func appendCombatEventsJSON(dst []byte, events []CombatEventView) []byte {
	if events == nil {
		return append(dst, `null`...)
	}
	dst = append(dst, '[')
	for i := range events {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = appendCombatEventViewJSON(dst, events[i])
	}
	dst = append(dst, ']')

	return dst
}

// appendCombatEventViewJSON writes one combat animation beat. The live
// state encoder reuses it with a stack allocated view of the feed
// record.
func appendCombatEventViewJSON(dst []byte, view CombatEventView) []byte {
	dst = append(dst, `{"seq":`...)
	dst = strconv.AppendUint(dst, view.Seq, 10)
	dst = append(dst, `,"kind":`...)
	dst = appendJSONString(dst, view.Kind)
	dst = append(dst, `,"attackerId":`...)
	dst = strconv.AppendInt(dst, int64(view.AttackerID), 10)
	dst = append(dst, `,"targetId":`...)
	dst = strconv.AppendInt(dst, int64(view.TargetID), 10)
	dst = append(dst, `,"amount":`...)
	dst = appendJSONFloat(dst, view.Amount)
	dst = append(dst, `,"atMs":`...)
	dst = strconv.AppendInt(dst, view.AtMs, 10)
	dst = append(dst, `,"x":`...)
	dst = strconv.AppendInt(dst, int64(view.X), 10)
	dst = append(dst, `,"y":`...)
	dst = strconv.AppendInt(dst, int64(view.Y), 10)
	dst = append(dst, `,"targetX":`...)
	dst = strconv.AppendInt(dst, int64(view.TargetX), 10)
	dst = append(dst, `,"targetY":`...)
	dst = strconv.AppendInt(dst, int64(view.TargetY), 10)

	return append(dst, '}')
}

// appendZoneJSON writes the hunting zone square (nil becomes null).
func appendZoneJSON(dst []byte, zone *Zone) []byte {
	if zone == nil {
		return append(dst, `null`...)
	}
	dst = append(dst, `{"cx":`...)
	dst = strconv.AppendInt(dst, int64(zone.CX), 10)
	dst = append(dst, `,"cy":`...)
	dst = strconv.AppendInt(dst, int64(zone.CY), 10)
	dst = append(dst, `,"half":`...)
	dst = strconv.AppendInt(dst, int64(zone.Half), 10)
	dst = append(dst, '}')

	return dst
}

// appendZoneViewsJSON writes the zone registry array of the map view.
func appendZoneViewsJSON(dst []byte, zones []ZoneView) []byte {
	if zones == nil {
		return append(dst, `null`...)
	}
	dst = append(dst, '[')
	for i := range zones {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = appendZoneViewJSON(dst, zones[i])
	}
	dst = append(dst, ']')

	return dst
}

// appendZoneViewJSON writes one zone registry entry of the map view.
// The live state encoder reuses it with the stored view directly.
func appendZoneViewJSON(dst []byte, zone ZoneView) []byte {
	dst = append(dst, `{"id":`...)
	dst = appendJSONString(dst, zone.ID)
	dst = append(dst, `,"name":`...)
	dst = appendJSONString(dst, zone.Name)
	dst = append(dst, `,"region":`...)
	dst = appendJSONString(dst, zone.Region)
	dst = append(dst, `,"minLevel":`...)
	dst = strconv.AppendInt(dst, int64(zone.MinLevel), 10)
	dst = append(dst, `,"maxLevel":`...)
	dst = strconv.AppendInt(dst, int64(zone.MaxLevel), 10)
	dst = append(dst, `,"minGear":`...)
	dst = strconv.AppendInt(dst, int64(zone.MinGear), 10)
	dst = append(dst, `,"cx":`...)
	dst = strconv.AppendInt(dst, int64(zone.CX), 10)
	dst = append(dst, `,"cy":`...)
	dst = strconv.AppendInt(dst, int64(zone.CY), 10)
	dst = append(dst, `,"half":`...)
	dst = strconv.AppendInt(dst, int64(zone.Half), 10)
	dst = append(dst, `,"active":`...)
	dst = strconv.AppendBool(dst, zone.Active)
	dst = append(dst, `,"deaths":`...)
	dst = strconv.AppendInt(dst, int64(zone.Deaths), 10)
	dst = append(dst, `,"demoted":`...)
	dst = strconv.AppendBool(dst, zone.Demoted)
	dst = append(dst, `,"kind":`...)
	dst = appendJSONString(dst, zone.Kind)
	dst = append(dst, `,"radius":`...)
	dst = strconv.AppendInt(dst, int64(zone.Radius), 10)
	dst = append(dst, `,"respawnMinSec":`...)
	dst = strconv.AppendInt(dst, int64(zone.RespawnMinSec), 10)
	dst = append(dst, `,"respawnMaxSec":`...)
	dst = strconv.AppendInt(dst, int64(zone.RespawnMaxSec), 10)
	dst = append(dst, `,"spawnMass":`...)
	dst = strconv.AppendInt(dst, int64(zone.SpawnMass), 10)
	dst = append(dst, `,"aggroMass":`...)
	dst = strconv.AppendInt(dst, int64(zone.AggroMass), 10)
	dst = append(dst, `,"adenaPerMin":`...)
	dst = appendJSONFloat(dst, zone.AdenaPerMin)
	dst = append(dst, `,"deathHeat":`...)
	dst = appendJSONFloat(dst, zone.DeathHeat)
	dst = append(dst, `,"nextRespawnSec":`...)
	dst = strconv.AppendInt(dst, int64(zone.NextRespawnSec), 10)
	dst = append(dst, `,"occupancy":`...)
	dst = strconv.AppendInt(dst, int64(zone.Occupancy), 10)
	dst = append(dst, `,"killX":`...)
	dst = strconv.AppendInt(dst, int64(zone.KillX), 10)
	dst = append(dst, `,"killY":`...)
	dst = strconv.AppendInt(dst, int64(zone.KillY), 10)

	return append(dst, '}')
}
