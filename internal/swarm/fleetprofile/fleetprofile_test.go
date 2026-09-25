// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Package fleetprofile drives the production hunt fleet (40 bots)
// through the real Run cadence and captures the CPU and allocation
// profiles of the window: the scalability instrument the owner asked
// for - what takes the most time, where the unnecessary allocations
// come from, where the memory sits. The suite needs the explicit opt
// in (the run holds wall clock seconds for the profile window):
//
//    SWARM_FLEET_PROFILE=40 go test ./internal/swarm/fleetprofile/ \
//        -v -timeout 15m
package fleetprofile

import (
    "context"
    "fmt"
    "os"
    "path/filepath"
    "runtime"
    "runtime/pprof"
    "sync"
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/gear"
    "github.com/melg8/swarm/internal/swarm/hunt"
    "github.com/melg8/swarm/internal/swarm/session"
    "github.com/melg8/swarm/internal/swarm/state"
)

// sinkGame implements hunt.GameAPI without recording: the fleet
// profile measures the decision path and the trackers, not the
// bookkeeping of the test doubles.
type sinkGame struct{}

func (s *sinkGame) AttackTarget(_ int32) error          { return nil }
func (s *sinkGame) WalkTo(_, _, _ int32) error          { return nil }
func (s *sinkGame) AbuseMovementEnabled() bool          { return false }
func (s *sinkGame) CursorKeyWalkTo(_, _, _ int32) error { return nil }
func (s *sinkGame) ClickWalkTo(_, _, _ int32) error     { return nil }
func (s *sinkGame) ClaimValidatePosition(_, _, _, _ int32) error {
    return nil
}
func (s *sinkGame) PickupItem(_ state.LootItem) error { return nil }
func (s *sinkGame) ActionSitStand() error             { return nil }
func (s *sinkGame) RestartAtVillage() error           { return nil }
func (s *sinkGame) DestroyItem(_, _ int32) error      { return nil }
func (s *sinkGame) SellItems(_ []state.InventoryItem) error {
    return nil
}
func (s *sinkGame) BuyItems(_ int32, _ []gear.Purchase) error {
    return nil
}
func (s *sinkGame) UseItem(_ int32) error                 { return nil }
func (s *sinkGame) ClickObject(_ int32) error             { return nil }
func (s *sinkGame) InteractPull(_ int32) error            { return nil }
func (s *sinkGame) ClearTarget() error                    { return nil }
func (s *sinkGame) AcquireSkill(_, _ int32) error         { return nil }
func (s *sinkGame) UseMagicSkill(_ int32) error           { return nil }
func (s *sinkGame) DropItem(_, _, _, _, _ int32) error    { return nil }
func (s *sinkGame) RequestLogout() error                  { return nil }
func (s *sinkGame) SendBypass(_ string) error             { return nil }
func (s *sinkGame) Say(_ string, _ int32, _ string) error { return nil }
func (s *sinkGame) LastHTMLDialog() (int32, string) {
    return 0, ""
}

func (s *sinkGame) LastHTMLDialogArrival() (int32, string, uint64) {
    return 0, "", 0
}

// fleetBot mirrors the live session shape: the loop drives itself on
// the production ticker (hunt.Loop.Run), the churn goroutine applies
// the packet load the connection reader would (the knownlist
// movements, the self sync, the status, the enter/exit churn, the
// combat broadcasts).
type fleetBot struct {
    bot   *state.Bot
    loop  *hunt.Loop
    game  *sinkGame
    mobs  []int32
    seed  int64
    ticks int64
}

