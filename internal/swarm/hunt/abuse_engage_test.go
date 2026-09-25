package hunt

import (
    "testing"
    "time"

    "github.com/stretchr/testify/require"

    "github.com/melg8/swarm/internal/swarm/state"
)

// spawnMobFar adds an attackable npc a thousand units out with a
// known ground height: the abuse engage claims of the -abuse mode
// close exactly that stretch (see abuseEngageClaim).
func spawnMobFar(bot *state.Bot) {
    bot.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 7, TemplateID: 1000001, Attackable: true,
        X: 46000, Y: 50000, Z: -3500, Name: "Gremlin",
    })
}

// claimEcho lands the adopted placement of an engage claim in the
// tracker: the cursor key branch of ValidatePosition.runImpl
// broadcasts the claimed placement back, the tracker learns the new
// position the same tick.
func claimEcho(bot *state.Bot) {
    bot.ApplyPlacement(state.Placement{
        ObjectID: 100, X: 45925, Y: 50000, Z: -3500,
    })
}

// TestAbuseEngageClaimTeleportsBeforeTheAttack pins the core
// contract of the -abuse engage: a picked target beyond the engage
// radius is reached through the claim channel BEFORE the attack
// request - the request arms the server side chase and the chase is
// the direct run-up the mode replaces. The claim lands at the engage
// point inside the weapon radius, the forced attack of the next tick
// starts the fight in range.
func TestAbuseEngageClaimTeleportsBeforeTheAttack(t *testing.T) {
    bot := newTestBot()
    spawnMobFar(bot)
    game := &fakeGame{abuse: true}
    loop := NewLoop(game, bot)
    loop.lastHit = time.Now().Add(-time.Minute)

    loop.tick()
    // The approach is one claim at the engage point (half the melee
    // radius of 150 on the line to the character), not the selecting
    // attack request that would start the server chase.
    require.Empty(t, game.forces,
        "no attack request may start the server side chase")
    require.Len(t, game.walks, 1)
    require.Equal(t, [3]int32{45925, 50000, -3500}, game.walks[0])

    // The claim echo lands the character next to the mob: the attack
    // request now starts the fight in range, no further claim fires.
    claimEcho(bot)
    loop.lastHit = time.Now().Add(-time.Minute)
    loop.tick()
    require.Equal(t, []int32{7}, game.forces,
        "the forced attack starts the fight once in range")
    require.Len(t, game.walks, 1,
        "a target inside the engage radius claims nothing")
}

// TestAbuseEngageClaimOverridesTheRunningChase pins the exact leg
// the owner reported: an armed chase toward a far target - the
// attack stance holds and the server AI runs the character at run
// speed while the chase stays healthy - must ride the claims too.
// The ordinary stall watchdog waits a healthy chase out by design,
// so the claim has to fire past it.
func TestAbuseEngageClaimOverridesTheRunningChase(t *testing.T) {
    bot := newTestBot()
    spawnMobFar(bot)
    game := &fakeGame{abuse: true}
    loop := NewLoop(game, bot)
    loop.target = 7
    loop.lastHit = time.Now().Add(-time.Minute)
    // The forced attack armed the stance and the server chase walks
    // the character toward the mob: a healthy chase, no stall.
    bot.ApplySelfTarget(7)
    bot.ApplyPawnMovement(state.PawnMovement{
        ObjectID: 100, TargetID: 7, Distance: 40,
        X: 45000, Y: 50000, TargetX: 45960, TargetY: 50000, TargetZ: -3500,
    })

    loop.tick()
    require.Len(t, game.walks, 1,
        "the claim must replace the healthy chase, not wait it out")
    require.Empty(t, game.forces,
        "the running fight re-requests nothing")
    require.Equal(t, [3]int32{45925, 50000, -3500}, game.walks[0])
}

// TestEngageApproachStaysDirectWithoutTheFlag pins the ordinary
// behavior the mode must not touch: without -abuse the first request
// selects the far target (the server chase owns the approach) and a
// healthy chase runs uninterrupted - no claim, no fallback walk.
func TestEngageApproachStaysDirectWithoutTheFlag(t *testing.T) {
    bot := newTestBot()
    spawnMobFar(bot)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.lastHit = time.Now().Add(-time.Minute)

    loop.tick()
    require.Equal(t, []int32{7}, game.forces,
        "the plain mode selects the target directly")
    require.Empty(t, game.walks)

    // The chase runs healthy: the loop keeps waiting it out.
    bot.ApplySelfTarget(7)
    bot.ApplyPawnMovement(state.PawnMovement{
        ObjectID: 100, TargetID: 7, Distance: 40,
        X: 45000, Y: 50000, TargetX: 45960, TargetY: 50000, TargetZ: -3500,
    })
    for range 3 {
        loop.lastHit = time.Now().Add(-time.Minute)
        loop.tick()
    }
    require.Empty(t, game.walks,
        "a healthy chase never triggers the fallback walk")
}

