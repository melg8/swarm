// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/melg8/swarm/internal/swarm/npcdata"
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

// CharacterState holds the observed state of the played character.
type CharacterState struct {
	Name             string
	Level            int32
	Race             int32
	ClassID          int32
	X                int32
	Y                int32
	Z                int32
	Heading          int32
	STR              int32
	DEX              int32
	CON              int32
	INT              int32
	WIT              int32
	MEN              int32
	Exp              int32
	Sp               int32
	CurHP            float64
	MaxHP            float64
	CurMP            float64
	MaxMP            float64
	Moving           bool
	DestX            int32
	DestY            int32
	DestZ            int32
	RunSpeed         float64
	WalkSpeed        float64
	CollisionRadius  float64
	MoveAt           time.Time
	AutoAttacking    bool
	CombatUntil      time.Time
	FightingTargetID int32
	TargetID         int32
	Sitting          bool
	LastHitAt        time.Time
	CurrentLoad      int32
	MaxLoad          int32
}

// newCharacterState creates a zero valued character state.
func newCharacterState() CharacterState {
	return CharacterState{
		Name:             "",
		Level:            0,
		Race:             0,
		ClassID:          0,
		X:                0,
		Y:                0,
		Z:                0,
		Heading:          0,
		STR:              0,
		DEX:              0,
		CON:              0,
		INT:              0,
		WIT:              0,
		MEN:              0,
		Exp:              0,
		Sp:               0,
		CurHP:            0,
		MaxHP:            0,
		CurMP:            0,
		MaxMP:            0,
		Moving:           false,
		DestX:            0,
		DestY:            0,
		DestZ:            0,
		RunSpeed:         defaultRunSpeed,
		WalkSpeed:        defaultWalkSpeed,
		CollisionRadius:  defaultSelfCollision,
		MoveAt:           time.Time{},
		AutoAttacking:    false,
		CombatUntil:      time.Time{},
		FightingTargetID: 0,
		TargetID:         0,
		Sitting:          false,
		LastHitAt:        time.Time{},
		CurrentLoad:      0,
		MaxLoad:          0,
	}
}

// inCombat reports whether the character fought within the combat window.
func (c CharacterState) inCombat(now time.Time) bool {
	return c.AutoAttacking || c.CombatUntil.After(now)
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
	Running         bool
	InCombat        bool
	Dead            bool
	Name            string
	Title           string
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
	X               int32
	Y               int32
	Z               int32
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

// Attack describes an Attack packet of one attacker.
type Attack struct {
	AttackerID  int32
	X           int32
	Y           int32
	Z           int32
	TargetX     int32
	TargetY     int32
	TargetZ     int32
	TargetIDs   [AttackTargets]int32
	TargetCount int
}

// Attribute is one id/value pair of a StatusUpdate packet.
type Attribute struct {
	ID    int32
	Value int32
}

// Bot tracks the observed state of a single bot session.
type Bot struct {
	mu        sync.RWMutex
	id        string
	status    Status
	selfID    int32
	char      CharacterState
	objects   map[int32]WorldObject
	inventory map[int32]InventoryItem
	events    []Event
	eventLen  int
	eventPos  int
	chatLog   []ChatEvent
	chatLen   int
	chatPos   int
	packets   int64
	version   uint64
	started   time.Time
	updated   time.Time
}

// NewBot creates a bot tracker for the given session id (account name).
func NewBot(id string) *Bot {
	return &Bot{
		mu:        sync.RWMutex{},
		id:        id,
		status:    StatusConnecting,
		selfID:    0,
		char:      newCharacterState(),
		objects:   make(map[int32]WorldObject),
		inventory: make(map[int32]InventoryItem),
		events:    make([]Event, eventCapacity),
		eventLen:  0,
		eventPos:  0,
		chatLog:   make([]ChatEvent, chatCapacity),
		chatLen:   0,
		chatPos:   0,
		packets:   0,
		version:   0,
		started:   time.Now(),
		updated:   time.Time{},
	}
}

// SelfObjectID returns the object id of the played character.
func (b *Bot) SelfObjectID() int32 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.selfID
}

// SelfTargetID returns the object id of the current target of the
// character, zero when nothing is targeted.
func (b *Bot) SelfTargetID() int32 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.char.TargetID
}

// SelfEngaged reports whether the character is currently fighting the
// given target: chasing it or attacking it inside the combat window. It
// is the signal that a forced attack request actually started the fight,
// so the hunt loop can stop re-requesting it.
func (b *Bot) SelfEngaged(targetID int32) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.char.FightingTargetID == targetID && b.char.inCombat(time.Now())
}

// SelfPosition returns the last observed placement of the played
// character.
func (b *Bot) SelfPosition() (int32, int32, int32, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.selfID == 0 {
		return 0, 0, 0, false
	}

	return b.char.X, b.char.Y, b.char.Z, true
}

// SelfSitting reports whether the character is sitting. The hunt loop
// gates the rest transitions on it because the sit/stand action is a
// server side toggle.
func (b *Bot) SelfSitting() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.char.Sitting
}

