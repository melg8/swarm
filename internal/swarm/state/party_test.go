// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
    "testing"

    "github.com/stretchr/testify/require"
)

// TestPartyMirrorWalksTheWindowPackets pins the party mirror: the window
// replacement, the joined member, the vitals update, the removal and the end.
func TestPartyMirrorWalksTheWindowPackets(t *testing.T) {
    bot := NewBot("test1")

    bot.ApplyPartyAll([]PartyMember{
        {ObjectID: 2, Name: "test2", HP: 100, MaxHP: 200},
        {ObjectID: 3, Name: "test3", HP: 150, MaxHP: 200},
    })
    require.Equal(t, 3, bot.PartyCount())
    member, ok := bot.PartyMember(2)
    require.True(t, ok)
    require.Equal(t, "test2", member.Name)
    require.Equal(t, int32(100), member.HP)

    bot.ApplyPartyAdd(PartyMember{ObjectID: 4, Name: "test4"})
    require.Equal(t, 4, bot.PartyCount())

    bot.ApplyPartyUpdate(PartyMember{
        ObjectID: 2, Name: "test2", HP: 50, MaxHP: 200,
    })
    member, ok = bot.PartyMember(2)
    require.True(t, ok)
    require.Equal(t, int32(50), member.HP)

    bot.ApplyPartyDelete(3)
    require.Equal(t, 3, bot.PartyCount())
    _, ok = bot.PartyMember(3)
    require.False(t, ok)

    require.Len(t, bot.PartyMembers(), 2)

    bot.ClearParty()
    require.Equal(t, 1, bot.PartyCount())
    require.Empty(t, bot.PartyMembers())
}

// TestPartyAskRoundTrip pins the pending invitation bookkeeping.
func TestPartyAskRoundTrip(t *testing.T) {
    bot := NewBot("test2")
    bot.ApplyPartyAsk(100)

    requestor, at := bot.PendingPartyAsk()
    require.Equal(t, int32(100), requestor)
    require.False(t, at.IsZero())

    bot.ClearPartyAsk()
    requestor, at = bot.PendingPartyAsk()
    require.Zero(t, requestor)
    require.True(t, at.IsZero())
}

// TestPartyPositionsRefreshTheWorld pins the placement mirror: the packet
// updates the member view and seeds the world record of a player the client
// does not see, so the follower walk reads the leader position anyway.
func TestPartyPositionsRefreshTheWorld(t *testing.T) {
    bot := NewBot("test2")
    bot.SetCharacter("self", 200, 18, 10, 20, -30, 100, 40)
    bot.ApplyPartyAll([]PartyMember{{ObjectID: 100, Name: "test1"}})

    bot.ApplyPartyPositions([]PartyMember{
        {ObjectID: 100, Name: "test1", Level: 12, X: 1000, Y: 2000, Z: -3000},
        {ObjectID: 200, Name: "self", X: 10, Y: 20, Z: -30},
    })

    member, ok := bot.PartyMember(100)
    require.True(t, ok)
    require.Equal(t, int32(1000), member.X)
    require.Equal(t, int32(12), member.Level)

    x, y, z, ok := bot.ObjectPosition(100)
    require.True(t, ok)
    require.Equal(t, int32(1000), x)
    require.Equal(t, int32(2000), y)
    require.Equal(t, int32(-3000), z)
    require.Equal(t, "test1", bot.ObjectName(100))

    _, _, _, ok = bot.ObjectPosition(200)
    require.False(t, ok, "the self entry never seeds a player record")
}

// TestPartyResetClearsTheParty pins the session reset: a fresh session owns
// an empty party and no pending ask.
func TestPartyResetClearsTheParty(t *testing.T) {
    bot := NewBot("test1")
    bot.ApplyPartyAll([]PartyMember{{ObjectID: 2, Name: "test2"}})
    bot.ApplyPartyAsk(2)

    bot.ResetSession()

    require.Equal(t, 1, bot.PartyCount())
    requestor, _ := bot.PendingPartyAsk()
    require.Zero(t, requestor)
    bot.mu.RLock()
    require.Empty(t, bot.party.members)
    bot.mu.RUnlock()
}
