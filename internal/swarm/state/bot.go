// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
	"math"
	"sort"
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
	CurrentLoad         int32
	MaxLoad             int32
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
	// HitFlags carries the per hit flags (see attackHitMissFlag) of
	// the TargetIDs entries.
	HitFlags    [AttackTargets]int8
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
)

// CombatEvent is one observed beat of the combat animation feed
// of the web view: an attack swing of the Attack broadcast or a
// damage landing of a StatusUpdate HP drop. The monotonic
// sequence lets the client replay every event exactly once across
// the repeated snapshots of the event stream.
type CombatEvent struct {
	Seq        uint64
	Kind       string
	AttackerID int32
	TargetID   int32
	Amount     float64
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

// Bot tracks the observed state of a single bot session.
type Bot struct {
	mu     sync.RWMutex
	id     string
	status Status
	// phase mirrors the hunt loop phase so the web UI can show a
	// human readable activity banner (hunting, walking to town,
	// selling, deleveling). Empty until the loop publishes its
	// first phase; the manual only sessions stay empty (the loop
	// never sets it) and the UI falls back to the status text.
	phase  string
	selfID int32
	char   CharacterState
	// world is the dense object storage (see objectStore): the
	// packet apply paths mutate the records in place and the
	// scans walk the memory sequentially.
	world     objectStore
	inventory inventoryStore
	// inventoryVersion counts the inventory and paperdoll
	// mutations: the equip managers of the hunt loop key their
	// cached scans on it, so an unchanged bag costs no per tick
	// re-scan (see hunt equipManager).
	inventoryVersion uint64
	paperdoll        [PaperdollSlots]int32
	// log is the rolling packet event log, chat the chat window
	// ring, combat the web view animation feed (one component
	// value each, see their types for the layout contracts).
	log       eventLog
	chat      chatLog
	combat    combatFeed
	zone      *Zone
	zoneViews []ZoneView
	// killMarks carries the recent kills of the hunt loop (the kill
	// ring of the spot hunter): the positions feed the fleet wide
	// cross layer of the map, so the crosses survive the bot switches
	// of the web view.
	killMarks    []KillMarkView
	packets      int64
	version      uint64
	started      time.Time
	updated      time.Time
	commandQueue chan Command
	// The published walk plan of the web UI and the state dump
	// (see SetWalkPlan): the planning origin, the full waypoint
	// list of the leg, the waypoint the follower currently heads
	// to and the final destination - the whole walk reads at a
	// glance in the dump.
	walkPlan   *WalkPlan
	walkPlanAt time.Time
	// shopping holds the published purchase queue of the shop
	// strategy (see SetShoppingPlan): what the bot plans to buy next
	// with the prices and the missing adena, nil while nothing is
	// published. shoppingAt bounds its freshness (shoppingPlanTTL).
	shopping   *ShoppingPlanView
	shoppingAt time.Time
	// skills holds the learned skill list of the server packet
	// (id -> level + passive). skillsRevision counts the SetSkills
	// calls and the weapon priority changes; skillQueue caches the
	// ordered learning queue and rebuilds when the class or the
	// revision moves (see ensureSkillQueueLocked). skillWeapons
	// holds the weapon families the queue prefers (the weapon in
	// hand and the next weapon of the purchase plan).
	skills             map[int32]learnedSkill
	skillsRevision     uint64
	skillQueue         []SkillPlanEntry
	skillQueueClass    int32
	skillQueueRevision uint64
	skillWeapons       []string
	// buffs holds the active effect list of the server
	// AbnormalStatusUpdate packets (skillId -> level + seconds left
	// at the arrival); buffsAt anchors the remaining seconds the
	// views count down from (see SetBuffs).
	buffs   map[int32]buffRecord
	buffsAt time.Time
	// loginCooldownUntil holds the reconnect pause the supervisor
	// honors after an emergency logout. The tracker outlives the
	// sessions, so the cooldown spans them (see SetLoginCooldown).
	loginCooldownUntil time.Time
}

// NewBot creates a bot tracker for the given session id (account name).
func NewBot(id string) *Bot {
	return &Bot{
		mu:                 sync.RWMutex{},
		id:                 id,
		status:             StatusConnecting,
		phase:              "",
		loginCooldownUntil: time.Time{},
		selfID:             0,
		char:               newCharacterState(),
		world:              newObjectStore(),
		inventory:          newInventoryStore(),
		inventoryVersion:   0,
		paperdoll:          [PaperdollSlots]int32{},
		log:                newEventLog(),
		chat:               newChatLog(),
		combat:             newCombatFeed(),
		zone:               nil,
		zoneViews:          nil,
		packets:            0,
		version:            0,
		started:            time.Now(),
		updated:            time.Time{},
		commandQueue:       make(chan Command, commandQueueCapacity),
		walkPlan:           nil,
		walkPlanAt:         time.Time{},
		shopping:           nil,
		shoppingAt:         time.Time{},
		skills:             nil,
		skillsRevision:     0,
		skillQueue:         nil,
		skillQueueClass:    0,
		skillQueueRevision: 0,
		skillWeapons:       nil,
		buffs:              nil,
		buffsAt:            time.Time{},
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

// SelfFighting reports whether the character is actually swinging at or
// chasing the given target right now: the engagement must be fresh (an
// attack or a chase step within the last seconds). The auto attack flag
// and the combat window linger after an interrupted fight, so a manual
// attack command relies on this stricter view: a stale engagement
// re-requests the forced attack instead of trusting the stale flags.
func (b *Bot) SelfFighting(targetID int32) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.char.FightingTargetID == targetID &&
		b.char.fightingFresh(time.Now())
}

// SelfCannotSeeTargetAt returns when the server last answered the attack
// with "Cannot see target." (zero when it never happened): the geodata
// line of sight to the attack target is blocked, an obstacle stands
// between the character and the target. The hunt loop reads it to
// recognize the obstructed engage and walk around the obstacle instead
// of re-requesting the refused attack forever.
func (b *Bot) SelfCannotSeeTargetAt() time.Time {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.char.CannotSeeTargetAt
}

// SelfWalking reports whether the character is moving right now: the
// moving flag is set by the movement broadcasts and the fresh window
// guards against a lost stop packet (no update for seconds means the
// walk stalled even though the flag claims motion).
func (b *Bot) SelfWalking() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.char.Moving && time.Since(b.char.MoveAt) <= walkingFreshWindow
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

// SelfLevel returns the observed level of the played character, zero
// before the first UserInfo or level StatusUpdate.
func (b *Bot) SelfLevel() int32 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.char.Level
}

// SelfClassID returns the class id of the played character: the skill
// teachers and the learning queue resolve through it. Zero before the
// first UserInfo.
func (b *Bot) SelfClassID() int32 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.char.ClassID
}

