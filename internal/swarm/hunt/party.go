// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

// The group play of the fleet: the leader invites the visible members, then
// publishes one command every tick - the shared target, the formation anchor,
// the first attacker and the rest decision. The followers walk into their
// formation slot, attack only the shared target and rest as one. The server
// party is the real C1 party (RequestJoinParty / RequestAnswerJoinParty); the
// process wide hub only carries the state the game client of a member cannot
// see (docs/hunting.md).

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
    // partySlotSpacing is the offset of a follower from the leader anchor and
    // partySlotRange the distance that ends the formation walk. The offsets
    // keep the members close without stacking them on one point.
    partySlotSpacing = 150.0
    partySlotRange   = 220.0
    // partyTargetRange bounds the shared target: a follower adopts the leader
    // target only while it sits inside this distance. Past
    // partyConvergenceRange a follower drops its own fight to rejoin the group.
    partyTargetRange      = 1200.0
    partyConvergenceRange = 2400.0
    // partyInvitePeriod paces the invitation retries per member: the server
    // refuses a second ask while the first one is pending.
    partyInvitePeriod = 6 * time.Second
    // partyAskWindow bounds the age of an unanswered invitation: an ask older
    // than this is dropped instead of answered late.
    partyAskWindow = 15 * time.Second
    // partyStateFreshness bounds the published state and the group command:
    // a logged out member keeps its last placement, and nobody chases it.
    partyStateFreshness = 20 * time.Second
    // partyTankSwapHealth is the health line the first attacker gives up: a
    // healthier member takes the lead when it beats the current one by
    // partyTankSwapMargin, and only after partyRotationCooldown.
    partyTankSwapHealth   = 65.0
    partyTankSwapMargin   = 20.0
    partyRotationCooldown = 12 * time.Second
    // partyStandDown holds the demoted first attacker out of the attack: the
    // new first has to pull the mob before the old one swings again.
    partyStandDown = 4 * time.Second
)

