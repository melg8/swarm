// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

// SPDX-License-Identifier: MIT

package connection

// The world packet apply paths of the game session, split out of
// the game.go god file: one apply function per packet family, each
// parsing into the reusable scratch struct of the client (zero
// allocation parsing, see GameClient) and mirroring the field walk
// into the state tracker.

import (
	fromgameserver "github.com/melg8/swarm/internal/swarm/packets/from_game_server"
	togameserver "github.com/melg8/swarm/internal/swarm/packets/to_game_server"
	"github.com/melg8/swarm/internal/swarm/state"
)

// applyUserInfo parses UserInfo and updates the character state.
func (gc *GameClient) applyUserInfo(payload []byte) {
	err := fromgameserver.ParseUserInfoPacket(&gc.userInfo, payload)
	if err != nil {
		gc.logger.Printf("Failed to parse user info: %v", err)

		return
	}
	if gc.tracker != nil {
		info := gc.userInfo
		gc.tracker.ApplyUserInfo(state.UserInfo{
			Name:               info.Name,
			Level:              info.Level,
			Race:               info.Race,
			ClassID:            info.ClassID,
			X:                  info.X,
			Y:                  info.Y,
			Z:                  info.Z,
			STR:                info.STR,
			DEX:                info.DEX,
			CON:                info.CON,
			INT:                info.INT,
			WIT:                info.WIT,
			MEN:                info.MEN,
			Exp:                info.Exp,
			Sp:                 info.Sp,
			MaxHP:              info.MaxHP,
			CurHP:              info.CurHP,
			MaxMP:              info.MaxMP,
			CurMP:              info.CurMP,
			CurrentLoad:        info.CurrentLoad,
			MaxLoad:            info.MaxLoad,
			RunSpeed:           info.RunSpeed,
			WalkSpeed:          info.WalkSpeed,
			MoveSpeedMult:      info.MoveSpeedMult,
			PaperdollObjectIDs: info.PaperdollObjectIDs,
		})
	}
	gc.logger.Printf("User info: %s level %d hp %d/%d mp %d/%d speed %d x %d",
		gc.userInfo.Name, gc.userInfo.Level,
		gc.userInfo.CurHP, gc.userInfo.MaxHP,
		gc.userInfo.CurMP, gc.userInfo.MaxMP, gc.userInfo.RunSpeed,
		gc.userInfo.CurrentLoad)
}

// applyNpcInfo parses NpcInfo and upserts the npc object.
func (gc *GameClient) applyNpcInfo(payload []byte) {
	if err := fromgameserver.ParseNpcInfoPacket(&gc.npcInfo, payload); err != nil {
		gc.logger.Printf("Failed to parse npc info: %v", err)

		return
	}
	if gc.tracker != nil {
		info := gc.npcInfo
		gc.tracker.ApplyNpcInfo(state.NpcInfo{
			ObjectID:        info.ObjectID,
			TemplateID:      info.TemplateID,
			Attackable:      info.Attackable,
			X:               info.X,
			Y:               info.Y,
			Z:               info.Z,
			Heading:         info.Heading,
			RunSpeed:        info.RunSpeed,
			WalkSpeed:       info.WalkSpeed,
			MoveSpeedMult:   info.MoveSpeedMult,
			CollisionRadius: info.CollisionRadius,
			Running:         info.Running,
			InCombat:        info.InCombat,
			Dead:            info.Dead,
			Name:            info.Name,
			Title:           info.Title,
		})
	}
	gc.logger.Printf("NPC %s spawned at %d %d %d heading %d",
		gc.npcInfo.Name, gc.npcInfo.X, gc.npcInfo.Y, gc.npcInfo.Z,
		gc.npcInfo.Heading)
}

// applyCharInfo parses CharInfo and upserts the player object.
func (gc *GameClient) applyCharInfo(payload []byte) {
	err := fromgameserver.ParseCharInfoPacket(&gc.charInfo, payload)
	if err != nil {
		gc.logger.Printf("Failed to parse char info: %v", err)

		return
	}
	if gc.tracker != nil {
		info := gc.charInfo
		gc.tracker.ApplyPlayerInfo(state.PlayerInfo{
			ObjectID:        info.ObjectID,
			Name:            info.Name,
			Title:           info.Title,
			Race:            info.Race,
			ClassID:         info.ClassID,
			RunSpeed:        info.RunSpeed,
			WalkSpeed:       info.WalkSpeed,
			MoveSpeedMult:   info.MoveSpeedMult,
			CollisionRadius: info.CollisionRadius,
			Running:         info.Running,
			InCombat:        info.InCombat,
			Dead:            info.Dead,
			Sitting:         !info.Standing,
			X:               info.X,
			Y:               info.Y,
			Z:               info.Z,
		})
	}
	gc.logger.Printf("Player %s appeared at %d %d %d speed %d",
		gc.charInfo.Name, gc.charInfo.X, gc.charInfo.Y, gc.charInfo.Z,
		gc.charInfo.RunSpeed)
}

