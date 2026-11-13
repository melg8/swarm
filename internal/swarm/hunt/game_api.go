// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "github.com/melg8/swarm/internal/swarm/gear"
    "github.com/melg8/swarm/internal/swarm/state"
)

// GameAPI abstracts the game actions the hunt loop needs. The
// connection.GameClient implements it.
type GameAPI interface {
    // AttackTarget sends the attack request for a target: the Mobius
    // server answers the first request for a new target with
    // MyTargetSelected (the request only selects it) and resolves the
    // repeated request to a forced attack.
    AttackTarget(objectID int32) error
    // WalkTo makes the character walk to a world point, like a ground
    // click of the official client.
    WalkTo(x int32, y int32, z int32) error
    // CursorKeyWalkTo sends the keyboard-mode move request (the
    // cursor keys of the official client, MoveToLocation movement
    // mode 0): the server arms the cursor key movement of the
    // session and every following ValidatePosition claim moves the
    // character server-side without any click validation - the
    // cursor key escape of a click-refusing cell (see
    // Loop.beginCursorKeyEscape).
    CursorKeyWalkTo(x int32, y int32, z int32) error
    // ClaimValidatePosition reports a claimed client position: while
    // the cursor key movement is armed the server syncs the claim
    // straight into the world and broadcasts it, so the claims walk
    // the character - the arrow-key movement stream of the official
    // client, the only movement a click-refusing cell answers.
    ClaimValidatePosition(x int32, y int32, z int32, heading int32) error
    // PickupItem clicks a ground item to walk to it and pick it up.
    PickupItem(item state.LootItem) error
    // ActionSitStand toggles between sitting and standing.
    ActionSitStand() error
    // RestartAtVillage revives a dead character at the nearest village.
    RestartAtVillage() error
    // DestroyItem destroys inventory items.
    DestroyItem(objectID int32, count int32) error
    // SellItems sells inventory items to the shop merchant.
    SellItems(items []state.InventoryItem) error
    // BuyItems buys items from the buylist of the selected merchant
    // (one list per request, paced by the transaction flood
    // protector).
    BuyItems(listID int32, items []gear.Purchase) error
    // UseItem uses an inventory item: equippable items toggle their
    // equipped state, the same packet equips and unequips.
    UseItem(objectID int32) error
    // DropItem drops an inventory item on the ground at the given
    // position (the server accepts drops at the feet only).
    DropItem(objectID int32, count int32, x int32, y int32, z int32) error
    // ClickObject selects a world object through the plain client
    // click: the server NpcClick handler selects the npc and
    // remembers it as the last folk the character talked to -
    // the teacher the lesson requests resolve their trainer
    // through.
    ClickObject(objectID int32) error
    // InteractPull fires the attack analog of the npc interaction: the
    // second plain click on an already selected npc. The Mobius NpcClick
    // handler resolves it to the interact branch: out of the interaction
    // distance the player AI takes the INTERACT intention and walks the
    // character to the npc along the straight line. It is not the attack
    // request (AttackTarget): no forced attack intention, no swings, the
    // folk npc never takes damage.
    InteractPull(objectID int32) error
    // ClearTarget drops the npc selection a conversation left
    // behind: the self click replaces the server side selection
    // (see GameClient.ClearTarget), so the leftover villager
    // never reads as a hunt target of the next engage.
    ClearTarget() error
    // AcquireSkill learns one lesson of the class skill tree at
    // the last folk teacher: the SP is charged and the demanded
    // spellbook consumed.
    AcquireSkill(skillID int32, level int32) error
    // UseMagicSkill casts a learned active skill - a strike at
    // the selected target or a self buff.
    UseMagicSkill(skillID int32) error
    // RequestLogout ends the session: the logout packet goes out
    // first (the server answers it while the combat stance lapsed)
    // and the connection closes either way (the server stores a
    // character that left mid combat fifteen seconds after the
    // combat ends).
    RequestLogout() error
    // SendBypass sends a RequestBypassToServer command to the server
    // (the gatekeeper teleport flow, the H-002 verification). The
    // command is the raw bypass string (the client strips the
    // "bypass -h " prefix the html button carries).
    SendBypass(command string) error
    // Say sends a chat message on the given ChatType client channel
    // (the web UI chat input). The whisper channel carries the
    // recipient name, every other channel an empty target.
    Say(text string, channel int32, target string) error
    // LastHTMLDialog returns the npc object id and the html body of
    // the last NpcHTMLMessage the server sent (the gatekeeper dialog
    // the bot reads the bypass buttons from). A zero npcObjID means
    // no dialog arrived yet.
    LastHTMLDialog() (npcObjID int32, html string)
    // LastHTMLDialogArrival returns the last dialog page together
    // with its arrival generation: the count of NpcHTMLMessage
    // packets the connection received at the moment that page
    // arrived. The dialog walker gates its page waits on the
    // generation - a page left over from a previous conversation of
    // the same npc (the recurring Newbie Guide stop) never
    // satisfies the wait of a fresh talk, so the first bypass goes
    // out only after the server opened the dialog it belongs to.
    LastHTMLDialogArrival() (npcObjID int32, html string, gen uint64)
}