// SelfUnderAttack reports whether the character was hit within the last
// seconds. The hunt loop uses it to keep the rest logic from sitting
// down in the middle of a fight it did not start itself.
func (b *Bot) SelfUnderAttack() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	last := b.char.LastHitAt

	return !last.IsZero() && time.Since(last) < underAttackWindow
}

// SelfDead reports whether the character died: a known maximum with a
// zero current HP only happens on death (the server broadcasts
// StatusUpdate CUR_HP 0 there).
func (b *Bot) SelfDead() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.char.MaxHP > 0 && b.char.CurHP <= 0
}

// ObjectPosition returns the last observed placement of a known object.
func (b *Bot) ObjectPosition(objectID int32) (int32, int32, int32, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	obj, ok := b.objects[objectID]
	if !ok {
		return 0, 0, 0, false
	}

	return obj.X, obj.Y, obj.Z, true
}

// ObjectAlive reports whether the object is known around the character
// and not dead.
func (b *Bot) ObjectAlive(objectID int32) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	obj, ok := b.objects[objectID]

	return ok && !obj.Dead
}

// SelfHealthPercent returns the current HP of the character as a
// percentage of the maximum (0..100). An unknown maximum (no UserInfo
// yet) counts as healthy: resting forever on missing vitals is worse
// than engaging. The hunt loop uses it to decide whether the character
// is healthy enough to instantly engage the next target.
func (b *Bot) SelfHealthPercent() float64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.char.MaxHP <= 0 {
		return 100
	}
	pct := b.char.CurHP / b.char.MaxHP * 100

	return math.Min(100, math.Max(0, pct))
}

// ID returns the session id of the bot.
func (b *Bot) ID() string {
	return b.id
}

// Version returns a counter that increases on every state change.
func (b *Bot) Version() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.version
}

// Status returns the current session status.
func (b *Bot) Status() Status {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.status
}

// SetOnline marks the character as being inside the world.
func (b *Bot) SetOnline(name string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.char.Name = name
	b.status = StatusOnline
	b.touch()
	b.recordLocked("entered the world as " + name)
}

// SetOffline marks the session as ended.
func (b *Bot) SetOffline() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.status = StatusOffline
	b.touch()
	b.recordLocked("left the world")
}

// ResetSession clears the observed world state before a new login. The
// server starts a fresh known list on every session, so the objects and
// inventory of a lost session must not leak into the next one. The event
// log, the packet counter and the session start time survive so the
// history and the uptime stay continuous across reconnects.
func (b *Bot) ResetSession() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.selfID = 0
	b.char = newCharacterState()
	b.objects = make(map[int32]WorldObject)
	b.inventory = make(map[int32]InventoryItem)
	b.status = StatusConnecting
	b.touch()
}

// SetCharacter applies the initial character state from CharSelected.
func (b *Bot) SetCharacter(
	name string, objectID int32, classID int32,
	x int32, y int32, z int32, curHP float64, curMP float64,
) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.char.Name = name
	b.selfID = objectID
	b.char.ClassID = classID
	b.char.X = x
	b.char.Y = y
	b.char.Z = z
	b.char.CurHP = curHP
	b.char.CurMP = curMP
	b.touch()
	b.recordLocked("character selected: " + name)
}

// UserInfo carries the observed self state from a UserInfo packet.
type UserInfo struct {
	Name          string
	Level         int32
	Race          int32
	ClassID       int32
	X             int32
	Y             int32
	Z             int32
	STR           int32
	DEX           int32
	CON           int32
	INT           int32
	WIT           int32
	MEN           int32
	Exp           int32
	Sp            int32
	MaxHP         int32
	CurHP         int32
	MaxMP         int32
	CurMP         int32
	CurrentLoad   int32
	MaxLoad       int32
	RunSpeed      int32
	WalkSpeed     int32
	MoveSpeedMult float64
}

// ApplyUserInfo updates the character state from a UserInfo packet.
func (b *Bot) ApplyUserInfo(info UserInfo) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.char.Name = info.Name
	b.char.Level = info.Level
	b.char.Race = info.Race
	b.char.ClassID = info.ClassID
	b.char.X = info.X
	b.char.Y = info.Y
	b.char.Z = info.Z
	b.char.STR = info.STR
	b.char.DEX = info.DEX
	b.char.CON = info.CON
	b.char.INT = info.INT
	b.char.WIT = info.WIT
	b.char.MEN = info.MEN
	b.char.Exp = info.Exp
	b.char.Sp = info.Sp
	b.char.MaxHP = float64(info.MaxHP)
	b.char.CurHP = float64(info.CurHP)
	b.char.MaxMP = float64(info.MaxMP)
	b.char.CurMP = float64(info.CurMP)
	b.char.CurrentLoad = info.CurrentLoad
	b.char.MaxLoad = info.MaxLoad
	b.char.RunSpeed = effectiveSpeed(
		info.RunSpeed, info.WalkSpeed, info.MoveSpeedMult)
	b.char.WalkSpeed = effectiveSpeed(
		info.WalkSpeed, info.WalkSpeed, info.MoveSpeedMult)
	b.touch()
}

