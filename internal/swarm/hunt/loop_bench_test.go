// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/gear"
	"github.com/melg8/swarm/internal/swarm/state"
)

// benchGame implements GameAPI without recording: the benchmark tick
// must measure the decision path itself, not the bookkeeping of the
// test doubles.
type benchGame struct{}

func (b *benchGame) AttackTarget(_ int32) error        { return nil }
func (b *benchGame) WalkTo(_, _, _ int32) error        { return nil }
func (b *benchGame) PickupItem(_ state.LootItem) error { return nil }
func (b *benchGame) ActionSitStand() error             { return nil }
func (b *benchGame) RestartAtVillage() error           { return nil }
func (b *benchGame) DestroyItem(_, _ int32) error      { return nil }
func (b *benchGame) SellItems(_ []state.InventoryItem) error {
	return nil
}

func (b *benchGame) BuyItems(_ int32, _ []gear.Purchase) error {
	return nil
}
func (b *benchGame) UseItem(_ int32) error              { return nil }
func (b *benchGame) ClickObject(_ int32) error          { return nil }
func (b *benchGame) AcquireSkill(_, _ int32) error      { return nil }
func (b *benchGame) UseMagicSkill(_ int32) error        { return nil }
func (b *benchGame) DropItem(_, _, _, _, _ int32) error { return nil }
func (b *benchGame) RequestLogout() error               { return nil }

// benchWorldBot builds a bot with npcCount living attackable mobs
// around the character and two skipped targets, the steady engage
// shape of a farming session.
func benchWorldBot(npcCount int) *state.Bot {
	bot := state.NewBot("bench")
	bot.SetCharacter("bench", 100, 18, 45000, 50000, -3500, 120, 80)
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrMaxHP, Value: 120},
		{ID: state.AttrCurHP, Value: 110},
	})
	for i := range npcCount {
		bot.ApplyNpcInfo(state.NpcInfo{
			ObjectID:   int32(1_000_000 + i*10),
			TemplateID: 1000003,
			Attackable: true,
			X:          45000 + int32((i*97)%2000-1000),
			Y:          50000 + int32((i*131)%2000-1000),
			Z:          -3500,
			Name:       "Goblin",
		})
	}

	return bot
}

// BenchmarkHuntTickEngage measures the engage tick of the hunt loop
// over a populated world: the target search with the social
// constraint, the self state reads and the phase bookkeeping. Every
// bot of the fleet pays this four times a second.
func BenchmarkHuntTickEngage(b *testing.B) {
	bot := benchWorldBot(50)
	loop := NewLoop(&benchGame{}, bot)
	loop.lastHit = time.Now().Add(-time.Minute)
	// Two stale targets held out of the search: the activeSkips
	// rebuild runs on every tick.
	loop.targetSkip = map[int32]time.Time{
		1_000_000: time.Now().Add(time.Minute),
		1_000_010: time.Now().Add(time.Minute),
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		loop.tick()
	}
}

// BenchmarkActiveSkips measures the dense skip list rebuild of the
// target search (the reused scratch buffer of the loop).
func BenchmarkActiveSkips(b *testing.B) {
	bot := benchWorldBot(50)
	loop := NewLoop(&benchGame{}, bot)
	loop.targetSkip = map[int32]time.Time{
		1_000_000: time.Now().Add(time.Minute),
		1_000_010: time.Now().Add(time.Minute),
		1_000_020: time.Now().Add(-time.Minute),
	}
	now := time.Now()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if skips := loop.activeSkips(now); len(skips) != 2 {
			b.Fatalf("skips: %d", len(skips))
		}
	}
}