// applyDropItem parses DropItem and upserts the ground item object.
func (gc *GameClient) applyDropItem(payload []byte) {
	err := fromgameserver.ParseDropItemPacket(&gc.dropItem, payload)
	if err != nil {
		gc.logger.Printf("Failed to parse drop item: %v", err)

		return
	}
	if gc.tracker != nil {
		info := gc.dropItem
		gc.tracker.ApplyItemInfo(state.ItemInfo{
			ObjectID:   info.ObjectID,
			TemplateID: info.TemplateID,
			Stackable:  info.Stackable,
			Count:      info.Count,
			X:          info.X,
			Y:          info.Y,
			Z:          info.Z,
		})
	}
}

// applyDeleteObject parses DeleteObject and removes the object.
func (gc *GameClient) applyDeleteObject(payload []byte) {
	err := fromgameserver.ParseDeleteObjectPacket(&gc.deleted, payload)
	if err != nil {
		gc.logger.Printf("Failed to parse delete object: %v", err)

		return
	}
	if gc.tracker != nil {
		gc.tracker.RemoveObject(gc.deleted.ObjectID)
	}
}

// applyMoveToLocation parses MoveToLocation and updates the movement.
func (gc *GameClient) applyMoveToLocation(payload []byte) {
	err := fromgameserver.ParseMoveToLocationPacket(&gc.moveTo, payload)
	if err != nil {
		gc.logger.Printf("Failed to parse movement: %v", err)

		return
	}
	if gc.tracker != nil {
		move := gc.moveTo
		gc.tracker.ApplyMovement(state.Movement{
			ObjectID: move.ObjectID,
			X:        move.X,
			Y:        move.Y,
			Z:        move.Z,
			DestX:    move.DestX,
			DestY:    move.DestY,
			DestZ:    move.DestZ,
		})
	}
}

// applyStopMove parses StopMove and updates the object placement.
func (gc *GameClient) applyStopMove(payload []byte) {
	err := fromgameserver.ParseStopMovePacket(&gc.stopMove, payload)
	if err != nil {
		gc.logger.Printf("Failed to parse stop move: %v", err)

		return
	}
	if gc.tracker != nil {
		stop := gc.stopMove
		gc.tracker.ApplyPlacement(state.Placement{
			ObjectID: stop.ObjectID,
			X:        stop.X,
			Y:        stop.Y,
			Z:        stop.Z,
			Heading:  stop.Heading,
			Moving:   false,
		})
	}
}

// applyValidateLocation parses ValidateLocation and updates the placement.
func (gc *GameClient) applyValidateLocation(payload []byte) {
	if err := fromgameserver.ParseValidateLocationPacket(
		&gc.validateLoc, payload); err != nil {
		gc.logger.Printf("Failed to parse validate location: %v", err)

		return
	}
	if gc.tracker != nil {
		place := gc.validateLoc
		gc.tracker.ApplyPlacement(state.Placement{
			ObjectID: place.ObjectID,
			X:        place.X,
			Y:        place.Y,
			Z:        place.Z,
			Heading:  place.Heading,
			Moving:   false,
		})
	}
}

// applyStatusUpdate parses StatusUpdate and applies the vitals changes.
func (gc *GameClient) applyStatusUpdate(payload []byte) {
	if err := fromgameserver.ParseStatusUpdatePacket(
		&gc.statusUpd, payload); err != nil {
		gc.logger.Printf("Failed to parse status update: %v", err)

		return
	}
	if gc.tracker != nil {
		attrs := gc.statusAttrs[:0]
		gc.statusUpd.ForEach(func(id int32, value int32) {
			attrs = append(attrs, state.Attribute{ID: id, Value: value})
		})
		gc.tracker.ApplyStatusUpdate(gc.statusUpd.ObjectID, attrs)
	}
}