// ApplyPlacement updates the position and heading of self or an object.
func (b *Bot) ApplyPlacement(p Placement) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if p.ObjectID == b.selfID {
		b.char.X = p.X
		b.char.Y = p.Y
		b.char.Z = p.Z
		b.char.Heading = p.Heading
		b.clearCharMovement()
		b.touch()

		return
	}
	obj, ok := b.objects[p.ObjectID]
	if !ok {
		return
	}
	obj.X = p.X
	obj.Y = p.Y
	obj.Z = p.Z
	obj.Heading = p.Heading
	obj.Moving = p.Moving
	if !p.Moving {
		obj.DestX = p.X
		obj.DestY = p.Y
		obj.DestZ = p.Z
	}
	obj.MoveAt = time.Now()
	obj.UpdatedAt = time.Now()
	b.objects[p.ObjectID] = obj
	b.touch()
}

// ApplyMovement updates the current position and destination of a moving
// object, computing the heading from the movement direction like the
// server does for its creatures. A zero distance packet is the arrival
// broadcast of the server: the object stands at the destination and
// keeps the heading it moved with, exactly like the official client
// renders it.
func (b *Bot) ApplyMovement(m Movement) {
	b.mu.Lock()
	defer b.mu.Unlock()
	arrived := m.DestX == m.X && m.DestY == m.Y && m.DestZ == m.Z
	if m.ObjectID == b.selfID {
		b.char.X = m.X
		b.char.Y = m.Y
		b.char.Z = m.Z
		if arrived {
			b.clearCharMovement()
		} else {
			b.char.Heading = HeadingFromDelta(
				m.DestX-m.X, m.DestY-m.Y)
			b.char.Moving = true
			b.char.DestX = m.DestX
			b.char.DestY = m.DestY
			b.char.DestZ = m.DestZ
			b.char.MoveAt = time.Now()
		}
		b.touch()

		return
	}
	obj, ok := b.objects[m.ObjectID]
	if !ok {
		return
	}
	obj.X = m.X
	obj.Y = m.Y
	obj.Z = m.Z
	if !arrived {
		obj.Heading = HeadingFromDelta(m.DestX-m.X, m.DestY-m.Y)
	}
	obj.DestX = m.DestX
	obj.DestY = m.DestY
	obj.DestZ = m.DestZ
	obj.Moving = !arrived
	obj.MoveAt = time.Now()
	obj.UpdatedAt = time.Now()
	b.objects[m.ObjectID] = obj
	b.touch()
}

// ApplyPawnMovement updates a chasing object: it runs toward the point
// `distance` in front of its target and faces the target. The packet is
// only sent for attacking creatures, so it also refreshes the combat
// state and the target reference. The played character is handled as
// well because the server broadcasts its own chase to it.
func (b *Bot) ApplyPawnMovement(m PawnMovement) {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	destX, destY := pawnDestination(m)
	if m.ObjectID == b.selfID {
		b.applySelfPawnMovementLocked(m, destX, destY, now)
		b.touch()

		return
	}
	if m.TargetID == b.selfID {
		b.char.X = m.TargetX
		b.char.Y = m.TargetY
		b.char.Z = m.TargetZ
		b.char.CombatUntil = now.Add(combatWindow)
		b.char.LastHitAt = now
		b.touch()
	}
	obj, ok := b.objects[m.ObjectID]
	if !ok {
		return
	}
	obj.X = m.X
	obj.Y = m.Y
	obj.Z = m.Z
	obj.DestX = destX
	obj.DestY = destY
	obj.DestZ = m.TargetZ
	if m.X != m.TargetX || m.Y != m.TargetY {
		obj.Heading = HeadingFromDelta(m.TargetX-m.X, m.TargetY-m.Y)
	}
	obj.Moving = true
	obj.Running = true
	obj.TargetID = m.TargetID
	b.markObjectCombatLocked(&obj, now)
	obj.MoveAt = now
	obj.UpdatedAt = now
	b.objects[m.ObjectID] = obj
	b.touch()
}

// applySelfPawnMovementLocked tracks the played character chasing its
// attack target. The caller must hold the state write lock.
func (b *Bot) applySelfPawnMovementLocked(
	m PawnMovement, destX int32, destY int32, now time.Time,
) {
	b.char.X = m.X
	b.char.Y = m.Y
	b.char.Z = m.Z
	if m.X != m.TargetX || m.Y != m.TargetY {
		b.char.Heading = HeadingFromDelta(m.TargetX-m.X, m.TargetY-m.Y)
	}
	b.char.DestX = destX
	b.char.DestY = destY
	b.char.DestZ = m.TargetZ
	b.char.Moving = destX != m.X || destY != m.Y
	b.char.MoveAt = now
	b.char.TargetID = m.TargetID
	b.char.FightingTargetID = m.TargetID
	b.char.CombatUntil = now.Add(combatWindow)
}

