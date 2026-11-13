// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package connection

import (
    "fmt"
    "strconv"

    "github.com/melg8/swarm/internal/swarm/gear"
    togameserver "github.com/melg8/swarm/internal/swarm/packets/to_game_server"
    "github.com/melg8/swarm/internal/swarm/state"
)

// AttackTarget repeats the attack request for an already selected target:
// the second click of the Mobius semantics, which the server resolves to
// onForcedAttack and the ATTACK intention of the player AI.
func (gc *GameClient) AttackTarget(objectID int32) error {
    if objectID == 0 {
        return nil
    }
    x, y, z, ok := gc.tracker.ObjectPosition(objectID)
    if !ok {
        return fmt.Errorf("failed to attack target %d: object unknown",
            objectID)
    }
    if err := gc.sendAttackRequest(objectID, x, y, z); err != nil {
        return fmt.Errorf("failed to attack: %w", err)
    }

    return nil
}

func (gc *GameClient) sendAttackRequest(
    targetID int32, x int32, y int32, z int32,
) error {
    request := togameserver.NewAttackRequestPacket()
    request.TargetID = targetID
    request.X = x
    request.Y = y
    request.Z = z

    return gc.sendPacket(request)
}

// PickupItem clicks a ground item: the server walks the character to it
// and picks it up.
func (gc *GameClient) PickupItem(item state.LootItem) error {
    request := togameserver.NewActionRequestPacket()
    request.ObjectID = item.ObjectID
    request.X = item.X
    request.Y = item.Y
    request.Z = item.Z
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to pick up item: %w", err)
    }
    gc.tracker.RecordEvent("picking up " + item.Name)

    return nil
}

// DestroyItem destroys inventory items to free slots or weight.
func (gc *GameClient) DestroyItem(objectID int32, count int32) error {
    request := togameserver.NewRequestDestroyItem()
    request.ObjectID = objectID
    request.Count = count
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to destroy item: %w", err)
    }

    return nil
}

// UseItem uses an inventory item: an equippable toggles its equipped
// state (one packet for both directions, UseItem.runImpl) and every other
// item runs its handler.
func (gc *GameClient) UseItem(objectID int32) error {
    request := togameserver.NewRequestUseItem()
    request.ObjectID = objectID
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to use item: %w", err)
    }
    gc.tracker.RecordEvent("using item " + strconv.Itoa(int(objectID)))

    return nil
}

// Say sends a chat message on the ChatType channel; only the whisper
// channel carries a recipient. An invalid chat disconnects the session.
func (gc *GameClient) Say(text string, channel int32, target string) error {
    request := togameserver.NewSay(text, channel)
    request.Target = target
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to send chat: %w", err)
    }

    return nil
}

// DropItem drops an inventory item at the given position: the server
// accepts drops only within 150 units of the player
// (RequestDropItem.runImpl) and splits a partial stack itself.
func (gc *GameClient) DropItem(
    objectID int32, count int32, x int32, y int32, z int32,
) error {
    request := togameserver.NewRequestDropItem()
    request.ObjectID = objectID
    request.Count = count
    request.X = x
    request.Y = y
    request.Z = z
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to drop item: %w", err)
    }
    gc.tracker.RecordEvent("dropping " + strconv.Itoa(int(count)) +
        " of item " + strconv.Itoa(int(objectID)))

    return nil
}

// SellItems sells items to the targeted merchant through the standard
// sell list (id 0); the server prices every item at referencePrice/2.
func (gc *GameClient) SellItems(items []state.InventoryItem) error {
    request := togameserver.NewRequestSellItemPacket()
    entries := make([]togameserver.SellItemEntry, 0, len(items))
    for _, item := range items {
        entries = append(entries, togameserver.SellItemEntry{
            ObjectID: item.ObjectID,
            ItemID:   item.ItemID,
            Count:    item.Count,
        })
    }
    request.Items = entries
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to sell items: %w", err)
    }

    return nil
}

// BuyItems buys from the buylist of the targeted merchant: the server
// requires the merchant within the interaction distance and prices every
// entry itself (the reference price plus the town tax), so the request
// carries item ids and counts only. Buying shares the transaction flood
// protector with selling, so the caller paces it like a sell batch.
func (gc *GameClient) BuyItems(listID int32, items []gear.Purchase) error {
    request := togameserver.NewRequestBuyItemPacket()
    request.ListID = listID
    entries := make([]togameserver.BuyItemEntry, 0, len(items))
    for _, item := range items {
        entries = append(entries, togameserver.BuyItemEntry{
            ItemID: item.ItemID,
            Count:  item.Count,
        })
    }
    request.Items = entries
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to buy items: %w", err)
    }

    return nil
}

