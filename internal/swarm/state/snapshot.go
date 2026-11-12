// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
    "time"

    "github.com/melg8/swarm/internal/swarm/npcdata"
)

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
    Crit       bool    `json:"crit"`
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
    // WalkOrigin is the position the published segment was planned
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
    // WalkSearch is the mesh search contract behind the published
    // walk (the repro contract of the 3D pathfind link), null when
    // the plan came from no mesh search.
    WalkSearch *WalkSearch `json:"walkSearch"`
    // LastWalkPath carries the most recent published walk plan after
    // its walk ended (see state lastWalkPlan): the report of a stuck
    // segment needs the whole planned walk even when the live plan is
    // already gone. The fields ride the Go side dump only, the wire
    // payload of the snapshot stays byte identical (the json "-").
    LastWalkPath   []WalkPoint `json:"-"`
    LastWalkOrigin *WalkPoint  `json:"-"`
    LastWalkIndex  int         `json:"-"`
    LastWalkDest   *WalkPoint  `json:"-"`
    LastWalkSearch *WalkSearch `json:"-"`
    LastWalkAt     time.Time   `json:"-"`
    // WalkStart is the moment the published walk was first seen
    // (the zero point of the waypoint timing) and WalkWpAt holds
    // the observed arrival time of every waypoint (the entry i is
    // set when the follower cursor moved past i). WalkAt is the
    // last plan refresh (the moment the walk was last seen alive).
    // The timing view rides the Go side dump only (the json "-",
    // see LastWalkPath); the last walk record carries its own copy
    // in LastWalkStart and LastWalkWpAt.
    WalkStart     time.Time   `json:"-"`
    WalkWpAt      []time.Time `json:"-"`
    WalkAt        time.Time   `json:"-"`
    LastWalkStart time.Time   `json:"-"`
    LastWalkWpAt  []time.Time `json:"-"`
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
    // SkillStates carries the live cast and reuse windows of the
    // learned skills (see ApplySkillCast): the milliseconds left of
    // the cast and the cooldown at the snapshot moment plus the
    // window totals. The web UI cast fills and the cooldown
    // countdowns of the skills widget render it; the map cast icon
    // reads the running cast from it.
    SkillStates []SkillStateView `json:"skillStates"`
    // CombatEvents carries the recent swings and damage
    // landings of the animation layer: the last
    // combatEventTTL window, in chronological order,
    // deduped by the client on the sequence.
    CombatEvents []CombatEventView `json:"combatEvents"`
    HuntingZone  *Zone             `json:"huntingZone"`
    HuntingZones []ZoneView        `json:"huntingZones"`
    // HuntMesh is the version of the installed Voronoi hunt mesh
    // (empty when no cell-mode registry is installed), HuntCell the
    // live record of the cell the loop holds (nil when none).
    HuntMesh     string        `json:"huntMesh"`
    HuntCell     *CellLiveView `json:"huntCell"`
    Packets      int64         `json:"packets"`
    Version      uint64        `json:"version"`
    ServerTimeMs int64         `json:"serverTimeMs"`
    StartedAt    time.Time     `json:"startedAt"`
    UpdatedAt    time.Time     `json:"updatedAt"`
    // ActionFailedAt is the arrival time of the last ActionFailed
    // (0x35) refusal answer of the server (the zero time when none
    // arrived): the dump prints its age next to the session header
    // so a server that refuses the move requests names itself in
    // the report (see ApplyActionFailed). The JSON tag stays "-":
    // the field feeds the Go side dump only, the wire payload of
    // the snapshot stays byte identical.
    ActionFailedAt time.Time `json:"-"`
    // Diagnostics is the health view of the live state: the
    // liveness ages and rates, the combat nuance, the known
    // list summary and the hunt loop internals (see
    // Diagnostics). The section rides the end of the snapshot,
    // after the world data it summarizes.
    Diagnostics Diagnostics `json:"diagnostics"`
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
    // the skull).
    BotID string `json:"botId"`
    X     int32  `json:"x"`
    Y     int32  `json:"y"`
    AtMs  int64  `json:"atMs"`
    // Name and Level carry the victim the tooltip shows (the mob
    // display name and its level, captured at the kill; the level is
    // 0 when the corpse left the knownlist before the record).
    Name  string `json:"name"`
    Level int32  `json:"level"`
}