// SelfSp returns the current skill points of the played character:
// the learning trigger and the per lesson budget of the teacher stop
// read it.
func (b *Bot) SelfSp() int32 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.char.Sp
}

// SkillsRevision returns the count of the SetSkills calls (and the
// weapon priority changes): the teacher stop of the hunt loop waits
// on its bump as the learn confirmation - the server answers every
// learned lesson with a fresh SkillList.
func (b *Bot) SkillsRevision() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.skillsRevision
}

// SkillPlan returns a copy of the current learning queue view (nil
// when the class is unknown or nothing is left to learn). The hunt
// loop plans its teacher trips on it.
func (b *Bot) SkillPlan() *SkillPlanView {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.skillPlanViewLocked()
}

// SelfExp returns the last observed experience of the played character.
// The server refreshes the value in the UserInfo packet after every
// experience change (the PlayerStat add and remove paths both call
// updateUserInfo), so a death experience penalty shows up here at once;
// a death that removed nothing leaves the value untouched.
func (b *Bot) SelfExp() int32 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.char.Exp
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

// SelfLandedHit returns the target of the last blow of the played
// character that actually landed, with the time it landed. The miss
// flagged swings never update the pair, so a zero target means no
// landed blow yet (or since the last tracker reset).
func (b *Bot) SelfLandedHit() (int32, time.Time) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.char.LastLandedHitTarget, b.char.LastLandedHitAt
}

// SelfAttackerCount returns how many living attackable npcs hold
// the played character as their target right now: the aggro load
// of the moment. The emergency logout of the hunt loop fires on a
// social pile up - two swinging mobs grind a lone farmer down
// faster than any escape could answer.
func (b *Bot) SelfAttackerCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.selfID == 0 {
		return 0
	}
	count := 0
	for i := range b.world.hot {
		obj := &b.world.hot[i]
		if obj.Kind == kindNPC && obj.Attackable && !obj.Dead &&
			obj.TargetID == b.selfID {
			count++
		}
	}

	return count
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
	obj, _ := b.objectLocked(objectID)
	if obj == nil {
		return 0, 0, 0, false
	}

	return obj.X, obj.Y, obj.Z, true
}

// ObjectName returns the display name of a known object, an empty
// string when the object is unknown.
func (b *Bot) ObjectName(objectID int32) string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	_, cold := b.objectLocked(objectID)
	if cold == nil {
		return ""
	}

	return cold.Name
}

// ObjectTemplateID returns the wire template id of a known npc (the
// NpcInfo template id, display id plus the 1000000 offset), zero when
// the object is unknown or not an npc. The spot respawn overlay keys
// the kill records through it onto the mob lists of the registry.
func (b *Bot) ObjectTemplateID(objectID int32) int32 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	obj, cold := b.objectLocked(objectID)
	if obj == nil || cold == nil || obj.Kind != kindNPC {
		return 0
	}

	return cold.TemplateID
}

// ObjectAlive reports whether the object is known around the character
// and not dead.
func (b *Bot) ObjectAlive(objectID int32) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	obj, _ := b.objectLocked(objectID)

	return obj != nil && !obj.Dead
}

// KnownObjectIDs lists the object ids of the currently known world (the
// known list of the session: npcs, players and ground items). The proxy
// snapshots it when a bot session ends and replays a DeleteObject for
// every entry before the resync onto the next session, so a held client
// never keeps ghosts of the old world. The ids are unordered.
func (b *Bot) KnownObjectIDs() []int32 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	ids := make([]int32, 0, len(b.world.hot))
	for i := range b.world.hot {
		ids = append(ids, b.world.hot[i].ObjectID)
	}

	return ids
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

// ActiveSkills returns the learned active skills (the non passive
// ones) in ascending skill id order: the combat casting of the hunt
// loop picks its strikes, spells and buffs from the list. The answer
// is a fresh slice - the caller owns it.
func (b *Bot) ActiveSkills() []LearnedSkill {
	b.mu.RLock()
	defer b.mu.RUnlock()
	skills := make([]LearnedSkill, 0, len(b.skills))
	for id, skill := range b.skills {
		if skill.passive {
			continue
		}
		skills = append(skills, LearnedSkill{
			SkillID: id,
			Level:   skill.level,
			Passive: false,
		})
	}
	sort.Slice(skills, func(i, j int) bool {
		return skills[i].SkillID < skills[j].SkillID
	})

	return skills
}

// ObjectHealthPercent returns the HP of an observed object as a
// percentage of its maximum (0..100), -1 when the object or its
// vitals are unknown. The server refreshes the vitals of the mob the
// character attacks, so the value is exact where the hunt loop needs
// it: the current fight.
func (b *Bot) ObjectHealthPercent(objectID int32) float64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	_, cold := b.objectLocked(objectID)
	if cold == nil || cold.MaxHP <= 0 {
		return -1
	}
	pct := cold.CurHP / cold.MaxHP * 100

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

// Packets returns the count of packets the session received so far:
// the fleet diagnostics sample it to report the live packet rate.
func (b *Bot) Packets() int64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.packets
}

// Status returns the current session status.
func (b *Bot) Status() Status {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.status
}

// Phase returns the last published hunt loop phase of the bot. It is
// empty before the loop sets it (the manual only sessions and the
// pre-world sessions never set it). The web UI maps it to a human
// readable activity banner.
func (b *Bot) Phase() string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.phase
}