// RequestInventory asks the server for the full inventory list.
func (gc *GameClient) RequestInventory() error {
    if err := gc.sendPacket(&togameserver.RequestItemList{}); err != nil {
        return fmt.Errorf("failed to request item list: %w", err)
    }

    return nil
}

// ActionSitStand toggles sitting and standing (RequestActionUse action
// 0) to rest at low HP; the server refuses it while moving, casting or
// attacking, so the caller sends it only while idle.
func (gc *GameClient) ActionSitStand() error {
    request := togameserver.NewRequestActionUsePacket()
    request.ActionID = togameserver.ActionSitStand
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to send action use: %w", err)
    }

    return nil
}

// ClickObject selects an object through the plain client click (Action 0x04):
// the NpcClick handler selects the npc and remembers it as the last folk talked
// to, which the RequestAcquireSkill trainer resolves through.
func (gc *GameClient) ClickObject(objectID int32) error {
    if objectID == 0 {
        return nil
    }
    x, y, z, ok := gc.tracker.ObjectPosition(objectID)
    if !ok {
        return fmt.Errorf(
            "failed to click object %d: object unknown", objectID)
    }
    request := togameserver.NewActionRequestPacket()
    request.ObjectID = objectID
    request.X = x
    request.Y = y
    request.Z = z
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to click object: %w", err)
    }
    gc.tracker.RecordEvent("talking to the teacher")

    return nil
}

// InteractPull fires the attack analog of the npc interaction: the
// second plain click on an already selected npc. The NpcClick interact
// branch walks the character to the npc along the straight line when out
// of the interaction distance (thinkInteract, 36 units) and opens the
// dialog; it is deliberately not the attack request (AttackTarget, the
// 0x0A packet the guard engage drives), so no forced attack intention
// starts and the folk npc never takes damage. The pull is safe at any
// range, since an unselected npc only takes the selection.
func (gc *GameClient) InteractPull(objectID int32) error {
    if objectID == 0 {
        return nil
    }
    x, y, z, ok := gc.tracker.ObjectPosition(objectID)
    if !ok {
        return fmt.Errorf(
            "failed to interact pull %d: object unknown", objectID)
    }
    request := togameserver.NewActionRequestPacket()
    request.ObjectID = objectID
    request.X = x
    request.Y = y
    request.Z = z
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to interact pull: %w", err)
    }
    gc.tracker.RecordEvent("attack analog pull to the npc")

    return nil
}

// ClearTarget drops a selection the loop no longer owns: C1 has no
// deselect request and the server never clears a selection on its own,
// so the self click is the official client way out (Action 0x04 on the
// own object id selects the character itself, and the follow intention
// on self moves nothing). A selection that already is the own object is
// left alone, since a second self click only re-sends ActionFailed.
func (gc *GameClient) ClearTarget() error {
    objectID := gc.tracker.SelfObjectID()
    selected := gc.tracker.SelfTargetID()
    if objectID == 0 || selected == 0 || selected == objectID {
        return nil
    }
    x, y, z, ok := gc.tracker.SelfPosition()
    if !ok {
        x, y, z = 0, 0, 0
    }
    request := togameserver.NewActionRequestPacket()
    request.ObjectID = objectID
    request.X = x
    request.Y = y
    request.Z = z
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to clear the target: %w", err)
    }
    gc.tracker.RecordEvent("clearing the target")

    return nil
}

// AcquireSkill learns one lesson of the class skill tree at the teacher the
// character last talked to (see ClickObject): the server charges the SP,
// consumes the required skill book and answers with a fresh SkillList.
func (gc *GameClient) AcquireSkill(skillID int32, level int32) error {
    request := togameserver.NewRequestAcquireSkillPacket()
    request.SkillID = skillID
    request.Level = level
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to acquire skill: %w", err)
    }
    gc.tracker.RecordEvent(fmt.Sprintf(
        "learning skill %d level %d", skillID, level))

    return nil
}

// UseMagicSkill casts a learned active skill (a strike at the selected
// target, a self buff on the caster); the server runs the range, mana and
// reuse checks and answers the refusals with ActionFailed.
func (gc *GameClient) UseMagicSkill(skillID int32) error {
    request := togameserver.NewRequestMagicSkillUsePacket()
    request.SkillID = skillID
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to use magic skill: %w", err)
    }

    return nil
}

// RestartAtVillage revives a dead character at the nearest village restart
// point (RequestRestartPoint 0x6D type 0), like the death dialog of the
// official client; the server refuses it while the character is alive.
func (gc *GameClient) RestartAtVillage() error {
    request := togameserver.NewRequestRestartPointPacket()
    request.PointType = togameserver.RestartTypeVillage
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to send restart point: %w", err)
    }

    return nil
}
