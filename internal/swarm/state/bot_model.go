// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
    "time"
)

// Status of a bot session.
type Status string

const (
    // StatusConnecting means the bot is between login and world entry.
    StatusConnecting Status = "connecting"
    // StatusOnline means the character is inside the world.
    StatusOnline Status = "online"
    // StatusOffline means the session has ended.
    StatusOffline Status = "offline"
)

// eventCapacity is the size of the rolling event log per bot.
const eventCapacity = 512

// snapshotEvents limits how many events a snapshot carries.
const snapshotEvents = 100

// combatWindow is how long an object counts as fighting after the last
// attack or NpcInfo combat flag.
const combatWindow = 10 * time.Second

// defaultSelfCollision is the collision radius of the played character
// when no packet carries it (UserInfo has no collision field). The map
// uses it for the exact arrival projection of the own moves.
const defaultSelfCollision = 9.0

// underAttackWindow is how long a recent hit on the character keeps the
// rest logic from sitting down: sitting into the blows of a mob that is
// still swinging would only prolong the fight.
const underAttackWindow = 3 * time.Second

// combatEventTTL bounds how long an animation event stays in the
// snapshot feed: the SSE poll delivers a snapshot every 300 ms, so
// a two second window guarantees every event reaches the web view
// at least once while the sequence dedupe keeps the replays
// silent.
const combatEventTTL = 2 * time.Second

// combatEventMax bounds the animation feed length so a burst of
// swings and status updates cannot grow it without end.
const combatEventMax = 64

// CharacterState holds the observed state of the played character.
type CharacterState struct {
    Name            string
    Level           int32
    Race            int32
    ClassID         int32
    X               int32
    Y               int32
    Z               int32
    Heading         int32
    STR             int32
    DEX             int32
    CON             int32
    INT             int32
    WIT             int32
    MEN             int32
    Exp             int32
    Sp              int32
    CurHP           float64
    MaxHP           float64
    CurMP           float64
    MaxMP           float64
    Moving          bool
    DestX           int32
    DestY           int32
    DestZ           int32
    RunSpeed        float64
    WalkSpeed       float64
    CollisionRadius float64
    MoveAt          time.Time
    SocialUntil     time.Time
    AutoAttacking   bool
    CombatUntil     time.Time
    CombatActiveAt  time.Time
    // CannotSeeTargetAt records when the server last answered the
    // attack with "Cannot see target." - the geodata line of sight to
    // the target is blocked (an obstacle between the character and the
    // target), the hunt loop walks around it instead of re-requesting
    // the refused attack.
    CannotSeeTargetAt time.Time
    FightingTargetID  int32
    TargetID          int32
    Sitting           bool
    LastHitAt         time.Time
    // LastLandedHitAt and LastLandedHitTarget record the last blow of
    // the played character that actually landed (the miss flagged
    // swings of the Attack packet skip): the deleveling provocation
    // separates a guard that ignores landed damage (a stale AI state,
    // worth switching the guard) from a fight stage where not a
    // single blow landed yet (the level gap miss streak of the town
    // guards, worth keep provoking).
    LastLandedHitAt     time.Time
    LastLandedHitTarget int32
    // LastSelfShotAt stamps the broadcast of the played character's
    // own Attack packet - hit or miss. The Mobius server commits the
    // bow shot the moment it builds the packet: the hit roll, the
    // arrow consumption and the HitTask schedule all happen before
    // the broadcast (Creature.doAttack -> doAttackHitByBow), and the
    // damage task carries no attacker movement check, so the shot is
    // final once the packet arrived. For the bow fight this is the
    // release moment the kite retreat rides: everything after it is
    // the cooldown the character may spend walking away (issue #60).
    LastSelfShotAt time.Time
    // PAtkSpd carries the live physical attack speed of the played
    // character (the StatusUpdate ATK_SPD attribute 0x12 the server
    // rebroadcasts whenever the equipment or the buffs move it). The
    // bow fight reads it through SelfPAtkSpd for the cooldown window
    // math: the C1 disable window is timeAtk + reuse =
    // 500000/pAtkSpd + reuseDelay*333/pAtkSpd (Creature.
    // calculateTimeBetweenAttacks / calculateReuseTime), so the walk
    // window that spends the whole cooldown walking scales with the
    // weapon speed the server actually runs (issue #60, the third
    // round - the owner ask: the proper archering delays from the C1
    // Mobius formulas, not a fixed guess).
    PAtkSpd int32
    // BowCycleAt and BowCycle record the RED SetupGauge broadcast of
    // the played character (the packet carries no object id: it is
    // sent only to the affected client). The server sends
    // SetupGauge(RED, sAtk + reuse) the moment it commits a bow shot
    // (Creature.doAttackHitByBow), so the max field is the exact
    // disable window of the bow in milliseconds - the windup plus
    // the reuse tail the character must stand through before the
    // server accepts a move or a re-shot. The kite reads it instead
    // of the pAtkSpd formula because the ATK_SPD attribute is not
    // rebroadcast after a relogin until the gear changes (see
    // docs/kite_timing_findings.md and the 2026-11-09 acceptance
    // run: the formula windup fired the retreat click inside the
    // real windup and the server answered ActionFailed).
    BowCycleAt  time.Time
    BowCycle    time.Duration
    CurrentLoad int32
    MaxLoad     int32
}