// SetPhase publishes the current hunt loop phase so the web UI can
// show a human readable activity banner (hunting, walking to town,
// selling, deleveling). The hunt loop calls this on every phase
// transition (and a steady state refresh is a no-op so the per tick
// call never churns the event stream).
func (b *Bot) SetPhase(phase string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.phase == phase {
		return
	}
	b.phase = phase
	b.touch()
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

// SetHuntingZone configures the hunting square of the bot for the web
// map display. The zone survives the session resets (it is a policy,
// not an observation).
func (b *Bot) SetHuntingZone(cx int32, cy int32, half int32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.zone = &Zone{CX: cx, CY: cy, Half: half}
	b.touch()
}

// SetHuntingZones publishes the hunting zone registry of the map
// view: every zone of the region with the active marker of the zone
// the loop hunts in. A repeated identical zone set only refreshes the
// timestamp: the hunt loop republishes on every zone state change
// (a zone pick, a death, a demotion), but the 227 zone registry is
// the same on most of those calls, so the defensive copy was pure
// allocation churn (4 MB over a 3 minute fleet run with 100 bots).
func (b *Bot) SetHuntingZones(zones []ZoneView) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if zoneViewsEqual(b.zoneViews, zones) {
		return
	}
	b.zoneViews = make([]ZoneView, len(zones))
	copy(b.zoneViews, zones)
	b.touch()
}

// SetKillMarks publishes the recent kill ring of the hunt loop: the
// ground positions where the bot's targets died. The marks feed the
// fleet wide cross layer of the map (Registry.FleetKillMarks serves
// them for every bot of the process), so the crosses accumulate
// across the bot switches of the web view. A repeated identical set
// only refreshes the timestamp: the hunt loop republishes the ring
// with every spot view refresh.
func (b *Bot) SetKillMarks(marks []KillMarkView) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if killMarksEqual(b.killMarks, marks) {
		return
	}
	b.killMarks = make([]KillMarkView, len(marks))
	copy(b.killMarks, marks)
	b.touch()
}

// KillMarks returns a defensive copy of the recent kill marks of the
// bot (the registry aggregates them for the fleet endpoint).
func (b *Bot) KillMarks() []KillMarkView {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if len(b.killMarks) == 0 {
		return nil
	}
	marks := make([]KillMarkView, len(b.killMarks))
	copy(marks, b.killMarks)

	return marks
}

// killMarksEqual reports whether two kill mark slices are element
// wise equal. The comparison lets SetKillMarks skip the defensive
// copy while the hunt ring holds the same kills.
func killMarksEqual(a, b []KillMarkView) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

// zoneViewsEqual reports whether two zone view slices are element wise
// equal. The comparison is used by SetHuntingZones to skip the
// defensive copy when the published zones have not changed.
func zoneViewsEqual(a, b []ZoneView) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

// SetLoginCooldown arms the login cooldown of the supervisor: an
// emergency logout of the hunt loop asks for a pause before the
// next session starts, so the mobs reset and the character
// regenerates in peace while it runs.
func (b *Bot) SetLoginCooldown(pause time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.loginCooldownUntil = time.Now().Add(pause)
	b.touch()
}

// LoginCooldownRemaining returns the pause left before the next
// login, zero when no cooldown is armed or it already lapsed.
func (b *Bot) LoginCooldownRemaining() time.Duration {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.loginCooldownUntil.IsZero() {
		return 0
	}
	remaining := time.Until(b.loginCooldownUntil)
	if remaining < 0 {
		return 0
	}

	return remaining
}

// ResetSession clears the observed world state before a new login. The
// server starts a fresh known list on every session, so the objects and
// inventory of a lost session must not leak into the next one. The event
// log, the packet counter and the session start time survive so the
// history and the uptime stay continuous across reconnects.
func (b *Bot) ResetSession() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.drainCommands()
	b.selfID = 0
	b.char = newCharacterState()
	b.world = newObjectStore()
	b.inventory = newInventoryStore()
	b.inventoryVersion++
	b.skills = nil
	b.skillsRevision++
	b.skillQueue = nil
	b.skillQueueClass = 0
	b.skillQueueRevision = 0
	b.skillWeapons = nil
	b.buffs = nil
	b.buffsAt = time.Time{}
	b.walkPlan = nil
	b.walkPlanAt = time.Time{}
	b.clearShoppingPlanLocked()
	b.phase = ""
	b.status = StatusConnecting
	b.touch()
}

// walkPlanTTL bounds how long a published walk plan survives
// without a refresh: the hunt loop re-publishes the plan every
// tick while the walk runs, so an expired plan means the
// loop moved on (or died) and the map must stop drawing the line
// and the marker.
const walkPlanTTL = 2 * time.Second

// WalkPoint is one waypoint of the published walk plan: a planned
// geodata waypoint, the planning origin or the final destination of
// the walk.
type WalkPoint struct {
	X int32 `json:"x"`
	Y int32 `json:"y"`
	Z int32 `json:"z"`
}

// WalkPlan is the published walk plan of a running leg: where the
// leg was planned from (Origin), the full waypoint list of the plan
// (Points), the waypoint the follower currently heads to (Index) and
// the final destination of the walk (Dest). The state dump prints
// the whole thing - a stuck trip reads at a glance - and the map
// draws the planned line against the live character position.
type WalkPlan struct {
	// Origin is the position the leg was planned from, nil when the
	// publisher does not know it (the manual direct walks).
	Origin *WalkPoint `json:"origin"`
	// Points lists every planned waypoint of the leg in walk order,
	// including the passed ones.
	Points []WalkPoint `json:"points"`
	// Index is the position in Points the follower currently aims
	// at: the waypoints before it are passed, the rest lies ahead.
	Index int `json:"index"`
	// Dest is the final destination of the walk (the merchant
	// spawn, the farm spot, the clicked point), nil when the plan
	// itself carries it.
	Dest *WalkPoint `json:"dest"`
}

// SetWalkPlan publishes the walk plan of a running leg: the planning
// origin, the full waypoint list, the current target index and the
// final destination. A plan without points clears it. Republishing
// an equal plan only refreshes its lifetime, so the steady per tick
// refresh of the hunt loop never churns the event stream.
func (b *Bot) SetWalkPlan(plan WalkPlan) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(plan.Points) == 0 {
		b.clearWalkPlanLocked()

		return
	}
	if b.walkPlan != nil && walkPlansEqual(*b.walkPlan, plan) {
		b.walkPlanAt = time.Now()

		return
	}
	b.walkPlan = &plan
	b.walkPlanAt = time.Now()
	b.touch()
}

