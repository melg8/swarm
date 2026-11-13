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

// defaultSelfCollision is the collision radius when no packet carries it
// (UserInfo has no collision field); the map projects the own moves with it.
const defaultSelfCollision = 9.0

// underAttackWindow keeps the rest logic from sitting down into the blows of a
// mob that is still swinging.
const underAttackWindow = 3 * time.Second

// combatEventTTL bounds how long an animation event stays in the snapshot feed:
// the SSE poll delivers one every 300 ms, so two seconds reach the web view at
// least once while the sequence dedupe keeps the replays silent.
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
    // CannotSeeTargetAt is the last "Cannot see target." answer: the geodata
    // line of sight is blocked, so the hunt walks around instead of
    // re-requesting the attack.
    CannotSeeTargetAt time.Time
    FightingTargetID  int32
    TargetID          int32
    Sitting           bool
    LastHitAt         time.Time
    // LastLandedHitAt and LastLandedHitTarget record the last blow that landed
    // (miss flagged swings skip): the deleveling provocation tells a guard that
    // ignores landed damage from the level gap miss streak worth provoking.
    LastLandedHitAt     time.Time
    LastLandedHitTarget int32
    // LastSelfShotAt stamps the broadcast of the own Attack packet, hit or
    // miss. The Mobius server commits a bow shot before the broadcast
    // (Creature.doAttackHitByBow) and the damage task checks no movement, so
    // the retreat after it spends the cooldown, nothing else.
    LastSelfShotAt time.Time
    // PAtkSpd is the live attack speed (the StatusUpdate ATK_SPD attribute
    // 0x12, rebroadcast when gear or buffs move it). The bow cooldown reads
    // 500000/pAtkSpd + reuseDelay*333/pAtkSpd (the C1
    // calculateTimeBetweenAttacks and calculateReuseTime).
    PAtkSpd int32
    // BowCycleAt and BowCycle record the RED SetupGauge of the committed bow
    // shot (the packet names no object, it goes to the affected client only):
    // the max field is the exact disable window, windup plus reuse tail. The
    // kite reads it over the pAtkSpd formula, because ATK_SPD is not
    // rebroadcast after a relogin until the gear changes (see
    // docs/kite_timing_findings.md).
    BowCycleAt  time.Time
    BowCycle    time.Duration
    CurrentLoad int32
    MaxLoad     int32
}

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

// walkingFreshWindow bounds how long the moving flag still counts without a
// fresh movement update: the server broadcasts the walking position
// continuously, so a silent stretch means the walk ended or stalled.
const walkingFreshWindow = 5 * time.Second

// fightingFreshWindow bounds how long the last attack or chase step still
// counts as a running fight: both arrive at a sub second cadence, so a few
// silent seconds mean the fight stopped.
const fightingFreshWindow = 3 * time.Second

// fightingFresh reports whether the fight of the character is running
// right now: an attack or a chase step landed within the fresh window.
func (c CharacterState) fightingFresh(now time.Time) bool {
    return now.Sub(c.CombatActiveAt) <= fightingFreshWindow
}

// noteSelfCombatLocked records fresh fight activity of the played character (a
// swing or a chase step). The caller must hold the write lock.
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
    // RightHandItem is the template rhand item id: the hunt resolves the
    // enemy weapon family from it (the anti-archer gate of the kite).
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
    // Sitting is the wait state of the observed player (the map rest icon).
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

// attackHitMissFlag mirrors the Mobius HITFLAG_MISS (0x80) per hit flag, so a
// landed blow reads apart from an evaded one. The reader hands the byte over
// as a signed int8, so the flag reads negative.
const attackHitMissFlag int8 = -128

// attackHitCritFlag mirrors the Mobius HITFLAG_CRIT (0x20) per hit flag, so
// the tracker labels the damage float of that blow as critical.
const attackHitCritFlag int8 = 0x20

// CritFromHitFlag reports the Mobius HITFLAG_CRIT verdict of one hit flags byte
// (the connection layer copies it into state.Attack.CritFlags).
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
    // HitFlags carries the per hit flags of the TargetIDs entries.
    HitFlags [AttackTargets]int8
    // CritFlags carries the critical verdicts derived from HitFlags.
    CritFlags   [AttackTargets]bool
    TargetCount int
}

// Combat event kinds of the web view animation feed.
const (
    // CombatEventAttack is a swing: the attacker placement in X and Y, the
    // target placement in TargetX and TargetY.
    CombatEventAttack = "attack"
    // CombatEventDamage is a hit landing: TargetID is the hurt unit, Amount
    // the HP it lost, X and Y its placement.
    CombatEventDamage = "damage"
    // CombatEventMiss is an evaded blow (the Hit.java MISS flag): the viewer
    // draws a miss float instead of a damage number.
    CombatEventMiss = "miss"
)

// CombatEvent is one beat of the animation feed: a swing, a damage landing of
// a StatusUpdate HP drop (Crit marks a critical) or a miss. The monotonic
// sequence lets the client replay every event exactly once.
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
