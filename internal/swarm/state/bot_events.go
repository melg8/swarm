// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
    "time"
)

// CountPacket accounts one received packet.
func (b *Bot) CountPacket() {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.packets++
    b.packetWindow.note(b.now().Unix())
}

// RecordEvent appends a message to the rolling event log.
func (b *Bot) RecordEvent(message string) {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.recordLocked(message)
}

// ApplyActionFailed records the arrival of an ActionFailed (0x35)
// answer of the server. The packet carries no request identity, so
// only the arrival time is stored: the hunt loop correlates it with
// the walk request it sent (refusalEvidence) - a click that got the
// answer while the character stood still is a server side refusal
// the offline click validation cannot see. The version stays
// untouched: the field feeds the hunt decisions and the state dump,
// not the SSE snapshot, so the encode path pays nothing for it.
func (b *Bot) ApplyActionFailed(at time.Time) {
    b.mu.Lock()
    b.actionFailedAt = at
    b.mu.Unlock()
}

// LastActionFailed returns the arrival time of the last ActionFailed
// answer of the server (the zero time when none arrived).
func (b *Bot) LastActionFailed() time.Time {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.actionFailedAt
}

// ApplyMoveRequestSent records the send time of the last walk click
// (MoveToLocation 0x01): the ActionFailed answer the refusal channel
// correlates has to belong to the click, and the only other requests
// of the session (the equips, the skills, the transactions) answer
// ActionFailed the same way, so their send times tell the correlation
// which arrival may be attributed to the click at all (see
// OtherRequestBetween).
func (b *Bot) ApplyMoveRequestSent(at time.Time) {
    b.mu.Lock()
    b.moveRequestAt = at
    b.mu.Unlock()
}

// ApplyOtherRequestSent records the send time of the last NON walk
// request of the session - an action request the server may answer
// with ActionFailed (an equip, a skill, a transaction; the send path
// skips the maintenance stream that never sees that answer, see
// connection.silentSessionOpcodes): an ActionFailed arriving after
// such a request answers it at least as likely as the walk click, so
// the refusal channel skips the attribution (see OtherRequestBetween).
func (b *Bot) ApplyOtherRequestSent(at time.Time) {
    b.mu.Lock()
    b.otherRequestAt = at
    b.mu.Unlock()
}

// OtherRequestBetween reports whether a non walk request was sent
// inside the answer window (after the walk click, before the
// ActionFailed arrival): the answer packet carries no request
// identity, so an arrival with such a request in flight may answer
// that request instead of the click - attributing it to the click
// would latch a refusal that never happened (the equip refusals of
// the gear swaps ride the walk clicks exactly this way).
//
// The window bounds are INCLUSIVE: the coarse Windows clock stamps
// the walk click, the action request and the arrival inside one
// timer tick, so an exclusive comparison reads the same-tick request
// as outside the window and attributes its answer to the click (the
// TestSendPacketAttributesOnlyTheAnsweredRequests failure of the
// 2026-11-09 host). A request on either boundary may own the answer,
// and the conservative reading skips the attribution.
func (b *Bot) OtherRequestBetween(after, before time.Time) bool {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return !b.otherRequestAt.Before(after) && !b.otherRequestAt.After(before)
}

// EventSink receives every recorded event with its timestamp. The
// session journal installs it to mirror the full event story into its
// persistent file. The contract is strict: the sink is called while
// the bot write lock is held, so it must never call back into the
// tracker and must never block (the journal sink hands the line to a
// buffered channel and drops on overflow).
type EventSink func(at time.Time, message string)

// SetEventSink installs (or with nil removes) the event observer. It
// must be called before the session traffic starts (the wiring in
// cmd/swarm installs it right after the tracker is created).
func (b *Bot) SetEventSink(sink EventSink) {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.eventSink = sink
}

// NewestEvents returns up to limit newest events of the rolling log
// in chronological order. The state dump reads a deeper window than
// the snapshot carries (the web UI log tail keeps the last entries
// only, the debug dump wants the whole story of the session).
func (b *Bot) NewestEvents(limit int) []Event {
    b.mu.RLock()
    defer b.mu.RUnlock()
    events := make([]Event, 0, min(b.log.length, limit))

    return b.log.appendNewest(events, limit)
}