// newCharacterState creates a zero valued character state.
func newCharacterState() CharacterState {
    return CharacterState{
        Name:                "",
        Level:               0,
        Race:                0,
        ClassID:             0,
        X:                   0,
        Y:                   0,
        Z:                   0,
        Heading:             0,
        STR:                 0,
        DEX:                 0,
        CON:                 0,
        INT:                 0,
        WIT:                 0,
        MEN:                 0,
        Exp:                 0,
        Sp:                  0,
        CurHP:               0,
        MaxHP:               0,
        CurMP:               0,
        MaxMP:               0,
        Moving:              false,
        DestX:               0,
        DestY:               0,
        DestZ:               0,
        RunSpeed:            defaultRunSpeed,
        WalkSpeed:           defaultWalkSpeed,
        CollisionRadius:     defaultSelfCollision,
        MoveAt:              time.Time{},
        SocialUntil:         time.Time{},
        AutoAttacking:       false,
        CombatUntil:         time.Time{},
        CombatActiveAt:      time.Time{},
        CannotSeeTargetAt:   time.Time{},
        FightingTargetID:    0,
        TargetID:            0,
        Sitting:             false,
        LastHitAt:           time.Time{},
        LastLandedHitAt:     time.Time{},
        LastLandedHitTarget: 0,
        LastSelfShotAt:      time.Time{},
        PAtkSpd:             0,
        BowCycleAt:          time.Time{},
        BowCycle:            0,
        CurrentLoad:         0,
        MaxLoad:             0,
    }
}

// inCombat reports whether the character fought within the combat window.
func (c CharacterState) inCombat(now time.Time) bool {
    return c.AutoAttacking || c.CombatUntil.After(now)
}

// walkingFreshWindow bounds how long the moving flag of the character
// still counts as an actually running walk without a fresh movement
// update: the server broadcasts the walking position continuously, so
// a silent stretch means the walk ended or stalled.
const walkingFreshWindow = 5 * time.Second

// fightingFreshWindow bounds how long the last attack or chase step of
// the character still counts as an actually running fight: swings and
// chase steps arrive at a sub second cadence while the auto attack
// runs, so a few seconds without either means the fight stopped even
// when the auto attack flag or the combat window still claim otherwise.
const fightingFreshWindow = 3 * time.Second

// fightingFresh reports whether the fight of the character is running
// right now: an attack or a chase step landed within the fresh window.
func (c CharacterState) fightingFresh(now time.Time) bool {
    return now.Sub(c.CombatActiveAt) <= fightingFreshWindow
}

// noteSelfCombatLocked records fresh fight activity of the played
// character (a swing or a chase step). The caller must hold the write
// lock.
func (b *Bot) noteSelfCombatLocked(now time.Time) {
    b.char.CombatActiveAt = now
    b.char.CombatUntil = now.Add(combatWindow)
}

// Event is a single entry of the rolling bot event log.
type Event struct {
    Time    time.Time `json:"time"`
    Message string    `json:"message"`
}

// NpcInfo carries the fields of a parsed NpcInfo packet.
type NpcInfo struct {
    ObjectID        int32
    TemplateID      int32
    Attackable      bool
    X               int32
    Y               int32
    Z               int32
    Heading         int32
    RunSpeed        int32
    WalkSpeed       int32
    MoveSpeedMult   float64
    CollisionRadius float64
    // RightHandItem is the template rhand item id of the npc (the
    // NpcInfo equipment block): the hunt resolves the enemy weapon
    // family from it (the anti-archer gate of the kite layer).
    RightHandItem int32
    Running       bool
    InCombat      bool
    Dead          bool
    Name          string
    Title         string
}