// pawnDestination computes the stop point of a chasing object.
func pawnDestination(m PawnMovement) (int32, int32) {
	dx := float64(m.X - m.TargetX)
	dy := float64(m.Y - m.TargetY)
	dist := math.Hypot(dx, dy)
	if dist < 1 {
		return m.X, m.Y
	}
	stopX := m.TargetX + int32(dx/dist*float64(m.Distance))
	stopY := m.TargetY + int32(dy/dist*float64(m.Distance))

	return stopX, stopY
}

// ApplyAttack updates the attacker placement and marks the attacker and
// every known hit target as fighting. The attacker faces its first hit
// target, exactly like the official client renders it: the Attack packet
// carries the attacker and the target positions but no heading, and the
// server set the attacker heading toward the target before broadcasting
// (see Creature.doAttack). When the played character is the target, the
// trailing target location doubles as a position update of it.
func (b *Bot) ApplyAttack(a Attack) {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	facing := HeadingFromDelta(a.TargetX-a.X, a.TargetY-a.Y)
	hasFacing := a.TargetX != a.X || a.TargetY != a.Y
	if a.AttackerID == b.selfID {
		b.char.X = a.X
		b.char.Y = a.Y
		b.char.Z = a.Z
		b.clearCharMovement()
		if hasFacing {
			b.char.Heading = facing
		}
		if a.TargetCount > 0 {
			b.char.TargetID = a.TargetIDs[0]
			b.char.FightingTargetID = a.TargetIDs[0]
		}
		b.char.CombatUntil = now.Add(combatWindow)
		b.touch()
	} else if obj, ok := b.objects[a.AttackerID]; ok {
		obj.X = a.X
		obj.Y = a.Y
		obj.Z = a.Z
		if hasFacing {
			obj.Heading = facing
		}
		if a.TargetCount > 0 {
			obj.TargetID = a.TargetIDs[0]
		}
		b.markObjectCombatLocked(&obj, now)
		obj.UpdatedAt = now
		b.objects[a.AttackerID] = obj
		b.touch()
	}
	for i := range a.TargetCount {
		if a.TargetIDs[i] == b.selfID {
			b.char.X = a.TargetX
			b.char.Y = a.TargetY
			b.char.Z = a.TargetZ
			b.char.CombatUntil = now.Add(combatWindow)
			b.char.LastHitAt = now
			b.touch()

			continue
		}
		if obj, ok := b.objects[a.TargetIDs[i]]; ok {
			b.markObjectCombatLocked(&obj, now)
			obj.UpdatedAt = now
			b.objects[a.TargetIDs[i]] = obj
		}
	}
}

// ApplyAutoAttackStart marks an object as auto attacking.
func (b *Bot) ApplyAutoAttackStart(objectID int32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	if objectID == b.selfID {
		b.char.AutoAttacking = true
		b.char.CombatUntil = now.Add(combatWindow)
		b.touch()

		return
	}
	obj, ok := b.objects[objectID]
	if !ok {
		return
	}
	obj.AutoAttacking = true
	b.markObjectCombatLocked(&obj, now)
	obj.UpdatedAt = now
	b.objects[objectID] = obj
	b.touch()
}

// ApplyAutoAttackStop clears the auto attack flag of an object.
func (b *Bot) ApplyAutoAttackStop(objectID int32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if objectID == b.selfID {
		b.char.AutoAttacking = false
		b.touch()

		return
	}
	obj, ok := b.objects[objectID]
	if !ok {
		return
	}
	obj.AutoAttacking = false
	obj.UpdatedAt = time.Now()
	b.objects[objectID] = obj
	b.touch()
}

// ApplyNpcInfo upserts an observed npc object.
func (b *Bot) ApplyNpcInfo(info NpcInfo) {
	b.mu.Lock()
	defer b.mu.Unlock()
	obj := b.upsertLocked(info.ObjectID, KindNPC)
	obj.TemplateID = info.TemplateID
	obj.Attackable = info.Attackable
	obj.Aggressive = npcdata.NPCIsAggressive(info.TemplateID)
	obj.AggroRange = npcdata.NPCAggroRange(info.TemplateID)
	obj.Level = npcdata.NPCLevel(info.TemplateID)
	obj.X = info.X
	obj.Y = info.Y
	obj.Z = info.Z
	obj.Heading = info.Heading
	obj.RunSpeed = info.RunSpeed
	obj.WalkSpeed = info.WalkSpeed
	obj.MoveSpeedMult = info.MoveSpeedMult
	obj.CollisionRadius = info.CollisionRadius
	obj.Running = info.Running
	obj.Moving = false
	obj.DestX = info.X
	obj.DestY = info.Y
	obj.DestZ = info.Z
	obj.Dead = info.Dead
	obj.Name = resolveNpcName(info.Name, info.TemplateID)
	obj.Title = info.Title
	now := time.Now()
	if info.InCombat {
		b.markObjectCombatLocked(&obj, now)
	}
	obj.MoveAt = now
	obj.UpdatedAt = now
	b.objects[info.ObjectID] = obj
	b.touch()
	b.recordLocked("npc spawned: " + obj.Name)
}