// TestAbuseEngageClaimPacesItsRepeats pins the repeat pacing: the
// echo of a claim needs a tick to land in the tracker, so the ticks
// inside the engage retry period own the approach without stacking
// another claim, and a target that still keeps its distance past the
// period gets the next claim.
func TestAbuseEngageClaimPacesItsRepeats(t *testing.T) {
    bot := newTestBot()
    spawnMobFar(bot)
    game := &fakeGame{abuse: true}
    loop := NewLoop(game, bot)
    loop.lastHit = time.Now().Add(-time.Minute)

    loop.tick()
    require.Len(t, game.walks, 1)

    // The echo never landed (the tracker still holds the old
    // position): the immediate tick must not stack a second claim.
    loop.lastHit = time.Now().Add(-time.Minute)
    loop.tick()
    require.Len(t, game.walks, 1,
        "the paced out tick owns the approach without a new claim")

    // The retry period passed and the target still stands far: the
    // next claim fires.
    loop.abuseEngageAt = time.Now().Add(-2 * engageRetryPeriod)
    loop.lastHit = time.Now().Add(-time.Minute)
    loop.tick()
    require.Len(t, game.walks, 2)
}

// TestUserAttackRidesTheAbuseEngageClaim pins the manual attack
// flow: a clicked far target is reached through the claim channel
// before any request - the selecting request arms the very chase the
// mode replaces - and the fight starts in range once the claim
// lands.
func TestUserAttackRidesTheAbuseEngageClaim(t *testing.T) {
    bot := newTestBot()
    spawnMobFar(bot)
    game := &fakeGame{abuse: true}
    loop := NewLoop(game, bot)
    loop.lastHit = time.Now().Add(-time.Minute)

    pushCommand(bot, state.Command{Kind: state.CommandAttack, ObjectID: 7})
    loop.tick()
    require.Empty(t, game.forces,
        "no selecting request may start the server side chase")
    require.Len(t, game.walks, 1)
    require.Equal(t, [3]int32{45925, 50000, -3500}, game.walks[0])

    // The claim lands: the selection starts the fight from inside
    // the engage radius.
    claimEcho(bot)
    loop.tick()
    require.Equal(t, []int32{7}, game.forces)
}

// TestUserAttackClaimOverridesTheRunningChase pins the manual twin
// of the running chase leg: the clicked target is selected, the
// forced attack armed the stance and the server chase walks the
// character - the claim still owns the approach.
func TestUserAttackClaimOverridesTheRunningChase(t *testing.T) {
    bot := newTestBot()
    spawnMobFar(bot)
    game := &fakeGame{abuse: true}
    loop := NewLoop(game, bot)
    loop.lastHit = time.Now().Add(-time.Minute)
    bot.ApplySelfTarget(7)
    bot.ApplyPawnMovement(state.PawnMovement{
        ObjectID: 100, TargetID: 7, Distance: 40,
        X: 45000, Y: 50000, TargetX: 45960, TargetY: 50000, TargetZ: -3500,
    })

    pushCommand(bot, state.Command{Kind: state.CommandAttack, ObjectID: 7})
    loop.tick()
    require.Len(t, game.walks, 1,
        "the claim must override the running chase, not wait it out")
    require.Empty(t, game.forces,
        "no attack request fires from beyond the engage radius")
}

// TestUserAttackStaysDirectWithoutTheFlag pins the ordinary manual
// flow the mode must not touch: the first request selects the far
// target and the healthy chase runs on.
func TestUserAttackStaysDirectWithoutTheFlag(t *testing.T) {
    bot := newTestBot()
    spawnMobFar(bot)
    game := &fakeGame{}
    loop := NewLoop(game, bot)
    loop.lastHit = time.Now().Add(-time.Minute)

    pushCommand(bot, state.Command{Kind: state.CommandAttack, ObjectID: 7})
    loop.tick()
    require.Equal(t, []int32{7}, game.forces)
    require.Empty(t, game.walks)
}

// TestAbuseEngagePointGeometry pins the engage point math: half the
// engage radius on the line from the target toward the character,
// the degenerate standing point included.
func TestAbuseEngagePointGeometry(t *testing.T) {
    // The melee radius of the plain hunt: 150 units.
    x, y := abuseEngagePoint(46000, 50000, 45000, 50000, 150)
    require.Equal(t, int32(45925), x)
    require.Equal(t, int32(50000), y)

    // The bow radius: 450 units.
    x, y = abuseEngagePoint(46000, 50000, 45000, 50000, 450)
    require.Equal(t, int32(45775), x)
    require.Equal(t, int32(50000), y)

    // A diagonal approach keeps the line.
    x, y = abuseEngagePoint(46000, 51000, 44000, 49000, 150)
    require.InDelta(t, 45946.97, float64(x), 0.6)
    require.InDelta(t, 50946.97, float64(y), 0.6)

    // The degenerate line: the character stands on the target.
    x, y = abuseEngagePoint(45000, 50000, 45000, 50000, 150)
    require.Equal(t, int32(45075), x)
    require.Equal(t, int32(50000), y)

    // The defensive clamp: a character barely inside the radius
    // claims a unit short of its standing point.
    x, y = abuseEngagePoint(45200, 50000, 45000, 50000, 450)
    require.Equal(t, int32(45001), x)
    require.Equal(t, int32(50000), y)
}