// applyAttack parses Attack and updates the combat state.
func (gc *GameClient) applyAttack(payload []byte) {
	if err := fromgameserver.ParseAttackPacket(&gc.attack, payload); err != nil {
		gc.logger.Printf("Failed to parse attack: %v", err)

		return
	}
	if gc.tracker != nil {
		// The tracker list is capped like the packet struct: the count
		// clamps to the capacity so the extra hits of a wide multi
		// attack never index past the array.
		count := min(gc.attack.HitCount, state.AttackTargets)
		attack := state.Attack{
			AttackerID:  gc.attack.AttackerID,
			X:           gc.attack.X,
			Y:           gc.attack.Y,
			Z:           gc.attack.Z,
			TargetX:     gc.attack.TargetX,
			TargetY:     gc.attack.TargetY,
			TargetZ:     gc.attack.TargetZ,
			TargetIDs:   [4]int32{},
			HitFlags:    [state.AttackTargets]int8{},
			TargetCount: count,
		}
		for i := range count {
			attack.TargetIDs[i] = gc.attack.Hits[i].TargetID
			attack.HitFlags[i] = gc.attack.Hits[i].Flags
		}
		gc.tracker.ApplyAttack(attack)
	}
}

// applyAutoAttackStart parses AutoAttackStart and marks combat.
func (gc *GameClient) applyAutoAttackStart(payload []byte) {
	if err := fromgameserver.ParseAutoAttackStartPacket(
		&gc.attackStart, payload); err != nil {
		gc.logger.Printf("Failed to parse auto attack start: %v", err)

		return
	}
	if gc.tracker != nil {
		gc.tracker.ApplyAutoAttackStart(gc.attackStart.ObjectID)
	}
}

// applyAutoAttackStop parses AutoAttackStop and clears combat.
func (gc *GameClient) applyAutoAttackStop(payload []byte) {
	if err := fromgameserver.ParseAutoAttackStopPacket(
		&gc.attackStop, payload); err != nil {
		gc.logger.Printf("Failed to parse auto attack stop: %v", err)

		return
	}
	if gc.tracker != nil {
		gc.tracker.ApplyAutoAttackStop(gc.attackStop.ObjectID)
	}
}

// applyMoveToPawn parses MoveToPawn and updates the chasing object.
func (gc *GameClient) applyMoveToPawn(payload []byte) {
	if err := fromgameserver.ParseMoveToPawnPacket(
		&gc.moveToPawn, payload); err != nil {
		gc.logger.Printf("Failed to parse move to pawn: %v", err)

		return
	}
	if gc.tracker != nil {
		move := gc.moveToPawn
		gc.tracker.ApplyPawnMovement(state.PawnMovement{
			ObjectID: move.ObjectID,
			TargetID: move.TargetID,
			Distance: move.Distance,
			X:        move.X,
			Y:        move.Y,
			Z:        move.Z,
			TargetX:  move.TargetX,
			TargetY:  move.TargetY,
			TargetZ:  move.TargetZ,
		})
	}
}

// applySpawnItem parses SpawnItem and upserts a ground item that already
// existed around the character.
func (gc *GameClient) applySpawnItem(payload []byte) {
	if err := fromgameserver.ParseSpawnItemPacket(
		&gc.spawnItem, payload); err != nil {
		gc.logger.Printf("Failed to parse spawn item: %v", err)

		return
	}
	if gc.tracker != nil {
		info := gc.spawnItem
		gc.tracker.ApplySpawnItem(state.ItemInfo{
			ObjectID:   info.ObjectID,
			TemplateID: info.TemplateID,
			Stackable:  info.Stackable,
			Count:      info.Count,
			X:          info.X,
			Y:          info.Y,
			Z:          info.Z,
		})
	}
}

// applyGetItem parses GetItem and removes the picked up ground item.
func (gc *GameClient) applyGetItem(payload []byte) {
	if err := fromgameserver.ParseGetItemPacket(&gc.getItem, payload); err != nil {
		gc.logger.Printf("Failed to parse get item: %v", err)

		return
	}
	if gc.tracker != nil {
		item := gc.getItem
		gc.tracker.ApplyItemPickup(state.ItemPickup{
			PlayerID: item.PlayerID,
			ObjectID: item.ObjectID,
			X:        item.X,
			Y:        item.Y,
			Z:        item.Z,
		})
	}
}