// upsertLocked returns the pointers to the hot and cold records of
// the object id or appends fresh ones (see objectStore.upsertLocked).
// The caller must hold the write lock.
func (b *Bot) upsertLocked(
    objectID int32, kind ObjectKind,
) (*objectHot, *objectCold) {
    return b.world.upsertLocked(objectID, kindCode(kind))
}

// objectLocked returns the pointers to the hot and cold records of
// the object id, both nil when the id is unknown. The caller must
// hold a lock.
func (b *Bot) objectLocked(objectID int32) (*objectHot, *objectCold) {
    return b.world.lookupLocked(objectID)
}

// removeObjectAtLocked frees a slot of the dense object array (see
// objectStore.removeAtLocked). The caller must hold the write lock.
func (b *Bot) removeObjectAtLocked(slot int32, objectID int32) {
    b.world.removeAtLocked(slot, objectID)
}

// charObjectID returns the object id of the self player, zero when the
// character is unknown. The caller must hold a lock.
func (b *Bot) charObjectID() int32 {
    return b.selfID
}

// touch bumps the version and the update timestamp.
func (b *Bot) touch() {
    b.version++
    b.updated = b.now()
}

// clearCharMovement resets the self movement tracking. The caller must
// hold the write lock.
func (b *Bot) clearCharMovement() {
    b.char.Moving = false
    b.char.DestX = b.char.X
    b.char.DestY = b.char.Y
    b.char.DestZ = b.char.Z
    b.char.MoveAt = b.now()
}

// applySelfPositionSampleLocked adopts a position sample of the played
// character that carries no movement state of its own: a UserInfo
// refresh, the target position of a pawn chase, an attack hit. The
// sample is the server position at packet build time, so a moving
// character is re-anchored to the sample arrival: the client
// projection keeps the destination and replays the ticks from the
// fresh point, exactly like a movement broadcast of the sampled
// packet would. Leaving the old anchor in place made the projection
// run ahead by the avenue already covered and snap back on the next
// broadcast. The caller must hold the write lock.
func (b *Bot) applySelfPositionSampleLocked(
    x int32, y int32, z int32, now time.Time,
) {
    b.char.X = x
    b.char.Y = y
    b.char.Z = z
    if b.char.Moving && now.After(b.char.MoveAt) {
        b.char.MoveAt = now
    }
}

// clearSelfTargetLocked drops the target of the character and records
// the transition. The Mobius server keeps a killed or despawned target
// selected on its side (MyTargetSelected only answers new selections and
// the own TargetUnselected broadcast is the sole removal notice), so the
// tracker mirrors the official client: the target is gone once it died
// or was removed from the world. The fight state follows the target - a
// corpse or a removed object cannot be the object of an ongoing fight.
// The 2026-11-10 run-33 acceptance sampled the corpse of the killed mob
// at 781 units on the loot walk of that same kill (the fight target
// survived the death) and failed the bow band by one unit. The caller
// must hold the write lock.
func (b *Bot) clearSelfTargetLocked(reason string) {
    if b.char.TargetID == 0 {
        return
    }
    if b.char.FightingTargetID == b.char.TargetID {
        b.char.FightingTargetID = 0
    }
    b.char.TargetID = 0
    b.touch()
    b.recordLocked("target cleared: " + reason)
}

// recordLocked appends an event to the rolling log (see eventLog)
// and hands it to the installed event sink. The caller must hold the
// write lock; the sink contract forbids any re-entry into the tracker
// (see SetEventSink).
func (b *Bot) recordLocked(message string) {
    at := b.now()
    b.log.record(message, at)
    if b.eventSink != nil {
        b.eventSink(at, message)
    }
}

// recordCombatEventLocked appends one combat animation beat (see
// combatFeed.record). The caller must hold the state write lock.
func (b *Bot) recordCombatEventLocked(e CombatEvent) {
    b.combat.record(e, b.now())
}
