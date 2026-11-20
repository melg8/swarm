// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The group play of the fleet: the leader invites the visible group members
// and the followers walk after the leader, adopt its target and answer the
// invitation of the configured leader only. The server party is the real C1
// party (RequestJoinParty / RequestAnswerJoinParty); the process wide hub only
// carries the live placement and the current target between the sessions of
// one fleet, because the follower decisions need the leader state even when
// the game client lost sight of it.

import (
    "math"
    "strings"
    "time"

    togameserver "github.com/melg8/swarm/internal/swarm/packets/to_game_server"
    "github.com/melg8/swarm/internal/swarm/party"
    "github.com/melg8/swarm/internal/swarm/state"
)

const (
    // partyInviteRange is the planar distance an invitation accepts: the
    // server refuses an invite to a target outside the known list, so the
    // leader only invites the members it can see nearby.
    partyInviteRange = 900.0
    // partyFollowDistance is the planar distance a follower starts walking
    // after the leader; partyCatchUpDistance is the hysteresis that ends the
    // walk, so a marching leader never toggles the follow every tick.
    partyFollowDistance  = 450.0
    partyCatchUpDistance = 280.0
    // partyTargetRange bounds the focus fire: the follower adopts the leader
    // target only when it sits inside this distance. Past
    // partyConvergenceRange a follower drops its own fight to rejoin the group.
    partyTargetRange      = 1200.0
    partyConvergenceRange = 2400.0
    // partyInvitePeriod paces the invitation retries per member: the server
    // refuses a second ask while the first one is pending.
    partyInvitePeriod = 6 * time.Second
    // partyAskWindow bounds the age of an unanswered invitation: an ask older
    // than this is dropped instead of answered late.
    partyAskWindow = 15 * time.Second
    // partyStateFreshness bounds the published state: a logged out leader
    // keeps its last placement, and the follower must not chase it.
    partyStateFreshness = 20 * time.Second
)

// partyState is the group role of one loop: the shared hub, the account, the
// leader account and the pacing of the invitations and the follow walk.
type partyState struct {
    hub     *party.Hub
    account string
    leader  string
    invites map[string]time.Time
    // following holds the hysteresis of the follow walk and followAt paces
    // its click requests.
    following bool
    followAt  time.Time
    // focusTarget names the adopted leader target the focus log already
    // covered.
    focusTarget int32
}

// SetParty attaches the process wide party hub to the loop: the account names
// the group role and the hub shares the live state with the rest of the
// fleet. A nil hub or an ungrouped account leaves the party play off.
func (l *Loop) SetParty(hub *party.Hub, account string) {
    if hub == nil || account == "" {
        return
    }
    leader, members, ok := hub.GroupOf(account)
    if !ok {
        return
    }
    l.party = &partyState{
        hub:         hub,
        account:     account,
        leader:      leader,
        invites:     make(map[string]time.Time),
        following:   false,
        followAt:    time.Time{},
        focusTarget: 0,
    }
    role := "follower"
    if leader == account {
        role = "leader"
    }
    l.logger.Printf("Party: %s plays the %s of %s[%s]", account, role,
        leader, strings.Join(members, ","))
}

// partyTick publishes the live state of the character and runs the group role.
// It reports true when the follower walk owns the tick.
func (l *Loop) partyTick(now time.Time) bool {
    if l.party == nil {
        return false
    }
    x, y, z, ok := l.tracker.SelfPosition()
    if !ok {
        return false
    }
    l.party.hub.Publish(party.MemberState{
        Account:  l.party.account,
        ObjectID: l.tracker.SelfObjectID(),
        TargetID: l.target,
        X:        x,
        Y:        y,
        Z:        z,
        Alive:    !l.tracker.SelfDead(),
        At:       now,
    })
    if l.party.leader == l.party.account {
        l.partyInviteTick(now)

        return false
    }
    l.partyAnswerTick(now)
    if l.partyFollowTick(now) {
        return true
    }
    l.partyAdoptTarget(now)

    return false
}

// partyInviteTick invites the visible group members that are not in the party
// yet. The leader alone invites (the server requires it) and the per member
// pacing keeps a refused invitation from flooding the transaction window.
func (l *Loop) partyInviteTick(now time.Time) {
    _, members, ok := l.party.hub.GroupOf(l.party.account)
    if !ok || l.tracker.PartyCount() >= len(members) {
        return
    }
    selfX, selfY, _, ok := l.tracker.SelfPosition()
    if !ok {
        return
    }
    for _, account := range members {
        objectID, eligible := l.partyInviteCandidate(
            account, now, selfX, selfY)
        if !eligible {
            continue
        }
        l.party.invites[account] = now
        if err := l.game.JoinParty(objectID); err != nil {
            l.logf("Hunt: party invite failed: %v", err)

            continue
        }
        l.logf("Hunt: inviting %s into the party", account)
    }
}

// partyInviteCandidate resolves one group member to an invitable object id.
// The leader skips itself, a member already in the window, one out of range or
// sight, a stale placement and a member inside the retry window.
func (l *Loop) partyInviteCandidate(
    account string, now time.Time, selfX int32, selfY int32,
) (int32, bool) {
    if account == l.party.account {
        return 0, false
    }
    member, known := l.party.hub.Member(account)
    if !known || !member.Alive || member.ObjectID == 0 ||
        now.Sub(member.At) > partyStateFreshness {
        return 0, false
    }
    if _, joined := l.tracker.PartyMember(member.ObjectID); joined {
        return 0, false
    }
    memberX, memberY, _, visible :=
        l.tracker.ObjectPosition(member.ObjectID)
    if !visible {
        return 0, false
    }
    if math.Hypot(float64(memberX-selfX), float64(memberY-selfY)) >
        partyInviteRange {
        return 0, false
    }
    if last, invited := l.party.invites[account]; invited &&
        now.Sub(last) < partyInvitePeriod {
        return 0, false
    }

    return member.ObjectID, true
}