// ApplyPlayerInfo upserts an observed player object.
func (b *Bot) ApplyPlayerInfo(info PlayerInfo) {
	b.mu.Lock()
	defer b.mu.Unlock()
	obj := b.upsertLocked(info.ObjectID, KindPlayer)
	obj.Name = info.Name
	obj.Title = info.Title
	obj.X = info.X
	obj.Y = info.Y
	obj.Z = info.Z
	obj.RunSpeed = info.RunSpeed
	obj.WalkSpeed = info.WalkSpeed
	obj.MoveSpeedMult = info.MoveSpeedMult
	obj.CollisionRadius = info.CollisionRadius
	obj.Running = info.Running
	obj.Dead = info.Dead
	obj.Moving = false
	obj.DestX = info.X
	obj.DestY = info.Y
	obj.DestZ = info.Z
	now := time.Now()
	if info.InCombat {
		b.markObjectCombatLocked(&obj, now)
	}
	obj.MoveAt = now
	obj.UpdatedAt = now
	b.objects[info.ObjectID] = obj
	b.touch()
	b.recordLocked("player appeared: " + info.Name)
}

// ApplyItemInfo upserts an observed ground item object.
func (b *Bot) ApplyItemInfo(info ItemInfo) {
	b.mu.Lock()
	defer b.mu.Unlock()
	obj := b.upsertLocked(info.ObjectID, KindItem)
	obj.TemplateID = info.TemplateID
	obj.Name = npcdata.ItemName(info.TemplateID)
	obj.Count = info.Count
	obj.X = info.X
	obj.Y = info.Y
	obj.Z = info.Z
	obj.UpdatedAt = time.Now()
	b.objects[info.ObjectID] = obj
	b.touch()
	b.recordLocked("item dropped: " + itemName(obj.Name, info.TemplateID))
}

// RemoveObject deletes an object that left the known list. When the
// bot itself targeted the object, the target is dropped as well: the
// server answers the removal only with the DeleteObject broadcast.
func (b *Bot) RemoveObject(objectID int32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	obj, ok := b.objects[objectID]
	if !ok {
		return
	}
	delete(b.objects, objectID)
	if b.char.TargetID == objectID {
		b.clearSelfTargetLocked("target object removed")
	}
	b.touch()
	b.recordLocked("object removed: " + obj.Name)
}

// Status attribute ids from the Mobius StatusUpdate packet.
const (
	AttrLevel   = 0x01
	AttrCurHP   = 0x09
	AttrMaxHP   = 0x0A
	AttrCurMP   = 0x0B
	AttrMaxMP   = 0x0C
	AttrCurLoad = 0x0E
	AttrMaxLoad = 0x0F
)

// ApplyStatusUpdate applies vitals attribute changes to self or an object.
func (b *Bot) ApplyStatusUpdate(objectID int32, attrs []Attribute) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if objectID == b.charObjectID() {
		for _, attr := range attrs {
			b.applyCharAttr(attr)
		}
		b.touch()

		return
	}
	obj, ok := b.objects[objectID]
	if !ok {
		return
	}
	for _, attr := range attrs {
		switch attr.ID {
		case AttrCurHP:
			obj.CurHP = float64(attr.Value)
			obj.Dead = attr.Value <= 0
		case AttrMaxHP:
			obj.MaxHP = float64(attr.Value)
		case AttrCurMP:
			obj.CurMP = float64(attr.Value)
		case AttrMaxMP:
			obj.MaxMP = float64(attr.Value)
		}
	}
	obj.UpdatedAt = time.Now()
	b.objects[objectID] = obj
	if obj.Dead && b.char.TargetID == objectID {
		// A killed target is no target anymore: the server keeps
		// the corpse selected, the tracker drops it so the HUD
		// and the hunt loop see the actual "no target" state.
		b.clearSelfTargetLocked("target died")
	}
	b.touch()
}

// applyCharAttr applies a single attribute to the played character.
func (b *Bot) applyCharAttr(attr Attribute) {
	switch attr.ID {
	case AttrCurHP:
		b.char.CurHP = float64(attr.Value)
	case AttrMaxHP:
		b.char.MaxHP = float64(attr.Value)
	case AttrCurMP:
		b.char.CurMP = float64(attr.Value)
	case AttrMaxMP:
		b.char.MaxMP = float64(attr.Value)
	case AttrLevel:
		b.char.Level = attr.Value
	case AttrCurLoad:
		b.char.CurrentLoad = attr.Value
	case AttrMaxLoad:
		b.char.MaxLoad = attr.Value
	}
}

// CountPacket accounts one received packet.
func (b *Bot) CountPacket() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.packets++
}

// RecordEvent appends a message to the rolling event log.
func (b *Bot) RecordEvent(message string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.recordLocked(message)
}

// upsertLocked returns the existing object or a fresh one for the id.
// The caller must hold the write lock.
func (b *Bot) upsertLocked(objectID int32, kind ObjectKind) WorldObject {
	if obj, ok := b.objects[objectID]; ok {
		return obj
	}

	return newWorldObject(objectID, kind)
}

