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

// hurtSelf sets the tracked health to a share of the maximum.
func hurtSelf(bot *state.Bot, percent float64) {
    bot.ApplyUserInfo(state.UserInfo{
        Name: "self", Level: 20, Race: 1, ClassID: 18,
        X: 10000, Y: 20000, Z: -3500,
        MaxHP: 1000, CurHP: int32(1000 * percent / 100),
        MaxMP: 100, CurMP: 100,
    })
}

// publishCommand records the leader command the followers read.
func publishCommand(hub *party.Hub, command party.Coordination) {
    hub.PublishCoordination("test1", command)
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

// TestFollowerWalksIntoTheFormationSlot pins the formation walk: a follower
// outside its slot clicks toward the leader anchor and inside the slot it
// stops walking and waits for the shared target.
func TestFollowerWalksIntoTheFormationSlot(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    hub := newPartyHub("test1", "test2")
    loop.SetParty(hub, "test2")
    setSelf(bot, 200, 10000)
    now := time.Unix(1_700_000_000, 0)
    publishCommand(hub, party.Coordination{
        Leader: "test1", First: "test1", FirstAt: now,
        X: 12000, Y: 20000, Z: -3500, At: now,
    })

    require.True(t, loop.partyTick(now), "the formation walk owns the tick")
    require.Len(t, game.walks, 1)

    setSelf(bot, 200, 12000)
    require.True(t, loop.partyTick(now.Add(time.Second)),
        "inside the slot the follower waits for the shared target")
    require.Len(t, game.walks, 1, "no extra formation click")
}

// TestFollowerIgnoresAStaleCommand pins the freshness guard: a logged out
// leader keeps its last command, and the follower never chases it.
func TestFollowerIgnoresAStaleCommand(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    hub := newPartyHub("test1", "test2")
    loop.SetParty(hub, "test2")
    setSelf(bot, 200, 10000)
    now := time.Unix(1_700_000_000, 0)
    publishCommand(hub, party.Coordination{
        Leader: "test1", First: "test1", X: 12000, Y: 20000,
        At: now.Add(-partyStateFreshness - time.Second),
    })

    require.False(t, loop.partyTick(now))
    require.Empty(t, game.walks)
}

// TestFarFollowerDropsItsPrivateFight pins the convergence rule: a member out
// of formation walks back even with a live target, and a member in formation
// waits for the group target instead of picking one of its own.
func TestFarFollowerDropsItsPrivateFight(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    hub := newPartyHub("test1", "test2")
    loop.SetParty(hub, "test2")
    setSelf(bot, 200, 10000)
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 500, TemplateID: 1000003, Attackable: true,
        X: 10100, Y: 20000, Z: -3500, Name: "Goblin",
    })
    loop.target = 500
    now := time.Now()
    publishCommand(hub, party.Coordination{
        Leader: "test1", First: "test1", FirstAt: now,
        X: 15000, Y: 20000, At: now,
    })

    require.True(t, loop.partyTick(now))
    require.Zero(t, loop.target)
    require.Len(t, game.walks, 1)

    publishCommand(hub, party.Coordination{
        Leader: "test1", First: "test1", FirstAt: now,
        X: 10000, Y: 20000, At: now,
    })

    require.True(t, loop.partyTick(now))
    require.Zero(t, loop.target, "a private pick is never resumed")
    require.Len(t, game.walks, 1)
}

// TestPartyRunsOutsideTheEngagePhase pins the every tick hook: a member that
// is not fighting still publishes its placement and walks into formation.
func TestPartyRunsOutsideTheEngagePhase(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    hub := newPartyHub("test1", "test2")
    loop.SetParty(hub, "test2")
    setSelf(bot, 200, 10000)
    now := time.Now()
    publishCommand(hub, party.Coordination{
        Leader: "test1", First: "test1", FirstAt: now,
        X: 12000, Y: 20000, At: now,
    })

    loop.tick()

    published, ok := hub.Member("test2")
    require.True(t, ok)
    require.Equal(t, int32(200), published.ObjectID)
    require.Len(t, game.walks, 1)
}

// TestTownTripYieldsToTheErrand pins the restock guard: a member on a town
// trip finishes the merchant errand instead of marching to the leader.
func TestTownTripYieldsToTheErrand(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    hub := newPartyHub("test1", "test2")
    loop.SetParty(hub, "test2")
    setSelf(bot, 200, 10000)
    loop.phase = phaseTownWalk
    now := time.Now()
    publishCommand(hub, party.Coordination{
        Leader: "test1", First: "test1", X: 12000, Y: 20000, At: now,
    })

    loop.partyTick(now)

    require.Empty(t, game.walks, "the errand owns the movement")
    published, ok := hub.Member("test2")
    require.True(t, ok)
    require.Equal(t, int32(200), published.ObjectID)
}

// TestFollowerFocusesTheLeaderTarget pins the simultaneous attack: the
// follower adopts the shared target instead of picking a mob of its own.
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
    publishCommand(hub, party.Coordination{
        Leader: "test1", First: "test1", FirstAt: now,
        TargetID: 500, X: 10000, Y: 20000, At: now,
    })

    loop.partyTick(now)

    require.Equal(t, int32(500), loop.target)
}

