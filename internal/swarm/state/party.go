// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import "time"

// The party view of the session: the members the server reported through the
// party small window and the pending invitation the hunt loop answers. The
// party is a server side fact (the C1 party packets own it), so the tracker
// only mirrors what arrived; the hunt decisions read the mirrored view.

// PartyMember is one member of the party as the server reported it.
type PartyMember struct {
    ObjectID int32
    Name     string
    HP       int32
    MaxHP    int32
    MP       int32
    MaxMP    int32
    ClassID  int32
    Level    int32
    X        int32
    Y        int32
    Z        int32
}

// partyStore is the mirrored party of the session.
type partyStore struct {
    members map[int32]PartyMember
    // askFrom is the requestor of the pending AskJoinParty; zero when none
    // arrived or the hunt loop answered it.
    askFrom int32
    askAt   time.Time
}

// newPartyStore creates the empty party of a session.
func newPartyStore() partyStore {
    return partyStore{
        members: make(map[int32]PartyMember),
        askFrom: 0,
        askAt:   time.Time{},
    }
}

// ApplyPartyAll replaces the party with the members of a PartySmallWindowAll.
func (b *Bot) ApplyPartyAll(members []PartyMember) {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.party.members = make(map[int32]PartyMember, len(members))
    for _, member := range members {
        b.party.members[member.ObjectID] = member
    }
    b.touch()
}

// ApplyPartyAdd adds one member of a PartySmallWindowAdd.
func (b *Bot) ApplyPartyAdd(member PartyMember) {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.party.members[member.ObjectID] = member
    b.touch()
}

// ApplyPartyUpdate refreshes one member of a PartySmallWindowUpdate.
func (b *Bot) ApplyPartyUpdate(member PartyMember) {
    b.mu.Lock()
    defer b.mu.Unlock()
    known, ok := b.party.members[member.ObjectID]
    if ok {
        member.X = known.X
        member.Y = known.Y
        member.Z = known.Z
    }
    b.party.members[member.ObjectID] = member
    b.touch()
}

// ApplyPartyDelete removes one member of a PartySmallWindowDelete.
func (b *Bot) ApplyPartyDelete(objectID int32) {
    b.mu.Lock()
    defer b.mu.Unlock()
    delete(b.party.members, objectID)
    b.touch()
}

// ClearParty empties the party and the pending ask: a PartySmallWindowDeleteAll
// ends the party, an ended session ends both.
func (b *Bot) ClearParty() {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.party.members = make(map[int32]PartyMember)
    b.party.askFrom = 0
    b.party.askAt = time.Time{}
    b.touch()
}

// PartyMembers returns a copy of the party members.
func (b *Bot) PartyMembers() []PartyMember {
    b.mu.RLock()
    defer b.mu.RUnlock()
    members := make([]PartyMember, 0, len(b.party.members))
    for _, member := range b.party.members {
        members = append(members, member)
    }

    return members
}

// PartyMember returns one member of the party.
func (b *Bot) PartyMember(objectID int32) (PartyMember, bool) {
    b.mu.RLock()
    defer b.mu.RUnlock()
    member, ok := b.party.members[objectID]

    return member, ok
}

// PartyCount reports the party size, including the self member.
func (b *Bot) PartyCount() int {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return 1 + len(b.party.members)
}

// ApplyPartyAsk records a pending AskJoinParty invitation.
func (b *Bot) ApplyPartyAsk(requestorObjectID int32) {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.party.askFrom = requestorObjectID
    b.party.askAt = b.now()
    b.touch()
}

// PendingPartyAsk returns the pending invitation requestor and its arrival
// time. The hunt loop ignores an ask older than its acceptance window.
func (b *Bot) PendingPartyAsk() (int32, time.Time) {
    b.mu.RLock()
    defer b.mu.RUnlock()

    return b.party.askFrom, b.party.askAt
}

// ClearPartyAsk forgets the pending invitation after the answer went out.
func (b *Bot) ClearPartyAsk() {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.party.askFrom = 0
    b.party.askAt = time.Time{}
}

// ApplyPartyPositions mirrors a PartyMemberPosition packet: the placements
// refresh the world records of the players, so the follow walk reads the leader
// position even when the leader left the visible range.
func (b *Bot) ApplyPartyPositions(members []PartyMember) {
    b.mu.Lock()
    defer b.mu.Unlock()
    now := b.now()
    for _, member := range members {
        if member.ObjectID == b.selfID {
            continue
        }
        if known, ok := b.party.members[member.ObjectID]; ok {
            known.X = member.X
            known.Y = member.Y
            known.Z = member.Z
            known.Level = member.Level
            known.ClassID = member.ClassID
            b.party.members[member.ObjectID] = known
        }
        obj, cold := b.upsertLocked(member.ObjectID, KindPlayer)
        obj.X = member.X
        obj.Y = member.Y
        obj.Z = member.Z
        obj.DestX = member.X
        obj.DestY = member.Y
        obj.DestZ = member.Z
        obj.Moving = false
        obj.Dead = false
        cold.Name = member.Name
        cold.UpdatedAt = now.UnixNano()
    }
    b.touch()
}