// newFleetBot builds one fleet member: the character, the paperdoll,
// a 30 item bag, the learned skills and a 60 npc knownlist (the mob
// mix of the farm zone plus the folk of the village edge). The
// journal is process wide (one file, every bot writes into it), the
// caller installs it on every loop.
func newFleetBot(index int) *fleetBot {
    bot := state.NewBot(fmt.Sprintf("temp%d", index+1))
    bot.SetCharacter(fmt.Sprintf("test%d", index+1), int32(100+index),
        18, 45000, 50000, -3500, 120, 80)
    bot.ApplyStatusUpdate(100, []state.Attribute{
        {ID: state.AttrMaxHP, Value: 120},
        {ID: state.AttrCurHP, Value: 110},
    })
    bot.ApplyUserInfo(state.UserInfo{
        Name: fmt.Sprintf("test%d", index+1), Level: 14, ClassID: 18,
        X: 45000, Y: 50000, Z: -3500,
        Exp: 150000, Sp: 3000, MaxHP: 120, CurHP: 110,
        MaxMP: 60, CurMP: 55, RunSpeed: 126,
    })
    items := make([]state.InventoryItem, 0, 32)
    items = append(items, state.InventoryItem{
        ObjectID: int32(200000 + index*100), ItemID: 57,
        Count: 48250, Type2: 4,
    })
    items = append(items, state.InventoryItem{
        ObjectID: int32(200001 + index*100), ItemID: 1,
        Count: 1, Equipped: true,
    })
    for slot := 2; slot <= 30; slot++ {
        items = append(items, state.InventoryItem{
            ObjectID: int32(200000 + index*100 + slot),
            ItemID:   int32(100 + slot),
            Count:    int32(slot),
            Type2:    5,
        })
    }
    bot.ApplyItemList(items)
    paperdoll := [state.PaperdollSlots]int32{}
    paperdoll[state.PaperdollRHand] = int32(200001 + index*100)
    bot.ApplyPaperdoll(paperdoll)
    bot.SetSkills([]state.LearnedSkill{
        {SkillID: 194, Level: 1, Passive: true},
        {SkillID: 16, Level: 3},
    })

    b := &fleetBot{bot: bot, game: &sinkGame{}, seed: int64(index) + 1}
    for i := range 48 {
        objectID := int32(1_000_000 + index*10_000 + i)
        bot.ApplyNpcInfo(state.NpcInfo{
            ObjectID:   objectID,
            TemplateID: 1000003,
            Attackable: true,
            X:          45000 + int32((i*97)%2000-1000),
            Y:          50000 + int32((i*131)%2000-1000),
            Z:          -3500,
            Name:       "Goblin",
        })
        b.mobs = append(b.mobs, objectID)
    }
    for i := range 12 {
        bot.ApplyNpcInfo(state.NpcInfo{
            ObjectID:   int32(2_000_000 + index*10_000 + i),
            TemplateID: int32(7147 + i),
            X:          44000 + int32(i*40),
            Y:          47000 + int32(i*30),
            Z:          -2982,
            Name:       fmt.Sprintf("Folk%d", i),
        })
    }
    loop := hunt.NewLoop(b.game, bot)
    loop.SetGearProfile(gear.MeleeFighter{})
    b.loop = loop

    return b
}

// churn models the packet load of a farming session between the hunt
// ticks: the knownlist movements, the self position sync, the status
// broadcast, the knownlist enter/exit churn and the combat broadcasts
// of the running fights. The deterministic per bot seed keeps the
// runs reproducible.
func (f *fleetBot) churn() {
    f.seed = f.seed*6364136223846793005 + 1442695040888963407
    next := uint32(f.seed >> 33)
    step := func(mod int) int {
        v := int(next % uint32(mod))
        f.seed = f.seed*6364136223846793005 + 1442695040888963407
        next = uint32(f.seed >> 33)

        return v
    }
    for range 3 {
        mobID := f.mobs[step(len(f.mobs))]
        f.bot.ApplyMovement(state.Movement{
            ObjectID: mobID,
            X:        45000 + int32(step(2000)-1000),
            Y:        50000 + int32(step(2000)-1000),
            Z:        -3500,
            DestX:    45000 + int32(step(2000)-1000),
            DestY:    50000 + int32(step(2000)-1000),
            DestZ:    -3500,
        })
    }
    f.bot.ApplyMovement(state.Movement{
        ObjectID: 100, X: 45000, Y: 50000, Z: -3500,
        DestX: 45000 + int32(step(400)), DestY: 50000 + int32(step(400)),
        DestZ: -3500,
    })
    f.bot.ApplyStatusUpdate(100, []state.Attribute{
        {ID: state.AttrCurHP, Value: int32(90 + step(20))},
    })
    if step(10) == 0 {
        // The knownlist churn: an npc despawns, another spawns.
        out := int32(2_000_000 + step(12))
        f.bot.RemoveObject(out)
        f.bot.ApplyNpcInfo(state.NpcInfo{
            ObjectID:   out,
            TemplateID: 1000003,
            Attackable: true,
            X:          45000, Y: 50000, Z: -3500, Name: "Goblin",
        })
    }
    if step(8) == 0 {
        // The combat broadcast of a running fight: a mob swing.
        attacker := f.mobs[step(len(f.mobs))]
        f.bot.ApplyAttack(state.Attack{
            AttackerID: attacker,
            X:          45100, Y: 50100, Z: -3500,
            TargetX: 45000, TargetY: 50000, TargetZ: -3500,
            TargetIDs:   [state.AttackTargets]int32{100},
            TargetCount: 1,
        })
    }
}