// applyBeginRotation parses BeginRotation and turns the object.
func (gc *GameClient) applyBeginRotation(payload []byte) {
	if err := fromgameserver.ParseBeginRotationPacket(
		&gc.beginRotation, payload); err != nil {
		gc.logger.Printf("Failed to parse begin rotation: %v", err)

		return
	}
	if gc.tracker != nil {
		gc.tracker.ApplyRotationStart(state.Rotation{
			ObjectID: gc.beginRotation.ObjectID,
			Heading:  gc.beginRotation.Heading,
		})
	}
}

// applyStopRotation parses StopRotation and turns the object.
func (gc *GameClient) applyStopRotation(payload []byte) {
	if err := fromgameserver.ParseStopRotationPacket(
		&gc.stopRotation, payload); err != nil {
		gc.logger.Printf("Failed to parse stop rotation: %v", err)

		return
	}
	if gc.tracker != nil {
		gc.tracker.ApplyRotationStop(state.Rotation{
			ObjectID: gc.stopRotation.ObjectID,
			Heading:  gc.stopRotation.Heading,
		})
	}
}

// applyChangeMoveType parses ChangeMoveType and updates the run state.
func (gc *GameClient) applyChangeMoveType(payload []byte) {
	if err := fromgameserver.ParseChangeMoveTypePacket(
		&gc.changeMoveType, payload); err != nil {
		gc.logger.Printf("Failed to parse change move type: %v", err)

		return
	}
	if gc.tracker != nil {
		gc.tracker.ApplyMoveType(state.MoveType{
			ObjectID: gc.changeMoveType.ObjectID,
			Running:  gc.changeMoveType.Running,
		})
	}
}

// applyChangeWaitType parses ChangeWaitType and tracks the sit state.
func (gc *GameClient) applyChangeWaitType(payload []byte) {
	if err := fromgameserver.ParseChangeWaitTypePacket(
		&gc.changeWait, payload); err != nil {
		gc.logger.Printf("Failed to parse change wait type: %v", err)

		return
	}
	if gc.tracker != nil {
		gc.tracker.ApplyWaitType(state.WaitType{
			ObjectID: gc.changeWait.ObjectID,
			Sitting:  gc.changeWait.Sitting,
		})
	}
}

// applyTeleport parses TeleportToLocation and snaps the object.
func (gc *GameClient) applyTeleport(payload []byte) {
	if err := fromgameserver.ParseTeleportToLocationPacket(
		&gc.teleport, payload); err != nil {
		gc.logger.Printf("Failed to parse teleport: %v", err)

		return
	}
	if gc.tracker != nil {
		tele := gc.teleport
		gc.tracker.ApplyTeleport(state.Teleport{
			ObjectID: tele.ObjectID,
			X:        tele.X,
			Y:        tele.Y,
			Z:        tele.Z,
			Heading:  tele.Heading,
		})
		// The server keeps the character teleporting until the client
		// confirms the finished teleport with Appearing: without it the
		// character AI silently ignores every move request (the village
		// revive left the bot stuck).
		if gc.tracker.SelfObjectID() == tele.ObjectID {
			if err := gc.sendPacket(togameserver.NewAppearingPacket()); err != nil {
				gc.logger.Printf("Failed to send appearing: %v", err)
			}
		}
	}
}

// applyMyTargetSelected parses MyTargetSelected and records the own
// target of the bot.
func (gc *GameClient) applyMyTargetSelected(payload []byte) {
	if err := fromgameserver.ParseMyTargetSelectedPacket(
		&gc.myTarget, payload); err != nil {
		gc.logger.Printf("Failed to parse my target selected: %v", err)

		return
	}
	if gc.tracker != nil {
		gc.tracker.ApplySelfTarget(gc.myTarget.ObjectID)
	}
}

// applyTargetSelected parses TargetSelected and records the target of
// another player.
func (gc *GameClient) applyTargetSelected(payload []byte) {
	if err := fromgameserver.ParseTargetSelectedPacket(
		&gc.targetSelected, payload); err != nil {
		gc.logger.Printf("Failed to parse target selected: %v", err)

		return
	}
	if gc.tracker != nil {
		selected := gc.targetSelected
		gc.tracker.ApplyObjectTarget(selected.ObjectID, selected.TargetID)
	}
}

// applyTargetUnselected parses TargetUnselected and clears the target
// reference of another player.
func (gc *GameClient) applyTargetUnselected(payload []byte) {
	if err := fromgameserver.ParseTargetUnselectedPacket(
		&gc.targetDropped, payload); err != nil {
		gc.logger.Printf("Failed to parse target unselected: %v", err)

		return
	}
	if gc.tracker != nil {
		gc.tracker.ApplyTargetClear(gc.targetDropped.ObjectID)
	}
}