// TestLeaderRotatesTheFirstAttacker pins the damage spread: a hurt first
// attacker hands the lead to a healthy warrior and stands down from the
// attack until the new first had time to pull the mob.
func TestLeaderRotatesTheFirstAttacker(t *testing.T) {
    loop, _, bot, _ := newTripLoop()
    hub := newPartyHub("test1", "test2", "test3")
    loop.SetParty(hub, "test1")
    setSelf(bot, 100, 10000)
    hurtSelf(bot, 30)
    now := time.Unix(1_700_000_000, 0)
    hub.Publish(party.MemberState{
        Account: "test2", ObjectID: 200, HPPercent: 95,
        Alive: true, At: now,
    })
    hub.Publish(party.MemberState{
        Account: "test3", ObjectID: 300, HPPercent: 80,
        Alive: true, At: now,
    })

    loop.partyTick(now)

    command, ok := hub.CoordinationOf("test1")
    require.True(t, ok)
    require.Equal(t, "test2", command.First)
    require.True(t, loop.partyAttackHeld(now),
        "the demoted first stands down")
    require.False(t, loop.partyAttackHeld(now.Add(partyStandDown+
        time.Second)), "the hold expires")
}

// TestFirstAttackerStaysWhileHealthy pins the rotation hysteresis: a healthy
// first keeps the lead until another member beats it by the swap margin.
func TestFirstAttackerStaysWhileHealthy(t *testing.T) {
    loop, _, bot, _ := newTripLoop()
    hub := newPartyHub("test1", "test2")
    loop.SetParty(hub, "test1")
    setSelf(bot, 100, 10000)
    hurtSelf(bot, 90)
    now := time.Unix(1_700_000_000, 0)
    hub.Publish(party.MemberState{
        Account: "test2", ObjectID: 200, HPPercent: 95,
        Alive: true, At: now,
    })
    publishCommand(hub, party.Coordination{
        Leader: "test1", First: "test1", FirstAt: now, At: now,
    })

    loop.partyTick(now)

    command, ok := hub.CoordinationOf("test1")
    require.True(t, ok)
    require.Equal(t, "test1", command.First)
    require.False(t, loop.partyAttackHeld(now))
}

// TestGroupRestsTogether pins the shared rest: a leader command with the rest
// flag sits a member that reached its slot and keeps the tick owned.
func TestGroupRestsTogether(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    hub := newPartyHub("test1", "test2")
    loop.SetParty(hub, "test2")
    setSelf(bot, 200, 10000)
    hurtSelf(bot, 30)
    now := time.Now()
    publishCommand(hub, party.Coordination{
        Leader: "test1", First: "test1", FirstAt: now,
        Rest: true, X: 10000, Y: 20000, At: now,
    })

    require.True(t, loop.partyTick(now))
    require.Equal(t, 1, game.sits, "the group sits together")
    require.Empty(t, game.walks, "the member already stands in its slot")
}

// TestGroupRestGathersTheMember pins the rest walk: a resting member outside
// its slot walks in before it sits down.
func TestGroupRestGathersTheMember(t *testing.T) {
    loop, game, bot, _ := newTripLoop()
    hub := newPartyHub("test1", "test2")
    loop.SetParty(hub, "test2")
    setSelf(bot, 200, 10000)
    now := time.Now()
    publishCommand(hub, party.Coordination{
        Leader: "test1", First: "test1", FirstAt: now,
        Rest: true, X: 12000, Y: 20000, At: now,
    })

    require.True(t, loop.partyTick(now))
    require.Len(t, game.walks, 1)
    require.Zero(t, game.sits, "the member sits only after it arrived")
}

// TestLeaderStartsTheGroupRest pins the rest decision: a member that wants
// recovery sets the group flag until every living member recovered.
func TestLeaderStartsTheGroupRest(t *testing.T) {
    loop, _, bot, _ := newTripLoop()
    hub := newPartyHub("test1", "test2")
    loop.SetParty(hub, "test1")
    setSelf(bot, 100, 10000)
    hurtSelf(bot, 30)
    now := time.Now()
    hub.Publish(party.MemberState{
        Account: "test1", ObjectID: 100, HPPercent: 30,
        RestWanted: true, Alive: true, At: now,
    })
    hub.Publish(party.MemberState{
        Account: "test2", ObjectID: 200, HPPercent: 95,
        Alive: true, At: now,
    })

    command := loop.partyLeadTick(now, 10000, 20000, -3500)

    require.True(t, command.Rest, "one hurt member rests the whole group")

    hurtSelf(bot, 100)
    hub.Publish(party.MemberState{
        Account: "test1", ObjectID: 100, HPPercent: 100,
        Alive: true, At: now,
    })
    hub.Publish(party.MemberState{
        Account: "test2", ObjectID: 200, HPPercent: 100,
        Alive: true, At: now,
    })
    hub.PublishCoordination("test1", command)

    command = loop.partyLeadTick(now.Add(time.Second), 10000, 20000, -3500)

    require.False(t, command.Rest, "the group resumes when all recovered")
}