// partyAnswerTick answers the pending invitation: the configured leader is
// accepted, every other requestor is declined (an unanswered ask blocks the
// requestor's party machinery).
func (l *Loop) partyAnswerTick(now time.Time) {
    requestor, at := l.tracker.PendingPartyAsk()
    if requestor == 0 {
        return
    }
    var response int32 = togameserver.PartyAnswerAccept
    leader, known := l.party.hub.LeaderOf(l.party.account)
    trusted := known && leader.ObjectID == requestor &&
        now.Sub(leader.At) <= partyStateFreshness
    if !trusted && l.tracker.ObjectName(requestor) != l.party.leader {
        response = togameserver.PartyAnswerDecline
    }
    if now.Sub(at) > partyAskWindow {
        response = togameserver.PartyAnswerDecline
    }
    if err := l.game.AnswerJoinParty(response); err != nil {
        l.logf("Hunt: party answer failed: %v", err)

        return
    }
    l.tracker.ClearPartyAsk()
    if response == togameserver.PartyAnswerAccept {
        l.logf("Hunt: joined the group of %s", l.party.leader)

        return
    }
    l.logf("Hunt: declined the party invitation of %d", requestor)
}

// partyFollowTick walks a follower after a leader it fell behind. Near the
// leader an active fight or an incoming blow holds the walk; far away the
// group wins and the private target is dropped. It reports true while the
// follow owns the movement.
func (l *Loop) partyFollowTick(now time.Time) bool {
    // An errand owns the movement until it ends: a follower that abandons a
    // town trip to chase its leader never reaches the merchant and never
    // restocks the potions the trip started for.
    if l.tripActive() {
        return false
    }
    leader, known := l.party.hub.LeaderOf(l.party.account)
    if !known || !leader.Alive || leader.ObjectID == 0 ||
        now.Sub(leader.At) > partyStateFreshness {
        l.party.following = false

        return false
    }
    selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        return false
    }
    dist := math.Hypot(float64(leader.X-selfX), float64(leader.Y-selfY))
    if dist > partyConvergenceRange {
        // A follower hunting in another zone always finds a local target and
        // an incoming blow, so it would never rejoin the group. Past this
        // range cohesion wins: drop the private fight and walk to the leader.
        if l.target != 0 {
            l.target = 0
            l.clearBlindRecovery()
        }
    } else {
        if l.target != 0 && l.tracker.ObjectAlive(l.target) {
            return false
        }
        if l.tracker.SelfUnderAttack() {
            return false
        }
    }
    if l.party.following {
        if dist <= partyCatchUpDistance {
            l.party.following = false

            return false
        }
    } else {
        if dist <= partyFollowDistance {
            return false
        }
        l.party.following = true
    }
    if !l.party.followAt.IsZero() &&
        now.Sub(l.party.followAt) < walkRequestPeriod {
        return true
    }
    moveX, moveY, _ := walkClickSegment(
        selfX, selfY, selfZ,
        float64(leader.X), float64(leader.Y), float64(selfZ),
        math.Min(returnWalkSegment, dist-partyCatchUpDistance))
    if err := l.game.WalkTo(int32(moveX), int32(moveY), selfZ); err != nil {
        l.logf("Hunt: follow walk failed: %v", err)

        return true
    }
    l.party.followAt = now
    l.logf("Hunt: walking after %s, %.0f units behind", l.party.leader, dist)

    return true
}

// partyAdoptTarget adopts the leader target for the focus fire: the follower
// attacks the mob the group fights instead of picking its own.
func (l *Loop) partyAdoptTarget(now time.Time) {
    if l.target != 0 {
        return
    }
    leader, known := l.party.hub.LeaderOf(l.party.account)
    if !known || !leader.Alive || leader.TargetID == 0 ||
        now.Sub(leader.At) > partyStateFreshness {
        return
    }
    target := leader.TargetID
    if !l.tracker.ObjectAlive(target) ||
        !l.tracker.ObjectAttackable(target) {
        return
    }
    targetX, targetY, targetZ, ok := l.tracker.ObjectPosition(target)
    if !ok {
        return
    }
    selfX, selfY, _, ok := l.tracker.SelfPosition()
    if !ok {
        return
    }
    if math.Hypot(float64(targetX-selfX), float64(targetY-selfY)) >
        partyTargetRange {
        return
    }
    l.target = target
    l.noTargetSince = time.Time{}
    l.noPickLogAt = time.Time{}
    l.engageAt = now
    l.clearBlindRecovery()
    l.maybeBeginLure(now)
    l.approachPick(now, state.AttackTarget{
        ObjectID: target,
        Name:     l.tracker.ObjectName(target),
        X:        targetX,
        Y:        targetY,
        Z:        targetZ,
    })
    if l.party.focusTarget != target {
        l.party.focusTarget = target
        l.logf("Hunt: focusing the target %s of %s",
            l.tracker.ObjectName(target), l.party.leader)
    }
}