// partyState is the group role of one loop: the shared hub, the account, the
// leader account, the invitation pacing, the walk pacing and the attack hold
// of a demoted first attacker.
type partyState struct {
    hub     *party.Hub
    account string
    leader  string
    invites map[string]time.Time
    // followAt paces the formation click requests.
    followAt time.Time
    // holdUntil keeps a demoted first attacker out of the attack window.
    holdUntil time.Time
    // focusTarget names the shared target the focus log already covered.
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
// The leader computes the command, the follower obeys it. It reports true when
// the formation walk or the shared rest owns the tick.
func (l *Loop) partyTick(now time.Time) bool {
    if l.party == nil {
        return false
    }
    x, y, z, ok := l.tracker.SelfPosition()
    if !ok {
        return false
    }
    hp := l.tracker.SelfHealthPercent()
    mp := l.tracker.SelfManaPercent()
    l.party.hub.Publish(party.MemberState{
        Account:   l.party.account,
        ObjectID:  l.tracker.SelfObjectID(),
        TargetID:  l.target,
        X:         x,
        Y:         y,
        Z:         z,
        HPPercent: hp,
        MPPercent: mp,
        Alive:     !l.tracker.SelfDead(),
        Caster:    l.isCaster(),
        RestWanted: hp < reengageHealthPercent ||
            l.isCaster() && mp < manaSitPercent,
        At: now,
    })
    if l.party.leader == l.party.account {
        l.partyInviteTick(now)
        command := l.partyLeadTick(now, x, y, z)
        l.partyRoleAttack(command, now)

        return false
    }
    l.partyAnswerTick(now)
    if l.phase == phaseLoot || l.tripActive() {
        // Looting and the merchant errand own their phases: the group waits
        // for the pickups and the potion restock before it marches again.
        return false
    }
    command, ok := l.party.hub.CoordinationOf(l.party.account)
    if !ok || now.Sub(command.At) > partyStateFreshness {
        return false
    }
    l.partyRoleAttack(command, now)

    return l.partyFollowTick(now, command)
}

// partyRoleAttack applies the first attacker role of the group command: the
// promoted member attacks at once, the demoted one stands down so the mob
// follows the new first instead of the old one.
func (l *Loop) partyRoleAttack(command party.Coordination, now time.Time) {
    if command.First == l.party.account {
        l.party.holdUntil = time.Time{}

        return
    }
    until := command.FirstAt.Add(partyStandDown)
    if now.Before(until) && (l.party.holdUntil.IsZero() ||
        until.After(l.party.holdUntil)) {
        l.party.holdUntil = until
        l.logf("Hunt: %s takes the lead of the group", command.First)
    }
}

// partyAttackHeld reports whether the group holds this member out of the
// attack: the demoted first attacker waits for the new one to pull the mob.
func (l *Loop) partyAttackHeld(now time.Time) bool {
    return l.party != nil && now.Before(l.party.holdUntil)
}

// partyLeadTick builds and publishes the group command from the fresh member
// states: the first attacker rotation, the shared rest and the leader target.
func (l *Loop) partyLeadTick(
    now time.Time, x int32, y int32, z int32,
) party.Coordination {
    _, members, ok := l.party.hub.GroupOf(l.party.account)
    if !ok {
        return party.Coordination{}
    }
    states := make([]party.MemberState, 0, len(members))
    for _, account := range members {
        member, known := l.party.hub.Member(account)
        if known && now.Sub(member.At) <= partyStateFreshness {
            states = append(states, member)
        }
    }
    previous, _ := l.party.hub.CoordinationOf(l.party.account)
    first, firstAt := l.partyChooseFirst(states, previous, now)
    command := party.Coordination{
        Leader:   l.party.account,
        First:    first,
        FirstAt:  firstAt,
        Rest:     l.partyWantRest(states, previous),
        TargetID: l.target,
        X:        x,
        Y:        y,
        Z:        z,
        At:       now,
    }
    l.party.hub.PublishCoordination(l.party.account, command)

    return command
}

// partyChooseFirst rotates the first attacker when its health fell below the
// swap line and a healthier member is ready. Casters never take the lead of a
// warrior group. A dead or stale first is replaced without the cooldown.
func (l *Loop) partyChooseFirst(
    states []party.MemberState, previous party.Coordination, now time.Time,
) (string, time.Time) {
    first := previous.First
    firstAt := previous.FirstAt
    fresh := first == ""
    if fresh {
        first = l.party.leader
        firstAt = now
    }
    current, known := partyStateOf(states, first)
    if known && current.HPPercent >= partyTankSwapHealth {
        return first, firstAt
    }
    best, found := partyBestFirst(states)
    if !found {
        return first, firstAt
    }
    if known && best.HPPercent < current.HPPercent+partyTankSwapMargin {
        return first, firstAt
    }
    if known && current.Alive && !fresh &&
        now.Sub(firstAt) < partyRotationCooldown {
        return first, firstAt
    }
    if best.Account == first {
        return first, firstAt
    }
    l.logf("Hunt: party first attacker is now %s at %.0f%%",
        best.Account, best.HPPercent)

    return best.Account, now
}

// partyBestFirst picks the healthiest living warrior of the fresh states.
// Casters never take the lead of a warrior group.
func partyBestFirst(states []party.MemberState) (party.MemberState, bool) {
    var best party.MemberState
    found := false
    for _, member := range states {
        if !member.Alive || member.Caster || member.ObjectID == 0 {
            continue
        }
        if !found || member.HPPercent > best.HPPercent {
            best, found = member, true
        }
    }

    return best, found
}

// partyStateOf finds the fresh state of one account.
func partyStateOf(
    states []party.MemberState, account string,
) (party.MemberState, bool) {
    for _, state := range states {
        if state.Account == account {
            return state, true
        }
    }

    return party.MemberState{}, false
}

// partyWantRest starts the shared rest when any member needs recovery and ends
// it only when every living member stands above the recovery lines. The
// hysteresis keeps a group from toggling between the fight and the sit.
func (l *Loop) partyWantRest(
    states []party.MemberState, previous party.Coordination,
) bool {
    rest := previous.Rest
    wanted := false
    recovered := len(states) > 0
    for _, member := range states {
        if !member.Alive {
            continue
        }
        if member.RestWanted {
            wanted = true
        }
        if member.HPPercent < standUpHealthPercent ||
            member.Caster && member.MPPercent < manaStandPercent {
            recovered = false
        }
    }
    if wanted {
        rest = true
    }
    if rest && recovered {
        rest = false
    }

    return rest
}

// partyFollowTick keeps a follower in its formation slot. A member outside the
// slot walks back (dropping a private fight), a resting member gathers and
// sits with the group, and a member in formation waits for the shared target
// instead of picking one of its own.
func (l *Loop) partyFollowTick(
    now time.Time, command party.Coordination,
) bool {
    selfX, selfY, selfZ, ok := l.tracker.SelfPosition()
    if !ok {
        return false
    }
    destX, destY := l.partySlot(command)
    dist := math.Hypot(destX-float64(selfX), destY-float64(selfY))
    if command.Rest {
        l.partyDropTarget()

        return l.partyRest(now, selfX, selfY, selfZ, destX, destY, dist)
    }
    if dist > partySlotRange {
        l.partyDropTarget()

        return l.partyWalkTo(now, selfX, selfY, selfZ, destX, destY, dist)
    }
    if l.partyAdoptTarget(now, command) {
        return false
    }
    if l.partyHoldPick(command) {
        return true
    }
    if l.tracker.SelfSitting() &&
        l.tracker.SelfHealthPercent() >= standUpHealthPercent {
        l.standUpGuarded(now)

        return true
    }

    return false
}

// partySlot returns the formation point of this member: the leader anchor plus
// a small per member offset around it.
func (l *Loop) partySlot(command party.Coordination) (float64, float64) {
    slot, ok := l.party.hub.SlotOf(l.party.account)
    if !ok || slot == 0 {
        return float64(command.X), float64(command.Y)
    }
    angle := float64(slot-1) * 2 * math.Pi / 6

    return float64(command.X) + math.Cos(angle)*partySlotSpacing,
        float64(command.Y) + math.Sin(angle)*partySlotSpacing
}

// partyWalkTo issues one paced ground click toward the formation point and
// reports that the walk owns the tick.
func (l *Loop) partyWalkTo(
    now time.Time, selfX int32, selfY int32, selfZ int32,
    destX float64, destY float64, dist float64,
) bool {
    if !l.party.followAt.IsZero() &&
        now.Sub(l.party.followAt) < walkRequestPeriod {
        return true
    }
    moveX, moveY, _ := walkClickSegment(
        selfX, selfY, selfZ, destX, destY, float64(selfZ),
        math.Min(returnWalkSegment, dist))
    if err := l.game.WalkTo(int32(moveX), int32(moveY), selfZ); err != nil {
        l.logf("Hunt: party formation walk failed: %v", err)

        return true
    }
    l.party.followAt = now
    l.logf("Hunt: party formation walk, %.0f units from the slot", dist)

    return true
}

// partyRest gathers a resting member and sits it down when it reached its
// slot. The shared rest ends with the leader command, never with one member
// that recovered early.
func (l *Loop) partyRest(
    now time.Time, selfX int32, selfY int32, selfZ int32,
    destX float64, destY float64, dist float64,
) bool {
    if dist > partySlotRange {
        return l.partyWalkTo(now, selfX, selfY, selfZ, destX, destY, dist)
    }
    if l.tracker.SelfUnderAttack() {
        // A mob reached the resting spot: stand and let the defensive
        // decision answer it, the group sits again once the blow stops.
        l.standUpGuarded(now)

        return true
    }
    l.rest()

    return true
}

// partyDropTarget clears a private fight when the group moves or rests.
func (l *Loop) partyDropTarget() {
    if l.target == 0 {
        return
    }
    l.target = 0
    l.clearBlindRecovery()
}

// partyHoldPick keeps a member in formation from picking a private fight: only
// the shared target is worth attacking. An attacker that reached the member is
// still answered by the defensive gate in engage.
func (l *Loop) partyHoldPick(command party.Coordination) bool {
    if l.phase != phaseEngage || l.tracker.SelfUnderAttack() {
        return false
    }
    if command.TargetID == 0 || !l.tracker.ObjectAlive(command.TargetID) ||
        !l.tracker.ObjectAttackable(command.TargetID) {
        return true
    }
    selfX, selfY, _, ok := l.tracker.SelfPosition()
    if !ok {
        return false
    }
    targetX, targetY, _, known := l.tracker.ObjectPosition(command.TargetID)
    if !known {
        return true
    }

    return math.Hypot(float64(targetX-selfX), float64(targetY-selfY)) >
        partyTargetRange
}

// partyAdoptTarget adopts the shared target for the simultaneous attack. It
// reports true when a fresh target was selected this tick.
func (l *Loop) partyAdoptTarget(
    now time.Time, command party.Coordination,
) bool {
    if command.TargetID == 0 || l.target == command.TargetID {
        return false
    }
    target := command.TargetID
    if !l.tracker.ObjectAlive(target) ||
        !l.tracker.ObjectAttackable(target) {
        return false
    }
    targetX, targetY, targetZ, ok := l.tracker.ObjectPosition(target)
    if !ok {
        return false
    }
    selfX, selfY, _, ok := l.tracker.SelfPosition()
    if !ok {
        return false
    }
    if math.Hypot(float64(targetX-selfX), float64(targetY-selfY)) >
        partyTargetRange {
        return false
    }
    l.partyDropTarget()
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

    return true
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