// SelfSnapshot returns the live character view of the played character
// alone (no world, inventory or event copies). The proxy uses it to
// patch the entering world packets of a connecting client with the
// position, vitals and level the bot has right now, so the reconnection
// replay never spawns the client at a stale login-time place. The
// adena sum and the slot count come from the inventory store, so the
// statistics sampler and the proxy read the real wallet, not a zero
// placeholder.
func (b *Bot) SelfSnapshot() CharacterSnapshot {
    b.mu.RLock()
    defer b.mu.RUnlock()

    adena, slots := b.inventoryTotalsLocked()

    return b.characterSnapshotLocked(time.Now(), adena, slots)
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
        Inventory:      nil,
        Objects:        make([]ObjectSnapshot, 0, len(b.world.hot)),
        CombatEvents:   make([]CombatEventView, 0, len(b.combat.events)),
        Events:         make([]Event, 0, min(b.log.length, snapshotEvents)),
        Chat:           make([]ChatEvent, 0, b.chat.length),
        WalkPath:       nil,
        WalkOrigin:     nil,
        WalkIndex:      0,
        WalkDest:       nil,
        LastWalkPath:   nil,
        LastWalkOrigin: nil,
        LastWalkIndex:  0,
        LastWalkDest:   nil,
        LastWalkAt:     time.Time{},
        WalkStart:      time.Time{},
        WalkWpAt:       nil,
        WalkAt:         time.Time{},
        LastWalkStart:  time.Time{},
        LastWalkWpAt:   nil,
        Shopping:       nil,
        Skills:         nil,
        SkillPlan:      nil,
        Buffs:          nil,
        HuntingZone:    nil,
        HuntingZones:   nil,
        Packets:        b.packets,
        Version:        b.version,
        ServerTimeMs:   now.UnixMilli(),
        StartedAt:      b.started,
        UpdatedAt:      b.updated,
        ActionFailedAt: b.actionFailedAt,
        Diagnostics:    Diagnostics{}, //nolint:exhaustruct_v5 // filled below
    }
    if b.walkPlan != nil && time.Since(b.walkPlanAt) <= walkPlanTTL {
        snap.WalkPath = make([]WalkPoint, len(b.walkPlan.Points))
        copy(snap.WalkPath, b.walkPlan.Points)
        snap.WalkOrigin = b.walkPlan.Origin
        snap.WalkIndex = b.walkPlan.Index
        snap.WalkDest = b.walkPlan.Dest
        snap.WalkSearch = b.walkPlan.Search
        snap.WalkStart = b.walkPlanStart
        snap.WalkAt = b.walkPlanAt
        snap.WalkWpAt = make([]time.Time, len(b.walkWpAt))
        copy(snap.WalkWpAt, b.walkWpAt)
    }
    if b.lastWalkPlan != nil {
        snap.LastWalkPath = make([]WalkPoint,
            len(b.lastWalkPlan.Points))
        copy(snap.LastWalkPath, b.lastWalkPlan.Points)
        snap.LastWalkOrigin = b.lastWalkPlan.Origin
        snap.LastWalkIndex = b.lastWalkPlan.Index
        snap.LastWalkDest = b.lastWalkPlan.Dest
        snap.LastWalkSearch = b.lastWalkPlan.Search
        snap.LastWalkAt = b.lastWalkPlanAt
        snap.LastWalkStart = b.lastWalkStart
        snap.LastWalkWpAt = make([]time.Time, len(b.lastWalkWpAt))
        copy(snap.LastWalkWpAt, b.lastWalkWpAt)
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
    snap.SkillStates = b.skillStateViewsLocked(now)
    nowNano := now.UnixNano()
    var counts worldCounts
    for i := range b.world.hot {
        snap.Objects = append(snap.Objects,
            b.objectSnapshotLocked(i, nowNano))
        counts.note(&b.world.hot[i], b.selfID)
    }
    snap.CombatEvents = b.combat.appendView(snap.CombatEvents, now)
    snap.Events = b.log.appendNewest(snap.Events, snapshotEvents)
    snap.Chat = b.chat.appendAll(snap.Chat)
    snap.HuntingZone = b.zone
    snap.HuntMesh = b.huntMeshVersion
    if b.huntCell.ID != "" {
        liveCopy := b.huntCell
        snap.HuntCell = &liveCopy
    }
    snap.HuntingZones = make([]ZoneView, len(b.zoneViews))
    copy(snap.HuntingZones, b.zoneViews)
    b.fillInventorySnapshot(&snap)
    snap.Diagnostics = b.diagnosticsLocked(now, counts)

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
