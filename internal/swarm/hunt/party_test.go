// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "testing"
    "time"

    "github.com/stretchr/testify/require"

    "github.com/melg8/swarm/internal/swarm/party"
    "github.com/melg8/swarm/internal/swarm/state"
)

// newPartyHub builds the hub of one test group led by test1.
func newPartyHub(members ...string) *party.Hub {
    return party.NewHub(&party.Config{Groups: []party.Group{
        {Leader: "test1", Members: members},
    }})
}

// setSelf snaps the tracked character to the id and the position at y 20000.
func setSelf(bot *state.Bot, objectID int32, x int32) {
    bot.SetCharacter("self", objectID, 18, x, 20000, -3500, 100, 40)
}

// TestLeaderInvitesTheVisibleMember pins the leader invitation: a fresh
// published member visible in range gets one ask per pacing period, the far
// one and the member already in the party none.
func TestLeaderInvitesTheVisibleMember(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    hub := newPartyHub("test1", "test2", "test3")
    loop.SetParty(hub, "test1")
    setSelf(bot, 100, 10000)
    bot.ApplyPlayerInfo(state.PlayerInfo{
        ObjectID: 200, Name: "test2", X: 10500, Y: 20000, Z: -3500,
    })
    bot.ApplyPlayerInfo(state.PlayerInfo{
        ObjectID: 300, Name: "test3", X: 40000, Y: 20000, Z: -3500,
    })
    now := time.Unix(1_700_000_000, 0)
    hub.Publish(party.MemberState{
        Account: "test2", ObjectID: 200, X: 10500, Y: 20000, Z: -3500,
        Alive: true, At: now,
    })
    hub.Publish(party.MemberState{
        Account: "test3", ObjectID: 300, X: 40000, Y: 20000, Z: -3500,
        Alive: true, At: now,
    })

    loop.partyTick(now)
    require.Equal(t, []int32{200}, game.partyInvites,
        "only the visible member inside the invite range is asked")

    loop.partyTick(now)
    require.Len(t, game.partyInvites, 1, "the retry waits the pacing period")

    loop.partyTick(now.Add(partyInvitePeriod + time.Second))
    require.Equal(t, []int32{200, 200}, game.partyInvites)
}

// TestLeaderStopsInvitingAJoinedMember pins the party size guard: a member
// already in the small window is never asked again.
func TestLeaderStopsInvitingAJoinedMember(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    hub := newPartyHub("test1", "test2")
    loop.SetParty(hub, "test1")
    setSelf(bot, 100, 10000)
    bot.ApplyPlayerInfo(state.PlayerInfo{
        ObjectID: 200, Name: "test2", X: 10500, Y: 20000, Z: -3500,
    })
    bot.ApplyPartyAdd(state.PartyMember{ObjectID: 200, Name: "test2"})
    now := time.Unix(1_700_000_000, 0)
    hub.Publish(party.MemberState{
        Account: "test2", ObjectID: 200, X: 10500, Y: 20000, Z: -3500,
        Alive: true, At: now,
    })

    loop.partyTick(now)

    require.Empty(t, game.partyInvites)
}

// TestFollowerAcceptsTheLeaderAsk pins the trusted invitation: the configured
// leader object id is accepted and the ask is cleared.
func TestFollowerAcceptsTheLeaderAsk(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    hub := newPartyHub("test1", "test2")
    loop.SetParty(hub, "test2")
    setSelf(bot, 200, 10000)
    hub.Publish(party.MemberState{
        Account: "test1", ObjectID: 100, Alive: true, At: time.Now(),
    })
    bot.ApplyPartyAsk(100)

    loop.partyTick(time.Now())

    require.Equal(t, []int32{1}, game.partyAnswers)
    requestor, _ := bot.PendingPartyAsk()
    require.Zero(t, requestor, "the answered ask is cleared")
}

// TestFollowerDeclinesAStranger pins the stranger guard: an invitation from
// anything but the configured leader is declined.
func TestFollowerDeclinesAStranger(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    hub := newPartyHub("test1", "test2")
    loop.SetParty(hub, "test2")
    setSelf(bot, 200, 10000)
    bot.ApplyPartyAsk(999)

    loop.partyTick(time.Now())

    require.Equal(t, []int32{0}, game.partyAnswers)
}

// TestFollowerWalksAfterTheLeader pins the follow walk: a distant fresh leader
// makes the follower click toward it and the walk owns the tick; inside the
// catch up distance the walk ends.
func TestFollowerWalksAfterTheLeader(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    hub := newPartyHub("test1", "test2")
    loop.SetParty(hub, "test2")
    setSelf(bot, 200, 10000)
    now := time.Unix(1_700_000_000, 0)
    hub.Publish(party.MemberState{
        Account: "test1", ObjectID: 100, X: 12000, Y: 20000,
        Z: -3500, Alive: true, At: now,
    })

    require.True(t, loop.partyTick(now), "the follow walk owns the tick")
    require.Len(t, game.walks, 1)

    setSelf(bot, 200, 11800)
    require.False(t, loop.partyTick(now.Add(time.Second)),
        "inside the catch up distance the hunt resumes")
    require.False(t, loop.party.following)
}

// TestFollowerFollowsAStaleLeaderState pins the freshness guard: a logged out
// leader leaves its last placement, and the follower never chases it.
func TestFollowerFollowsAStaleLeaderState(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    hub := newPartyHub("test1", "test2")
    loop.SetParty(hub, "test2")
    setSelf(bot, 200, 10000)
    now := time.Unix(1_700_000_000, 0)
    hub.Publish(party.MemberState{
        Account: "test1", ObjectID: 100, X: 12000, Y: 20000,
        Z: -3500, Alive: true, At: now.Add(-partyStateFreshness -
            time.Second),
    })

    require.False(t, loop.partyTick(now))
    require.Empty(t, game.walks)
}

// TestPartyRunsOutsideTheEngagePhase pins the every tick hook: a member that
// walks to town still publishes its placement and follows its leader, so the
// hub does not go stale while the group travels.
func TestPartyRunsOutsideTheEngagePhase(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    hub := newPartyHub("test1", "test2")
    loop.SetParty(hub, "test2")
    setSelf(bot, 200, 10000)
    loop.phase = phaseTownWalk
    now := time.Now()
    hub.Publish(party.MemberState{
        Account: "test1", ObjectID: 100, X: 12000, Y: 20000,
        Z: -3500, Alive: true, At: now,
    })

    loop.tick()

    published, ok := hub.Member("test2")
    require.True(t, ok)
    require.Equal(t, int32(200), published.ObjectID)
    require.Len(t, game.walks, 1,
        "the follower walks after the leader outside the fight")
}

// TestFollowerFocusesTheLeaderTarget pins the focus fire: the follower adopts
// the living leader target in range instead of picking a mob of its own.
func TestFollowerFocusesTheLeaderTarget(t *testing.T) {
    loop, _, bot, _ := newTripLoop()
    hub := newPartyHub("test1", "test2")
    loop.SetParty(hub, "test2")
    setSelf(bot, 200, 10000)
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 500, TemplateID: 1000003, Attackable: true,
        X: 10300, Y: 20000, Z: -3500, Name: "Goblin",
    })
    now := time.Unix(1_700_000_000, 0)
    hub.Publish(party.MemberState{
        Account: "test1", ObjectID: 100, TargetID: 500,
        X: 10000, Y: 20000, Z: -3500, Alive: true, At: now,
    })

    loop.partyTick(now)

    require.Equal(t, int32(500), loop.target)
}