// applyItemList parses ItemList and replaces the tracked inventory.
func (gc *GameClient) applyItemList(payload []byte) {
	err := fromgameserver.ParseItemListPacket(
		&gc.itemList, payload)
	if err != nil {
		gc.logger.Printf("Failed to parse item list: %v", err)

		return
	}
	if gc.tracker == nil {
		return
	}
	items := gc.convertInventoryItems(gc.itemList.Items)
	gc.tracker.ApplyItemList(items)
	gc.logger.Printf("Inventory listed with %d items", len(items))
}

// applySkillList parses SkillList and replaces the learned skill set
// of the tracker (the web UI renders it and computes the learning
// queue from it). The server sends the whole list, so a merge is
// never needed.
func (gc *GameClient) applySkillList(payload []byte) {
	if err := fromgameserver.ParseSkillListPacket(
		&gc.skillList, payload); err != nil {
		gc.logger.Printf("Failed to parse skill list: %v", err)

		return
	}
	if gc.tracker == nil {
		return
	}
	skills := gc.convertSkills(gc.skillList.Skills)
	gc.tracker.SetSkills(skills)
	gc.logger.Printf("Skill list with %d skills", len(skills))
}

// convertSkills copies the parsed entries into state entries through
// a fresh slice: the state layer stores its own map, but the parse
// buffer is reused by the next packet, so the values must not alias
// it.
func (gc *GameClient) convertSkills(
	source []fromgameserver.SkillListEntry,
) []state.LearnedSkill {
	skills := make([]state.LearnedSkill, 0, len(source))
	for _, skill := range source {
		skills = append(skills, state.LearnedSkill{
			SkillID: skill.SkillID,
			Level:   skill.Level,
			Passive: skill.Passive,
		})
	}

	return skills
}

// applyAbnormalStatusUpdate parses AbnormalStatusUpdate and replaces
// the active effect list of the tracker: the buff casting of the hunt
// loop skips the buffs that already run, the web UI buffs widget
// renders the list with the remaining durations. The server sends
// the whole list whenever an effect changes, so a merge is never
// needed.
func (gc *GameClient) applyAbnormalStatusUpdate(payload []byte) {
	if err := fromgameserver.ParseAbnormalStatusUpdatePacket(
		&gc.abnormalStatus, payload); err != nil {
		gc.logger.Printf("Failed to parse abnormal status: %v", err)

		return
	}
	if gc.tracker == nil {
		return
	}
	buffs := make([]state.BuffEntry, 0, len(gc.abnormalStatus.Buffs))
	for _, buff := range gc.abnormalStatus.Buffs {
		buffs = append(buffs, state.BuffEntry{
			SkillID: buff.SkillID,
			Level:   buff.Level,
			Time:    buff.Time,
		})
	}
	gc.tracker.SetBuffs(buffs)
	if gc.trace {
		gc.logger.Printf("Abnormal status with %d buffs", len(buffs))
	}
}

// applyInventoryUpdate parses InventoryUpdate and applies the changes.
func (gc *GameClient) applyInventoryUpdate(payload []byte) {
	if err := fromgameserver.ParseInventoryUpdatePacket(
		&gc.invUpdate, payload); err != nil {
		gc.logger.Printf("Failed to parse inventory update: %v", err)

		return
	}
	if gc.tracker == nil {
		return
	}
	items := gc.convertInventoryItems(gc.invUpdate.Items)
	gc.tracker.ApplyInventoryUpdate(items)
}

// convertInventoryItems copies the parsed items into state items through
// the reused conversion buffer. The state layer copies the item values
// into its own inventory map, so the buffer is not retained.
func (gc *GameClient) convertInventoryItems(
	source []fromgameserver.InventoryItem,
) []state.InventoryItem {
	gc.invItems = gc.invItems[:0]
	for _, item := range source {
		gc.invItems = append(gc.invItems, state.InventoryItem{
			ObjectID: item.ObjectID,
			ItemID:   item.ItemID,
			Count:    item.Count,
			Type1:    item.Type1,
			Type2:    item.Type2,
			Equipped: item.Equipped,
			BodyPart: item.BodyPart,
			Enchant:  item.Enchant,
			Change:   item.Change,
		})
	}

	return gc.invItems
}