// charObjectID returns the object id of the self player, zero when the
// character is unknown. The caller must hold a lock.
func (b *Bot) charObjectID() int32 {
	return b.selfID
}

// touch bumps the version and the update timestamp.
func (b *Bot) touch() {
	b.version++
	b.updated = time.Now()
}

// clearCharMovement resets the self movement tracking. The caller must
// hold the write lock.
func (b *Bot) clearCharMovement() {
	b.char.Moving = false
	b.char.DestX = b.char.X
	b.char.DestY = b.char.Y
	b.char.DestZ = b.char.Z
	b.char.MoveAt = time.Now()
}

// clearSelfTargetLocked drops the target of the character and records
// the transition. The Mobius server keeps a killed or despawned target
// selected on its side (MyTargetSelected only answers new selections and
// the own TargetUnselected broadcast is the sole removal notice), so the
// tracker mirrors the official client: the target is gone once it died
// or was removed from the world. The caller must hold the write lock.
func (b *Bot) clearSelfTargetLocked(reason string) {
	if b.char.TargetID == 0 {
		return
	}
	b.char.TargetID = 0
	b.touch()
	b.recordLocked("target cleared: " + reason)
}

// AttackTarget describes a target the bot can attack.
type AttackTarget struct {
	ObjectID int32
	Name     string
	X        int32
	Y        int32
	Z        int32
}

// NearestAttackable returns the closest living attackable npc within the
// given distance of the character. The distance uses the projected
// current position of every npc (see projectedPosition), not the raw
// packet position: the server broadcasts movement at most once per
// second, so a moving mob is typically tens or hundreds of units away
// from its last packet start position and a stale "nearest" choice
// would send the character to a mob that is no longer the closest one.
func (b *Bot) NearestAttackable(maxDistance float64) (AttackTarget, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	//nolint:exhaustruct // zero value grows inside the loop
	best := AttackTarget{}
	bestDist := maxDistance
	found := false
	selfX := float64(b.char.X)
	selfY := float64(b.char.Y)
	now := time.Now()
	for _, obj := range b.objects {
		if obj.Kind != KindNPC || !obj.Attackable || obj.Dead {
			continue
		}
		x, y := projectedPosition(obj, now)
		dist := math.Hypot(x-selfX, y-selfY)
		if dist < bestDist {
			bestDist = dist
			found = true
			best = AttackTarget{
				ObjectID: obj.ObjectID,
				Name:     obj.Name,
				X:        int32(math.Round(x)),
				Y:        int32(math.Round(y)),
				Z:        obj.Z,
			}
		}
	}

	return best, found
}

// projectedPosition estimates where an object is right now: standing
// objects keep their packet position, moving ones advance from the
// segment start toward the destination at their effective speed. It is
// the server side counterpart of the web map interpolation (the Mobius
// Creature.updatePosition loop steps creatures toward the destination
// every 100 ms game tick from the last broadcast position).
func projectedPosition(obj WorldObject, now time.Time) (float64, float64) {
	if !obj.Moving || obj.MoveAt.IsZero() {
		return float64(obj.X), float64(obj.Y)
	}
	dx := float64(obj.DestX - obj.X)
	dy := float64(obj.DestY - obj.Y)
	dist := math.Hypot(dx, dy)
	speed := obj.EffectiveSpeed()
	if dist < 1 || speed <= 0 {
		return float64(obj.X), float64(obj.Y)
	}
	elapsed := now.Sub(obj.MoveAt).Seconds()
	if elapsed <= 0 {
		return float64(obj.X), float64(obj.Y)
	}
	traveled := math.Min(speed*elapsed, dist)
	frac := traveled / dist

	return float64(obj.X) + dx*frac, float64(obj.Y) + dy*frac
}

// recordLocked appends an event to the ring buffer. The caller must hold
// the write lock.
func (b *Bot) recordLocked(message string) {
	b.events[b.eventPos] = Event{Time: time.Now(), Message: message}
	b.eventPos = (b.eventPos + 1) % eventCapacity
	if b.eventLen < eventCapacity {
		b.eventLen++
	}
}

// markObjectCombatLocked refreshes the combat window of an object and
// logs the transition into combat once. The caller must hold the state
// write lock.
func (b *Bot) markObjectCombatLocked(obj *WorldObject, now time.Time) {
	if !obj.InCombat(now) && obj.Name != "" {
		b.recordLocked(obj.Name + " enters combat")
	}
	obj.CombatUntil = now.Add(combatWindow)
}