// ClearWalkPlan drops the published walk plan (a no-op when none
// is published).
func (b *Bot) ClearWalkPlan() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clearWalkPlanLocked()
}

// clearWalkPlanLocked drops the walk plan, the caller must hold
// the state write lock.
func (b *Bot) clearWalkPlanLocked() {
	if b.walkPlan == nil {
		return
	}
	b.walkPlan = nil
	b.walkPlanAt = time.Time{}
	b.touch()
}

// walkPlansEqual compares two walk plans field by field, the points
// element wise.
func walkPlansEqual(a, b WalkPlan) bool {
	if (a.Origin == nil) != (b.Origin == nil) ||
		(a.Dest == nil) != (b.Dest == nil) ||
		a.Index != b.Index || len(a.Points) != len(b.Points) {
		return false
	}
	if a.Origin != nil && *a.Origin != *b.Origin {
		return false
	}
	if a.Dest != nil && *a.Dest != *b.Dest {
		return false
	}
	for i := range a.Points {
		if a.Points[i] != b.Points[i] {
			return false
		}
	}

	return true
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
	Name               string
	Level              int32
	Race               int32
	ClassID            int32
	X                  int32
	Y                  int32
	Z                  int32
	STR                int32
	DEX                int32
	CON                int32
	INT                int32
	WIT                int32
	MEN                int32
	Exp                int32
	Sp                 int32
	MaxHP              int32
	CurHP              int32
	MaxMP              int32
	CurMP              int32
	CurrentLoad        int32
	MaxLoad            int32
	RunSpeed           int32
	WalkSpeed          int32
	MoveSpeedMult      float64
	PaperdollObjectIDs [PaperdollSlots]int32
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
	b.paperdoll = info.PaperdollObjectIDs
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
	obj, cold := b.objectLocked(p.ObjectID)
	if obj == nil {
		return
	}
	obj.X = p.X
	obj.Y = p.Y
	obj.Z = p.Z
	cold.Heading = p.Heading
	obj.Moving = p.Moving
	if !p.Moving {
		obj.DestX = p.X
		obj.DestY = p.Y
		obj.DestZ = p.Z
	}
	nowNano := time.Now().UnixNano()
	obj.MoveAt = nowNano
	cold.UpdatedAt = nowNano
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
	obj, cold := b.objectLocked(m.ObjectID)
	if obj == nil {
		return
	}
	obj.X = m.X
	obj.Y = m.Y
	obj.Z = m.Z
	if !arrived {
		cold.Heading = HeadingFromDelta(m.DestX-m.X, m.DestY-m.Y)
	}
	obj.DestX = m.DestX
	obj.DestY = m.DestY
	obj.DestZ = m.DestZ
	obj.Moving = !arrived
	nowNano := time.Now().UnixNano()
	obj.MoveAt = nowNano
	cold.UpdatedAt = nowNano
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
	obj, cold := b.objectLocked(m.ObjectID)
	if obj == nil {
		return
	}
	obj.X = m.X
	obj.Y = m.Y
	obj.Z = m.Z
	obj.DestX = destX
	obj.DestY = destY
	obj.DestZ = m.TargetZ
	if m.X != m.TargetX || m.Y != m.TargetY {
		cold.Heading = HeadingFromDelta(m.TargetX-m.X, m.TargetY-m.Y)
	}
	obj.Moving = true
	obj.Running = true
	obj.TargetID = m.TargetID
	b.markObjectCombatLocked(obj, cold, now)
	obj.MoveAt = now.UnixNano()
	cold.UpdatedAt = now.UnixNano()
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
	b.noteSelfCombatLocked(now)
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
		b.noteSelfCombatLocked(now)
		b.touch()
	} else if obj, cold := b.objectLocked(a.AttackerID); obj != nil {
		obj.X = a.X
		obj.Y = a.Y
		obj.Z = a.Z
		if hasFacing {
			cold.Heading = facing
		}
		if a.TargetCount > 0 {
			obj.TargetID = a.TargetIDs[0]
		}
		b.markObjectCombatLocked(obj, cold, now)
		cold.UpdatedAt = now.UnixNano()
		b.touch()
	}
	b.recordSwingEventsLocked(a, now)
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
		if obj, cold := b.objectLocked(a.TargetIDs[i]); obj != nil {
			b.markObjectCombatLocked(obj, cold, now)
			cold.UpdatedAt = now.UnixNano()
		}
	}
}