// TestFleetProfile runs the fleet through the production Run cadence
// and captures the profiles: the CPU over the whole window, the
// cumulative allocation profile and the inuse heap at the end. The
// artifacts land in runs/fleetprofile/.
func TestFleetProfile(t *testing.T) {
    fleetSize := 40
    if value := os.Getenv("SWARM_FLEET_PROFILE"); value != "" {
        if _, err := fmt.Sscanf(value, "%d", &fleetSize); err != nil {
            t.Fatalf("bad SWARM_FLEET_PROFILE %q: %v", value, err)
        }
    }
    outDir := "../../../runs/fleetprofile"
    if err := os.MkdirAll(outDir, 0o755); err != nil {
        t.Fatalf("output dir: %v", err)
    }
    journalRoot := filepath.Join(outDir, "journals")
    if err := os.MkdirAll(journalRoot, 0o755); err != nil {
        t.Fatalf("journal dir: %v", err)
    }

    fleetJournal, err := session.NewJournal(journalRoot, nil)
    if err != nil {
        t.Fatalf("fleet journal: %v", err)
    }
    defer fleetJournal.Close()
    fleet := make([]*fleetBot, 0, fleetSize)
    for index := range fleetSize {
        b := newFleetBot(index)
        b.loop.SetJournal(fleetJournal)
        fleet = append(fleet, b)
    }

    // The fleet runs the production path: one Run goroutine per bot
    // (the 250 ms ticker inside), one churn goroutine per bot (the
    // packet reader shape, 8 Hz).
    var wg sync.WaitGroup
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    for _, f := range fleet {
        wg.Add(2)
        go func(fb *fleetBot) {
            defer wg.Done()
            fb.loop.Run(ctx)
        }(f)
        go func(fb *fleetBot) {
            defer wg.Done()
            ticker := time.NewTicker(125 * time.Millisecond)
            defer ticker.Stop()
            for {
                select {
                case <-ctx.Done():

                    return
                case <-ticker.C:
                    fb.churn()
                    fb.ticks++
                }
            }
        }(f)
    }

    // The warmup: the equip bursts, the trip planning and the first
    // caches settle before the profile window opens.
    time.Sleep(5 * time.Second)

    var memBefore, memAfter runtime.MemStats
    runtime.GC()
    runtime.ReadMemStats(&memBefore)

    cpuFile, err := os.Create(filepath.Join(outDir, "cpu.prof"))
    if err != nil {
        t.Fatalf("cpu profile: %v", err)
    }
    if err := pprof.StartCPUProfile(cpuFile); err != nil {
        t.Fatalf("start cpu: %v", err)
    }

    const window = 40 * time.Second
    time.Sleep(window)
    pprof.StopCPUProfile()
    cpuFile.Close()

    // The allocation profile: the cumulative mallocs of the process
    // (the pre window baselines subtract in the analysis).
    allocFile, err := os.Create(filepath.Join(outDir, "alloc.prof"))
    if err != nil {
        t.Fatalf("alloc profile: %v", err)
    }
    runtime.GC()
    if err := pprof.WriteHeapProfile(allocFile); err != nil {
        t.Fatalf("write alloc: %v", err)
    }
    allocFile.Close()

    runtime.ReadMemStats(&memAfter)
    heapFile, err := os.Create(filepath.Join(outDir, "heap.prof"))
    if err != nil {
        t.Fatalf("heap profile: %v", err)
    }
    if err := pprof.Lookup("heap").WriteTo(heapFile, 0); err != nil {
        t.Fatalf("write heap: %v", err)
    }
    heapFile.Close()

    var ticks int64
    for _, f := range fleet {
        ticks += f.ticks
    }
    report := fmt.Sprintf(
        "fleet: %d bots, window %s, churn packets applied %d, "+
            "allocated in window %.1f MB, heap inuse %.1f MB, "+
            "goroutines %d\n",
        fleetSize, window, ticks,
        float64(memAfter.TotalAlloc-memBefore.TotalAlloc)/(1<<20),
        float64(memAfter.HeapInuse)/(1<<20),
        runtime.NumGoroutine())
    t.Log(report)
    if err := os.WriteFile(filepath.Join(outDir, "report.txt"),
        []byte(report), 0o600); err != nil {
        t.Fatalf("report: %v", err)
    }
}