// PlayerInfo carries the fields of a parsed CharInfo packet.
type PlayerInfo struct {
    ObjectID        int32
    Name            string
    Title           string
    Race            int32
    ClassID         int32
    RunSpeed        int32
    WalkSpeed       int32
    MoveSpeedMult   float64
    CollisionRadius float64
    Running         bool
    InCombat        bool
    Dead            bool
    // Sitting carries the wait state of the observed player: false
    // while it stands, true while it sits (the rest icon of the map).
    Sitting bool
    X       int32
    Y       int32
    Z       int32
}

// ItemInfo carries the fields of a parsed DropItem packet.
type ItemInfo struct {
    ObjectID   int32
    TemplateID int32
    Stackable  bool
    Count      int32
    X          int32
    Y          int32
    Z          int32
}

// Movement describes a MoveToLocation packet of one object.
type Movement struct {
    ObjectID int32
    X        int32
    Y        int32
    Z        int32
    DestX    int32
    DestY    int32
    DestZ    int32
}

// PawnMovement describes a MoveToPawn packet of a chasing object.
type PawnMovement struct {
    ObjectID int32
    TargetID int32
    Distance int32
    X        int32
    Y        int32
    Z        int32
    TargetX  int32
    TargetY  int32
    TargetZ  int32
}

// Placement describes a ValidateLocation or StopMove packet.
type Placement struct {
    ObjectID int32
    X        int32
    Y        int32
    Z        int32
    Heading  int32
    Moving   bool
}

// AttackTargets is the capacity of the Attack target list.
const AttackTargets = 4

// attackHitMissFlag mirrors the Mobius Hit flag of a missed swing
// (Hit.java HITFLAG_MISS, bit pattern 0x80): the Attack packet
// carries it per hit, so the tracker can tell a landed blow from an
// evaded one. The packet reader hands the byte over as a signed
// int8, so the flag reads as a negative value.
const attackHitMissFlag int8 = -128

// attackHitCritFlag mirrors the Mobius Hit flag of a critical blow
// (Hit.java HITFLAG_CRIT, bit pattern 0x20): the Attack packet
// carries it per hit, so the tracker can label the damage float of
// that blow as a critical one.
const attackHitCritFlag int8 = 0x20

// CritFromHitFlag reports the Mobius HITFLAG_CRIT verdict of one raw
// hit flags byte (the connection layer reads the parsed hits of the
// Attack packet and copies the verdict into state.Attack.CritFlags).
func CritFromHitFlag(flag int8) bool {
    return flag&attackHitCritFlag != 0
}

// Attack describes an Attack packet of one attacker.
type Attack struct {
    AttackerID int32
    X          int32
    Y          int32
    Z          int32
    TargetX    int32
    TargetY    int32
    TargetZ    int32
    TargetIDs  [AttackTargets]int32
    // HitFlags carries the per hit flags (see attackHitMissFlag and
    // attackHitCritFlag) of the TargetIDs entries.
    HitFlags [AttackTargets]int8
    // CritFlags carries the per hit critical verdicts derived from
    // HitFlags (the web view labels the damage floats with them).
    CritFlags   [AttackTargets]bool
    TargetCount int
}

// Combat event kinds of the web view animation feed.
const (
    // CombatEventAttack is a swing: the attacker placement runs in
    // X and Y, the hit target placement in TargetX and TargetY.
    CombatEventAttack = "attack"
    // CombatEventDamage is a hit landing: TargetID is the hurt
    // unit, Amount the HP it lost and X/Y its placement at the
    // moment.
    CombatEventDamage = "damage"
    // CombatEventMiss is an evaded blow: the swing was thrown at
    // TargetID but the server resolved it as a miss (the Hit.java
    // MISS flag). The viewer draws the miss float instead of a
    // damage number, so a fight stays legible when blows start
    // missing.
    CombatEventMiss = "miss"
)

// CombatEvent is one observed beat of the combat animation feed
// of the web view: an attack swing of the Attack broadcast, a
// damage landing of a StatusUpdate HP drop (Crit marks the blows
// the server resolved as critical) or a miss float of an evaded
// blow. The monotonic sequence lets the client replay every event
// exactly once across the repeated snapshots of the event stream.
type CombatEvent struct {
    Seq        uint64
    Kind       string
    AttackerID int32
    TargetID   int32
    Amount     float64
    Crit       bool
    At         time.Time
    X          int32
    Y          int32
    TargetX    int32
    TargetY    int32
}

// Attribute is one id/value pair of a StatusUpdate packet.
type Attribute struct {
    ID    int32
    Value int32
}