// CharacterSnapshot is the JSON view of the character state.
type CharacterSnapshot struct {
	ObjectID        int32   `json:"objectId"`
	Name            string  `json:"name"`
	TargetID        int32   `json:"targetId"`
	Moving          bool    `json:"moving"`
	DestX           int32   `json:"destX"`
	DestY           int32   `json:"destY"`
	DestZ           int32   `json:"destZ"`
	Speed           float64 `json:"speed"`
	CollisionRadius float64 `json:"collisionRadius"`
	MoveAtMs        int64   `json:"moveAtMs"`
	Level           int32   `json:"level"`
	Race            int32   `json:"race"`
	ClassID         int32   `json:"classId"`
	X               int32   `json:"x"`
	Y               int32   `json:"y"`
	Z               int32   `json:"z"`
	Heading         int32   `json:"heading"`
	CurHP           float64 `json:"curHp"`
	MaxHP           float64 `json:"maxHp"`
	CurMP           float64 `json:"curMp"`
	MaxMP           float64 `json:"maxMp"`
	Sitting         bool    `json:"sitting"`
	STR             int32   `json:"str"`
	DEX             int32   `json:"dex"`
	CON             int32   `json:"con"`
	INT             int32   `json:"int"`
	WIT             int32   `json:"wit"`
	MEN             int32   `json:"men"`
	Exp             int32   `json:"exp"`
	ExpPercent      float64 `json:"expPercent"`
	Sp              int32   `json:"sp"`
	InCombat        bool    `json:"inCombat"`
	CurrentLoad     int32   `json:"load"`
	MaxLoad         int32   `json:"maxLoad"`
	InventorySlots  int     `json:"inventorySlots"`
	InventoryMax    int     `json:"inventoryMax"`
	Adena           int32   `json:"adena"`
}

// ObjectSnapshot is the JSON view of a world object.
type ObjectSnapshot struct {
	ObjectID        int32      `json:"objectId"`
	Kind            ObjectKind `json:"kind"`
	Name            string     `json:"name"`
	Title           string     `json:"title"`
	TemplateID      int32      `json:"templateId"`
	Attackable      bool       `json:"attackable"`
	Aggressive      bool       `json:"aggressive"`
	AggroRange      int32      `json:"aggroRange"`
	Level           int32      `json:"level"`
	TargetID        int32      `json:"targetId"`
	InCombat        bool       `json:"inCombat"`
	Dead            bool       `json:"dead"`
	Moving          bool       `json:"moving"`
	Running         bool       `json:"running"`
	Speed           float64    `json:"speed"`
	CollisionRadius float64    `json:"collisionRadius"`
	Count           int32      `json:"count"`
	X               int32      `json:"x"`
	Y               int32      `json:"y"`
	Z               int32      `json:"z"`
	Heading         int32      `json:"heading"`
	DestX           int32      `json:"destX"`
	DestY           int32      `json:"destY"`
	DestZ           int32      `json:"destZ"`
	MoveAtMs        int64      `json:"moveAtMs"`
	CurHP           float64    `json:"curHp"`
	MaxHP           float64    `json:"maxHp"`
	CurMP           float64    `json:"curMp"`
	MaxMP           float64    `json:"maxMp"`
}

// Snapshot is the JSON view of the whole bot state.
type Snapshot struct {
	ID           string            `json:"id"`
	Status       Status            `json:"status"`
	Character    CharacterSnapshot `json:"character"`
	Objects      []ObjectSnapshot  `json:"objects"`
	Events       []Event           `json:"events"`
	Chat         []ChatEvent       `json:"chat"`
	Packets      int64             `json:"packets"`
	Version      uint64            `json:"version"`
	ServerTimeMs int64             `json:"serverTimeMs"`
	StartedAt    time.Time         `json:"startedAt"`
	UpdatedAt    time.Time         `json:"updatedAt"`
}