// recordSwingEventsLocked feeds the swing animation of the web view
// with one event per hit that actually landed. The Mobius Attack
// packet carries the miss flag of every hit, so an evaded blow draws
// nothing - the streak only plays for the blows that connect, and
// the viewer reads who hit whom instead of a swing storm of dodged
// attacks. The caller must hold the write lock.
func (b *Bot) recordSwingEventsLocked(a Attack, now time.Time) {
	for i := range a.TargetCount {
		if a.HitFlags[i]&attackHitMissFlag != 0 {
			continue
		}
		if a.AttackerID == b.selfID {
			// A landed blow of the played character: the deleveling
			// fight stage reads the pair to tell a guard that ignores
			// real damage from a stage without any landed blow yet.
			b.char.LastLandedHitAt = now
			b.char.LastLandedHitTarget = a.TargetIDs[i]
		}
		//nolint:exhaustruct // the ring assigns Seq, a swing carries no amount
		b.recordCombatEventLocked(CombatEvent{
			Kind:       CombatEventAttack,
			AttackerID: a.AttackerID,
			TargetID:   a.TargetIDs[i],
			X:          a.X,
			Y:          a.Y,
			TargetX:    a.TargetX,
			TargetY:    a.TargetY,
			At:         now,
		})
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
	obj, cold := b.objectLocked(objectID)
	if obj == nil {
		return
	}
	obj.AutoAttacking = true
	b.markObjectCombatLocked(obj, cold, now)
	cold.UpdatedAt = now.UnixNano()
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
	obj, cold := b.objectLocked(objectID)
	if obj == nil {
		return
	}
	obj.AutoAttacking = false
	cold.UpdatedAt = time.Now().UnixNano()
	b.touch()
}

// ApplyNpcInfo upserts an observed npc object.
func (b *Bot) ApplyNpcInfo(info NpcInfo) {
	b.mu.Lock()
	defer b.mu.Unlock()
	obj, cold := b.upsertLocked(info.ObjectID, KindNPC)
	cold.TemplateID = info.TemplateID
	obj.Attackable = info.Attackable
	obj.Aggressive = npcdata.NPCIsAggressive(info.TemplateID)
	cold.AggroRange = npcdata.NPCAggroRange(info.TemplateID)
	obj.Level = npcdata.NPCLevel(info.TemplateID)
	obj.ClanHelpRange = npcdata.NPCClanHelpRange(info.TemplateID)
	obj.ClanMask = npcdata.NPCClanMask(info.TemplateID)
	obj.X = info.X
	obj.Y = info.Y
	obj.Z = info.Z
	cold.Heading = info.Heading
	obj.RunSpeed = info.RunSpeed
	obj.WalkSpeed = info.WalkSpeed
	obj.MoveSpeedMult = info.MoveSpeedMult
	cold.CollisionRadius = info.CollisionRadius
	obj.Running = info.Running
	obj.Moving = false
	obj.DestX = info.X
	obj.DestY = info.Y
	obj.DestZ = info.Z
	obj.Dead = info.Dead
	cold.Name = resolveNpcName(info.Name, info.TemplateID)
	cold.Title = info.Title
	now := time.Now()
	if info.InCombat {
		b.markObjectCombatLocked(obj, cold, now)
	}
	obj.MoveAt = now.UnixNano()
	cold.UpdatedAt = now.UnixNano()
	b.touch()
	b.recordLocked("npc spawned: " + cold.Name)
}

// ApplyPlayerInfo upserts an observed player object.
func (b *Bot) ApplyPlayerInfo(info PlayerInfo) {
	b.mu.Lock()
	defer b.mu.Unlock()
	obj, cold := b.upsertLocked(info.ObjectID, KindPlayer)
	cold.Name = info.Name
	cold.Title = info.Title
	obj.X = info.X
	obj.Y = info.Y
	obj.Z = info.Z
	obj.RunSpeed = info.RunSpeed
	obj.WalkSpeed = info.WalkSpeed
	obj.MoveSpeedMult = info.MoveSpeedMult
	cold.CollisionRadius = info.CollisionRadius
	obj.Running = info.Running
	obj.Dead = info.Dead
	cold.Sitting = info.Sitting
	obj.Moving = false
	obj.DestX = info.X
	obj.DestY = info.Y
	obj.DestZ = info.Z
	now := time.Now()
	if info.InCombat {
		b.markObjectCombatLocked(obj, cold, now)
	}
	obj.MoveAt = now.UnixNano()
	cold.UpdatedAt = now.UnixNano()
	b.touch()
	b.recordLocked("player appeared: " + info.Name)
}

// ApplyItemInfo upserts an observed ground item object.
func (b *Bot) ApplyItemInfo(info ItemInfo) {
	b.mu.Lock()
	defer b.mu.Unlock()
	obj, cold := b.upsertLocked(info.ObjectID, KindItem)
	cold.TemplateID = info.TemplateID
	cold.Name = npcdata.ItemName(info.TemplateID)
	cold.Count = info.Count
	obj.X = info.X
	obj.Y = info.Y
	obj.Z = info.Z
	cold.UpdatedAt = time.Now().UnixNano()
	b.touch()
	b.recordLocked("item dropped: " + itemName(cold.Name, info.TemplateID))
}

// RemoveObject deletes an object that left the known list. When the
// bot itself targeted the object, the target is dropped as well: the
// server answers the removal only with the DeleteObject broadcast.
func (b *Bot) RemoveObject(objectID int32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, cold := b.world.lookupLocked(objectID)
	if cold == nil {
		return
	}
	name := cold.Name
	b.removeObjectAtLocked(b.world.slotLocked(objectID), objectID)
	if b.char.TargetID == objectID {
		b.clearSelfTargetLocked("target object removed")
	}
	b.touch()
	b.recordLocked("object removed: " + name)
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
	now := time.Now()
	if objectID == b.charObjectID() {
		b.recordCharDamageLocked(attrs, now)
		for _, attr := range attrs {
			b.applyCharAttr(attr)
		}
		b.touch()

		return
	}
	obj, cold := b.objectLocked(objectID)
	if obj == nil {
		return
	}
	b.recordObjectDamageLocked(obj, cold, objectID, attrs, now)
	for _, attr := range attrs {
		switch attr.ID {
		case AttrCurHP:
			cold.CurHP = float64(attr.Value)
			obj.Dead = attr.Value <= 0
		case AttrMaxHP:
			cold.MaxHP = float64(attr.Value)
		case AttrCurMP:
			cold.CurMP = float64(attr.Value)
		case AttrMaxMP:
			cold.MaxMP = float64(attr.Value)
		}
	}
	cold.UpdatedAt = time.Now().UnixNano()
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

// NewestEvents returns up to limit newest events of the rolling log
// in chronological order. The state dump reads a deeper window than
// the snapshot carries (the web UI log tail keeps the last entries
// only, the debug dump wants the whole story of the session).
func (b *Bot) NewestEvents(limit int) []Event {
	b.mu.RLock()
	defer b.mu.RUnlock()
	events := make([]Event, 0, min(b.log.length, limit))

	return b.log.appendNewest(events, limit)
}

// upsertLocked returns the pointers to the hot and cold records of
// the object id or appends fresh ones (see objectStore.upsertLocked).
// The caller must hold the write lock.
func (b *Bot) upsertLocked(
	objectID int32, kind ObjectKind,
) (*objectHot, *objectCold) {
	return b.world.upsertLocked(objectID, kindCode(kind))
}

// objectLocked returns the pointers to the hot and cold records of
// the object id, both nil when the id is unknown. The caller must
// hold a lock.
func (b *Bot) objectLocked(objectID int32) (*objectHot, *objectCold) {
	return b.world.lookupLocked(objectID)
}

// removeObjectAtLocked frees a slot of the dense object array (see
// objectStore.removeAtLocked). The caller must hold the write lock.
func (b *Bot) removeObjectAtLocked(slot int32, objectID int32) {
	b.world.removeAtLocked(slot, objectID)
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

// recordLocked appends an event to the rolling log (see eventLog).
// The caller must hold the write lock.
func (b *Bot) recordLocked(message string) {
	b.log.record(message, time.Now())
}

// recordCombatEventLocked appends one combat animation beat (see
// combatFeed.record). The caller must hold the state write lock.
func (b *Bot) recordCombatEventLocked(e CombatEvent) {
	b.combat.record(e, time.Now())
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
	SocialUntilMs   int64   `json:"socialUntilMs"`
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

// InventoryItemSnapshot is the JSON view of one inventory item of the
// equipment widget. BodyPart is the slot mask of the item template
// (0x80 right hand, 0x400 chest and so on), Icon the file name inside
// data/icons and Name the resolved display name; both are empty for
// unknown items.
//
// The Type, WeaponType, ArmorType, PAtk, MAtk, PDef, MDef, SDef, RShld,
// PAtkSpd, SoulShots, SpiritShots, Weight and Price fields drive the
// multi line item status tooltip of the equipment widget (one stat
// line per family: weapon, armor, jewelry, etc). They stay zero / empty
// when the item carries no value of that field, so the tooltip renders
// only the lines the item actually has.
type InventoryItemSnapshot struct {
	ObjectID    int32  `json:"objectId"`
	ItemID      int32  `json:"itemId"`
	Count       int32  `json:"count"`
	Type2       int16  `json:"type2"`
	Equipped    bool   `json:"equipped"`
	BodyPart    int32  `json:"bodyPart"`
	Enchant     int16  `json:"enchant"`
	Name        string `json:"name"`
	Icon        string `json:"icon"`
	Type        string `json:"type"`
	WeaponType  string `json:"weaponType"`
	ArmorType   string `json:"armorType"`
	BodyPartKey string `json:"bodyPartKey"`
	PAtk        int32  `json:"pAtk"`
	MAtk        int32  `json:"mAtk"`
	PDef        int32  `json:"pDef"`
	MDef        int32  `json:"mDef"`
	SDef        int32  `json:"sDef"`
	RShld       int32  `json:"rShld"`
	PAtkSpd     int32  `json:"pAtkSpd"`
	SoulShots   int32  `json:"soulShots"`
	SpiritShots int32  `json:"spiritShots"`
	Weight      int32  `json:"weight"`
	Price       int64  `json:"price"`
}

// ObjectSnapshot is the JSON view of a world object.
type ObjectSnapshot struct {
	ObjectID   int32      `json:"objectId"`
	Kind       ObjectKind `json:"kind"`
	Name       string     `json:"name"`
	Title      string     `json:"title"`
	TemplateID int32      `json:"templateId"`
	Attackable bool       `json:"attackable"`
	Aggressive bool       `json:"aggressive"`
	AggroRange int32      `json:"aggroRange"`
	// ClanHelpRange is the distance within the attacked npc calls
	// its clan mates to help (0 for loners): the map draws the
	// social links between clan mates inside it.
	ClanHelpRange int32 `json:"clanHelpRange"`
	// ClanMask is the clan bitmask of the npc as a decimal string
	// (see npcdata.NPCClanMask: the low bits are the clan alphabet,
	// the top bit marks the ALL clan that matches everything). A
	// string because the ALL bit exceeds the safe integer range of
	// JavaScript; the map parses it with BigInt for the pairwise
	// link test.
	ClanMask        string  `json:"clanMask"`
	Level           int32   `json:"level"`
	TargetID        int32   `json:"targetId"`
	InCombat        bool    `json:"inCombat"`
	Dead            bool    `json:"dead"`
	Sitting         bool    `json:"sitting"`
	Moving          bool    `json:"moving"`
	Running         bool    `json:"running"`
	Speed           float64 `json:"speed"`
	CollisionRadius float64 `json:"collisionRadius"`
	SocialUntilMs   int64   `json:"socialUntilMs"`
	Count           int32   `json:"count"`
	X               int32   `json:"x"`
	Y               int32   `json:"y"`
	Z               int32   `json:"z"`
	Heading         int32   `json:"heading"`
	DestX           int32   `json:"destX"`
	DestY           int32   `json:"destY"`
	DestZ           int32   `json:"destZ"`
	MoveAtMs        int64   `json:"moveAtMs"`
	CurHP           float64 `json:"curHp"`
	MaxHP           float64 `json:"maxHp"`
	CurMP           float64 `json:"curMp"`
	MaxMP           float64 `json:"maxMp"`
}

// CombatEventView is the JSON view of one combat animation beat
// (see CombatEvent).
type CombatEventView struct {
	Seq        uint64  `json:"seq"`
	Kind       string  `json:"kind"`
	AttackerID int32   `json:"attackerId"`
	TargetID   int32   `json:"targetId"`
	Amount     float64 `json:"amount"`
	AtMs       int64   `json:"atMs"`
	X          int32   `json:"x"`
	Y          int32   `json:"y"`
	TargetX    int32   `json:"targetX"`
	TargetY    int32   `json:"targetY"`
}

// Snapshot is the JSON view of the whole bot state.
type Snapshot struct {
	ID     string `json:"id"`
	Status Status `json:"status"`
	// Phase is the last published hunt loop phase (hunt.HuntPhase*
	// string values: engage, loot, townWalk, townSell, townReturn,
	// delevel, user, idle). The web UI maps it to the human readable
	// activity banner of the bot widget. Empty before the loop sets
	// it (the manual only sessions never do).
	Phase     string                  `json:"phase"`
	Character CharacterSnapshot       `json:"character"`
	Inventory []InventoryItemSnapshot `json:"inventory"`
	Objects   []ObjectSnapshot        `json:"objects"`
	Events    []Event                 `json:"events"`
	Chat      []ChatEvent             `json:"chat"`
	WalkPath  []WalkPoint             `json:"walkPath"`
	// WalkOrigin is the position the published leg was planned
	// from (the "where we wanted to go from" of the dump), null
	// when the publisher carries no origin.
	WalkOrigin *WalkPoint `json:"walkOrigin"`
	// WalkIndex is the position in WalkPath the follower
	// currently aims at: the waypoints before it are passed, the
	// rest lies ahead.
	WalkIndex int `json:"walkIndex"`
	// WalkDest is the final destination of the published walk,
	// null when the plan itself carries it.
	WalkDest *WalkPoint `json:"walkDest"`
	// Shopping carries the published purchase queue of the shop
	// strategy (see SetShoppingPlan): what the bot plans to buy next
	// with the prices and the missing adena, null when nothing is
	// published or the plan expired.
	Shopping *ShoppingPlanView `json:"shopping"`
	// Skills carries the learned skill list of the server SkillList
	// packets enriched with the display data of the generated
	// dictionary (name, icon, passive flag).
	Skills []SkillSnapshot `json:"skills"`
	// SkillPlan carries the learning queue of the class tree (see
	// skillPlanViewLocked): the remaining lessons in the planned
	// order with the SP costs, null when the class is unknown or
	// nothing is left to learn.
	SkillPlan *SkillPlanView `json:"skillPlan"`
	// Buffs carries the active effect list of the character (see
	// SetBuffs): the running buffs with their levels and remaining
	// seconds, resolved with the display data of the generated
	// dictionary. The web UI buffs widget renders it.
	Buffs []BuffSnapshot `json:"buffs"`
	// CombatEvents carries the recent swings and damage
	// landings of the animation layer: the last
	// combatEventTTL window, in chronological order,
	// deduped by the client on the sequence.
	CombatEvents []CombatEventView `json:"combatEvents"`
	HuntingZone  *Zone             `json:"huntingZone"`
	HuntingZones []ZoneView        `json:"huntingZones"`
	Packets      int64             `json:"packets"`
	Version      uint64            `json:"version"`
	ServerTimeMs int64             `json:"serverTimeMs"`
	StartedAt    time.Time         `json:"startedAt"`
	UpdatedAt    time.Time         `json:"updatedAt"`
}

// ZoneView is one hunting ground of the map view: the registry entry
// of the deployment with the active marker of the ground the loop
// hunts in, the death bookkeeping of the session and - for the spot
// registries - the respawn window, the expected population, the
// measured income rate and the next respawn prediction of the spot
// economy. The legacy square entries leave the spot fields zero and
// the kind empty.
type ZoneView struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Region   string `json:"region"`
	MinLevel int32  `json:"minLevel"`
	MaxLevel int32  `json:"maxLevel"`
	MinGear  int32  `json:"minGear"`
	CX       int32  `json:"cx"`
	CY       int32  `json:"cy"`
	Half     int32  `json:"half"`
	Active   bool   `json:"active"`
	Deaths   int32  `json:"deaths"`
	Demoted  bool   `json:"demoted"`
	// Kind marks the registry shape: "spot" for the spot anchored
	// registry (circles on the map), empty for the legacy squares.
	Kind string `json:"kind"`
	// Radius is the spot circle (the leash square Half inscribes
	// in it); zero for the legacy squares.
	Radius int32 `json:"radius"`
	// RespawnMinSec and RespawnMaxSec bound the respawn window of
	// the ground.
	RespawnMinSec int32 `json:"respawnMinSec"`
	RespawnMaxSec int32 `json:"respawnMaxSec"`
	// SpawnMass is the expected population of the spot ground.
	SpawnMass int32 `json:"spawnMass"`
	// AggroMass is the expected aggressive population (the static
	// danger input of the spot safety).
	AggroMass int32 `json:"aggroMass"`
	// AdenaPerMin is the measured income rate of the session at
	// the ground (the bootstrap prior stays out of the view - the
	// measurement replaces it silently).
	AdenaPerMin float64 `json:"adenaPerMin"`
	// DeathHeat is the decayed death heat of the spot (the safety
	// multiplier input, see the spot metrics).
	DeathHeat float64 `json:"deathHeat"`
	// NextRespawnSec is the predicted ETA of the earliest respawn
	// of the spot overlay, -1 when the overlay holds no pending
	// prediction.
	NextRespawnSec int32 `json:"nextRespawnSec"`
	// Occupancy is the number of hunters of the fleet holding the
	// spot right now.
	Occupancy int32 `json:"occupancy"`
	// KillX and KillY are the live kill centroid of the session at
	// the spot (the EMA of the kill positions, zero before the
	// first kill).
	KillX int32 `json:"killX"`
	KillY int32 `json:"killY"`
}

// KillMarkView is one recent kill of the fleet kill ring: the ground
// position where a bot's target died with the time of the kill. The
// web map draws the marks as the crosses of the whole deployment -
// they accumulate across the bots (every kill of every bot stays
// visible) instead of the per zone centroid that the observed bot
// alone carries.
type KillMarkView struct {
	// BotID names the bot that landed the kill (the tooltip of
	// the cross).
	BotID string `json:"botId"`
	X     int32  `json:"x"`
	Y     int32  `json:"y"`
	AtMs  int64  `json:"atMs"`
}

// SelfSnapshot returns the live character view of the played character
// alone (no world, inventory or event copies). The proxy uses it to
// patch the entering world packets of a connecting client with the
// position, vitals and level the bot has right now, so the reconnection
// replay never spawns the client at a stale login-time place.
func (b *Bot) SelfSnapshot() CharacterSnapshot {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.characterSnapshotLocked(time.Now(), 0, len(b.inventory.items))
}

// Snapshot returns a deep copy of the current state for serialization.
// The full world copy of the web view; the section split is planned
// (docs/quality_review_and_agent_prompts.md P11).
func (b *Bot) Snapshot() Snapshot { //nolint:funlen
	b.mu.RLock()
	defer b.mu.RUnlock()
	now := time.Now()

	snap := Snapshot{
		ID:     b.id,
		Status: b.status,
		Phase:  b.phase,
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
			SocialUntilMs:   b.char.SocialUntil.UnixMilli(),
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
			Sp:             b.char.Sp,
			InCombat:       b.char.inCombat(now),
			CurrentLoad:    0,
			MaxLoad:        0,
			InventorySlots: 0,
			InventoryMax:   0,
			Adena:          0,
		},
		Inventory:    nil,
		Objects:      make([]ObjectSnapshot, 0, len(b.world.hot)),
		CombatEvents: make([]CombatEventView, 0, len(b.combat.events)),
		Events:       make([]Event, 0, min(b.log.length, snapshotEvents)),
		Chat:         make([]ChatEvent, 0, b.chat.length),
		WalkPath:     nil,
		WalkOrigin:   nil,
		WalkIndex:    0,
		WalkDest:     nil,
		Shopping:     nil,
		Skills:       nil,
		SkillPlan:    nil,
		Buffs:        nil,
		HuntingZone:  nil,
		HuntingZones: nil,
		Packets:      b.packets,
		Version:      b.version,
		ServerTimeMs: now.UnixMilli(),
		StartedAt:    b.started,
		UpdatedAt:    b.updated,
	}
	if b.walkPlan != nil && time.Since(b.walkPlanAt) <= walkPlanTTL {
		snap.WalkPath = make([]WalkPoint, len(b.walkPlan.Points))
		copy(snap.WalkPath, b.walkPlan.Points)
		snap.WalkOrigin = b.walkPlan.Origin
		snap.WalkIndex = b.walkPlan.Index
		snap.WalkDest = b.walkPlan.Dest
	}
	if b.shoppingPlanLive(now) {
		entries := make([]ShoppingEntryView, len(b.shopping.Entries))
		copy(entries, b.shopping.Entries)
		snap.Shopping = &ShoppingPlanView{
			Entries: entries,
			Adena:   b.shopping.Adena,
			Total:   b.shopping.Total,
			Trip:    b.shopping.Trip,
		}
	}
	snap.Skills = b.skillSnapshotsLocked()
	snap.SkillPlan = b.skillPlanViewLocked()
	snap.Buffs = b.buffSnapshotsLocked(now)
	nowNano := now.UnixNano()
	for i := range b.world.hot {
		snap.Objects = append(snap.Objects,
			b.objectSnapshotLocked(i, nowNano))
	}
	snap.CombatEvents = b.combat.appendView(snap.CombatEvents, now)
	snap.Events = b.log.appendNewest(snap.Events, snapshotEvents)
	snap.Chat = b.chat.appendAll(snap.Chat)
	snap.HuntingZone = b.zone
	snap.HuntingZones = make([]ZoneView, len(b.zoneViews))
	copy(snap.HuntingZones, b.zoneViews)
	b.fillInventorySnapshot(&snap)

	return snap
}

// fillInventorySnapshot completes the character view with the inventory
// usage and builds the item list of the equipment widget: every entry
// carries the resolved display name, the icon file name of the icon
// pack and the paperdoll slot mask. The dense store keeps the records
// in the canonical widget order (see inventoryStore), so the fill is a
// straight sequential walk with no read time sort. The caller must
// hold the read lock.
func (b *Bot) fillInventorySnapshot(snap *Snapshot) {
	snap.Character.CurrentLoad = b.char.CurrentLoad
	snap.Character.MaxLoad = b.char.MaxLoad
	snap.Character.InventorySlots = len(b.inventory.items)
	snap.Character.InventoryMax = inventorySlotLimit
	snap.Inventory = make([]InventoryItemSnapshot, 0, len(b.inventory.items))
	for _, item := range b.inventory.items {
		stats, hasStats := npcdata.ItemGearStats(item.ItemID)
		itemType := stats.Type
		if !hasStats {
			itemType = npcdata.ItemType(item.ItemID)
		}
		snap.Inventory = append(snap.Inventory, InventoryItemSnapshot{
			ObjectID:    item.ObjectID,
			ItemID:      item.ItemID,
			Count:       item.Count,
			Type2:       item.Type2,
			Equipped:    item.Equipped,
			BodyPart:    item.BodyPart,
			Enchant:     item.Enchant,
			Name:        npcdata.ItemName(item.ItemID),
			Icon:        npcdata.ItemIcon(item.ItemID),
			Type:        itemType,
			WeaponType:  stats.WeaponType,
			ArmorType:   stats.ArmorType,
			BodyPartKey: stats.BodyPart,
			PAtk:        stats.PAtk,
			MAtk:        stats.MAtk,
			PDef:        stats.PDef,
			MDef:        stats.MDef,
			SDef:        stats.SDef,
			RShld:       stats.RShld,
			PAtkSpd:     stats.PAtkSpd,
			SoulShots:   stats.SoulShots,
			SpiritShots: stats.SpiritShots,
			Weight:      npcdata.ItemWeight(item.ItemID),
			Price:       npcdata.ItemPrice(item.ItemID),
		})
		if item.Type2 == itemType2Adena {
			snap.Character.Adena += item.Count
		}
	}
}

// compareInventoryItems orders two widget entries: the equipped gear
// first, then the item id, the object id breaks the ties. It doubles as
// the canonical order comparator of the dense inventory store.
func compareInventoryItems(
	a InventoryItem, b InventoryItem,
) int {
	if a.Equipped != b.Equipped {
		if a.Equipped {
			return -1
		}

		return 1
	}
	if a.ItemID != b.ItemID {
		if a.ItemID < b.ItemID {
			return -1
		}

		return 1
	}
	if a.ObjectID != b.ObjectID {
		if a.ObjectID < b.ObjectID {
			return -1
		}

		return 1
	}

	return 0
}

// BotInfo is the compact JSON view used by the bot list endpoint. The
// vitals and the combat flag let the sidebar show the mini HP/MP/XP bars
// and the fighting state of every session at a glance.
type BotInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status Status `json:"status"`
	// Phase mirrors the hunt loop phase so the sidebar bot row can
	// show the same activity banner as the map HUD. Empty for the
	// manual only sessions and the pre-world sessions.
	Phase      string  `json:"phase"`
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
		Phase:      b.phase,
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
