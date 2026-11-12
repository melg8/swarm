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

// AttackTarget repeats the attack request for a target that is already
// selected. This is the second click of the Mobius semantics: the server
// resolves it to onForcedAttack and notifies the player AI with the
// ATTACK intention, which starts the chase and the auto attack.
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

// sendAttackRequest serializes and sends one AttackRequest packet.
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
// and adds it to the inventory (the movement is broadcast as
// MoveToLocation and the pickup as GetItem with a StopMove to self).
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

// UseItem uses an inventory item. Equippable items toggle their
// equipped state (the same packet equips and unequips, see
// UseItem.runImpl -> useEquippableItem), other items run their item
// handler. The web UI drives it from the equipment widget: double
// click and drag-and-drop of the cells.
func (gc *GameClient) UseItem(objectID int32) error {
    request := togameserver.NewRequestUseItem()
    request.ObjectID = objectID
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to use item: %w", err)
    }
    gc.tracker.RecordEvent("using item " + strconv.Itoa(int(objectID)))

    return nil
}

// Say sends a chat message on the given ChatType client channel: the
// web UI chat input drives it through the say command of the hunt
// loop. The whisper channel carries the recipient name, every other
// channel leaves it empty. The Say2 packet validates itself (an
// invalid chat disconnects the session server side).
func (gc *GameClient) Say(text string, channel int32, target string) error {
    request := togameserver.NewSay(text, channel)
    request.Target = target
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to send chat: %w", err)
    }

    return nil
}

// DropItem drops an inventory item on the ground at the given world
// position: the server only accepts drops within 150 units of the
// player (see RequestDropItem.runImpl), so the caller passes the
// character position. Stackable items drop a partial stack through the
// count, the server splits the stack itself.
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

// SellItems sells inventory items to the targeted merchant. The packet
// uses the standard inventory sell list of the official client (list id
// 0): the server prices every item itself at referencePrice/2, answers
// with InventoryUpdate removals and adds the adena.
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

// BuyItems buys items from the buylist of the targeted merchant. The
// server requires the selected merchant within the interaction
// distance and prices every entry itself (the reference price with
// the town tax), so the request carries item ids and counts only.
// Buying shares the transaction flood protector with selling, so the
// caller paces it like the sell batches.
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

// ActionSitStand toggles between sitting and standing (RequestActionUse
// action 0). The hunt loop uses it to rest at low HP: the sitting
// regeneration is faster. The server refuses the transition while
// moving, casting or attacking, so the caller sends it only while idle.
func (gc *GameClient) ActionSitStand() error {
    request := togameserver.NewRequestActionUsePacket()
    request.ActionID = togameserver.ActionSitStand
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to send action use: %w", err)
    }

    return nil
}

// ClickObject selects a world object through the plain client click
// (Action 0x04): the NpcClick handler of the server selects the npc
// AND remembers it as the last folk the character talked to - the
// RequestAcquireSkill learning resolves its trainer through exactly
// that. The hunt loop clicks the skill teacher with it before the
// lesson requests.
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
// second plain client click on an already selected npc (the client
// double click). The Mobius NpcClick handler resolves it to the
// interact branch - for a folk npc out of the interaction distance
// the player AI takes the INTERACT intention and walks the character
// to the npc along the straight line (thinkInteract moves the pawn
// to 36 units), opening the dialog on arrival; within the distance
// it just opens the dialog. The interaction distance is therefore
// met whatever the geometry between the character and the npc does.
// It is deliberately NOT the real attack request (AttackTarget, the
// 0x0A packet the guard engage drives): no forced attack intention,
// no swings, the folk npc never takes damage - and a plain click on
// an npc the character does not hold yet only selects it, so the
// pull is safe at any range.
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

// ClearTarget drops a selection the loop no longer owns (see the
// hunt's clearServerSelection): the client cannot unselect directly -
// the C1 protocol has no deselect request and the server never clears
// a selection on its own (only the next selection replaces it) - so
// the self click is the official client way out: Action 0x04 on the
// own object id runs through the PlayerClick handler, selects the
// character itself (MyTargetSelected of the own id, the friendly npc
// or the abandoned fight target is gone) and the follow intention on
// self moves nothing. A selection that already IS the own object is
// left alone: a second self click only re-sends ActionFailed and
// re-arms the follow-self intention.
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

// AcquireSkill learns one lesson of the class skill tree at the
// teacher the character last talked to (see ClickObject): the server
// charges the SP, consumes the required skill book and answers with
// a fresh SkillList.
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

// UseMagicSkill casts a learned active skill: a strike at the
// selected target, a self buff on the caster. The server runs the
// cast flow (the range check, the mana cost, the reuse delay) and
// answers the refusals with ActionFailed.
func (gc *GameClient) UseMagicSkill(skillID int32) error {
    request := togameserver.NewRequestMagicSkillUsePacket()
    request.SkillID = skillID
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to use magic skill: %w", err)
    }

    return nil
}

// RestartAtVillage revives a dead character at the nearest village
// restart point, exactly like the death dialog of the official client
// (RequestRestartPoint 0x6D type 0). The server refuses the request
// while the character is alive, so the caller sends it only after death.
func (gc *GameClient) RestartAtVillage() error {
    request := togameserver.NewRequestRestartPointPacket()
    request.PointType = togameserver.RestartTypeVillage
    if err := gc.sendPacket(request); err != nil {
        return fmt.Errorf("failed to send restart point: %w", err)
    }

    return nil
}