// Snapshot returns a deep copy of the current state for serialization.
func (b *Bot) Snapshot() Snapshot {
	b.mu.RLock()
	defer b.mu.RUnlock()
	now := time.Now()

	snap := Snapshot{
		ID:     b.id,
		Status: b.status,
		Character: CharacterSnapshot{
			ObjectID:        b.selfID,
			Name:            b.char.Name,
			TargetID:        b.char.TargetID,
			Moving:          b.char.Moving,
			DestX:           b.char.DestX,
			DestY:           b.char.DestY,
			DestZ:           b.char.DestZ,
			Speed:           b.char.RunSpeed,
			CollisionRadius: b.char.CollisionRadius,
			MoveAtMs:        b.char.MoveAt.UnixMilli(),
			Level:           b.char.Level,
			Race:            b.char.Race,
			ClassID:         b.char.ClassID,
			X:               b.char.X,
			Y:               b.char.Y,
			Z:               b.char.Z,
			Heading:         b.char.Heading,
			CurHP:           b.char.CurHP,
			MaxHP:           b.char.MaxHP,
			CurMP:           b.char.CurMP,
			MaxMP:           b.char.MaxMP,
			Sitting:         b.char.Sitting,
			STR:             b.char.STR,
			DEX:             b.char.DEX,
			CON:             b.char.CON,
			INT:             b.char.INT,
			WIT:             b.char.WIT,
			MEN:             b.char.MEN,
			Exp:             b.char.Exp,
			ExpPercent: ExpPercent(b.char.Level,
				int64(b.char.Exp)),
			Sp:       b.char.Sp,
			InCombat: b.char.inCombat(now),
		},
		Objects:      make([]ObjectSnapshot, 0, len(b.objects)),
		Events:       make([]Event, 0, min(b.eventLen, snapshotEvents)),
		Chat:         make([]ChatEvent, 0, b.chatLen),
		Packets:      b.packets,
		Version:      b.version,
		ServerTimeMs: now.UnixMilli(),
		StartedAt:    b.started,
		UpdatedAt:    b.updated,
	}
	for _, obj := range b.objects {
		snap.Objects = append(snap.Objects, ObjectSnapshot{
			ObjectID:        obj.ObjectID,
			Kind:            obj.Kind,
			Name:            obj.Name,
			Title:           obj.Title,
			TemplateID:      obj.TemplateID,
			Attackable:      obj.Attackable,
			Aggressive:      obj.Aggressive,
			AggroRange:      obj.AggroRange,
			Level:           obj.Level,
			TargetID:        obj.TargetID,
			InCombat:        obj.InCombat(now),
			Dead:            obj.Dead,
			Moving:          obj.Moving,
			Running:         obj.Running,
			Speed:           obj.EffectiveSpeed(),
			CollisionRadius: obj.CollisionRadius,
			Count:           obj.Count,
			X:               obj.X,
			Y:               obj.Y,
			Z:               obj.Z,
			Heading:         obj.Heading,
			DestX:           obj.DestX,
			DestY:           obj.DestY,
			DestZ:           obj.DestZ,
			MoveAtMs:        obj.MoveAt.UnixMilli(),
			CurHP:           obj.CurHP,
			MaxHP:           obj.MaxHP,
			CurMP:           obj.CurMP,
			MaxMP:           obj.MaxMP,
		})
	}
	snap.Events = appendEvents(
		snap.Events, b.events, b.eventLen, b.eventPos)
	snap.Chat = appendChat(snap.Chat, b.chatLog, b.chatLen, b.chatPos)
	b.fillInventorySnapshot(&snap)

	return snap
}

// fillInventorySnapshot completes the character view with the inventory
// usage. The caller must hold the read lock.
func (b *Bot) fillInventorySnapshot(snap *Snapshot) {
	snap.Character.CurrentLoad = b.char.CurrentLoad
	snap.Character.MaxLoad = b.char.MaxLoad
	snap.Character.InventorySlots = len(b.inventory)
	snap.Character.InventoryMax = inventorySlotLimit
	for _, item := range b.inventory {
		if item.Type2 == itemType2Adena {
			snap.Character.Adena += item.Count
		}
	}
}

// appendChat copies the chat window lines out of the ring buffer in
// chronological order.
func appendChat(
	dst []ChatEvent, chat []ChatEvent, length int, pos int,
) []ChatEvent {
	for i := 0; i < length; i++ {
		dst = append(dst, chat[(pos-length+i+chatCapacity)%chatCapacity])
	}

	return dst
}

// appendEvents copies the newest events out of the ring buffer.
func appendEvents(dst []Event, events []Event, length int, pos int) []Event {
	count := min(length, snapshotEvents)
	for i := count; i > 0; i-- {
		index := (pos - i + eventCapacity) % eventCapacity
		dst = append(dst, events[index])
	}

	return dst
}

// BotInfo is the compact JSON view used by the bot list endpoint. The
// vitals and the combat flag let the sidebar show the mini HP/MP/XP bars
// and the fighting state of every session at a glance.
type BotInfo struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Status     Status  `json:"status"`
	Level      int32   `json:"level"`
	CurHP      float64 `json:"curHp"`
	MaxHP      float64 `json:"maxHp"`
	CurMP      float64 `json:"curMp"`
	MaxMP      float64 `json:"maxMp"`
	ExpPercent float64 `json:"expPercent"`
	InCombat   bool    `json:"inCombat"`
	Sitting    bool    `json:"sitting"`
}

// Info returns the compact bot description.
func (b *Bot) Info() BotInfo {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return BotInfo{
		ID:         b.id,
		Name:       b.char.Name,
		Status:     b.status,
		Level:      b.char.Level,
		CurHP:      b.char.CurHP,
		MaxHP:      b.char.MaxHP,
		CurMP:      b.char.CurMP,
		MaxMP:      b.char.MaxMP,
		ExpPercent: ExpPercent(b.char.Level, int64(b.char.Exp)),
		InCombat:   b.char.inCombat(time.Now()),
		Sitting:    b.char.Sitting,
	}
}

// npcDisplayOffset mirrors the template id offset of NpcInfo packets.
const npcDisplayOffset = 1000000

// resolveNpcName prefers the server side name and falls back to the
// generated npc dictionary for client side names.
func resolveNpcName(name string, templateID int32) string {
	if name != "" {
		return name
	}
	resolved := npcdata.NPCName(templateID)
	if resolved != "" {
		return resolved
	}

	return "npc #" + strconv.Itoa(int(templateID-npcDisplayOffset))
}

// itemName formats the item name with the display id fallback.
func itemName(name string, displayID int32) string {
	if name != "" {
		return name
	}

	return "#" + strconv.Itoa(int(displayID))
}
