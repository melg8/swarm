// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import (
	"strconv"
	"testing"
)

// benchBotCount is the stretch goal of the project: up to 100
// concurrent bot sessions in one process (AGENTS.md). The registry
// benchmarks measure the per poll cost the web interface pays for
// the bot list and the per session footprint of the trackers
// themselves, so the scaling problems surface before the fleet
// grows past the single bot development shape.
const benchBotCount = 100

// benchFleetNpcCount is the per bot world size of the fleet fixtures.
const benchFleetNpcCount = 20

// benchRegistry builds a registry with benchBotCount bots, each
// loaded with a small live world (the realistic multi bot shape:
// every session tracks its own surroundings, not one huge map).
func benchRegistry() *Registry {
	registry := NewRegistry()
	for i := range benchBotCount {
		bot := NewBot("bot" + strconv.Itoa(i))
		bot.SetCharacter("char"+strconv.Itoa(i),
			int32(268473919+i), 18,
			benchSelfX, benchSelfY, benchSelfZ, 100, 50)
		bot.SetOnline("char" + strconv.Itoa(i))
		bot.SetPhase("engage")
		for j := range benchFleetNpcCount {
			bot.ApplyNpcInfo(benchNpcInfo(
				int32(1_000_000+j*10), benchGoblinTemplate,
				benchSelfX+int32((j*97)%2200-1100),
				benchSelfY+int32((j*131)%2200-1100)))
		}
		registry.Add(bot)
	}

	return registry
}

// BenchmarkRegistryList100 measures the whole registry list build
// (the /api/bots payload source) with 100 sessions: the per bot
// Info() walk under the registry read lock.
func BenchmarkRegistryList100(b *testing.B) {
	registry := benchRegistry()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		infos := registry.List()
		if len(infos) != benchBotCount {
			b.Fatalf("list length: %d", len(infos))
		}
	}
}

// BenchmarkRegistryList100Parallel measures the concurrent list
// polls of the web view: every browser page and the sidebar
// refresh hit the registry at their own cadence.
func BenchmarkRegistryList100Parallel(b *testing.B) {
	registry := benchRegistry()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if infos := registry.List(); len(infos) != benchBotCount {
				b.Fatalf("list length: %d", len(infos))
			}
		}
	})
}

// BenchmarkRegistryGet measures the single bot lookup of the SSE
// and state endpoints (100 bots online, the map is full).
func BenchmarkRegistryGet(b *testing.B) {
	registry := benchRegistry()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, ok := registry.Get("bot42"); !ok {
			b.Fatal("bot missing")
		}
	}
}

// BenchmarkBotInfo measures the per bot compact info build: the
// web list calls this once per session per poll, so it multiplies
// by the bot count and the poll rate.
func BenchmarkBotInfo(b *testing.B) {
	bot := benchWorld(20)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		info := bot.Info()
		if info.ID != "bench" {
			b.Fatal("wrong bot")
		}
	}
}

// BenchmarkNewBot measures the tracker construction cost: the
// supervisor restarts sessions (runBotForever) and the fleet of
// 100 pays this per session start.
func BenchmarkNewBot(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		bot := NewBot("bench")
		if bot.Status() != StatusConnecting {
			b.Fatal("wrong status")
		}
	}
}

// BenchmarkRecordEvent measures the rolling event log write: the
// connection layer records packet events at the live packet rate
// of every session, so the per event cost multiplies by the fleet
// size.
func BenchmarkRecordEvent(b *testing.B) {
	bot := NewBot("bench")
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		bot.RecordEvent("event " + strconv.Itoa(i%64))
	}
}

// BenchmarkSnapshotContended measures the snapshot build while the
// packet apply path writes: the live shape of a fleet - the game
// sessions mutate their trackers while the SSE readers snapshot
// them. One writer goroutine applies movements at full speed while
// the benchmark goroutine builds snapshots.
func BenchmarkSnapshotContended(b *testing.B) {
	bot := benchWorld(200)
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				bot.ApplyMovement(benchMovement(1_000_000))
			}
		}
	}()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		snap := bot.Snapshot()
		if len(snap.Objects) == 0 {
			b.Fatal("empty snapshot")
		}
	}
	close(stop)
}

// benchMovement builds a movement packet payload for the object at
// the given id slot of the bench world.
func benchMovement(objectID int32) Movement {
	return Movement{
		ObjectID: objectID,
		X:        benchSelfX + 100,
		Y:        benchSelfY + 100,
		Z:        benchSelfZ,
		DestX:    benchSelfX + 500,
		DestY:    benchSelfY + 500,
		DestZ:    benchSelfZ,
	}
}

// BenchmarkVersionPoll measures the SSE poll tick cost: every
// streamEvents connection checks the version every 300 ms, so 100
// bots watched by the web view pay this 333 times per second.
func BenchmarkVersionPoll(b *testing.B) {
	bot := benchWorld(20)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if bot.Version() == 0 {
			b.Fatal("zero version")
		}
	}
}

// BenchmarkHundredBotsSnapshot rounds the scale measurement: the
// aggregate cost of snapshotting every bot of the fleet once (the
// worst case of a web view polling all streams at once).
func BenchmarkHundredBotsSnapshot(b *testing.B) {
	registry := benchRegistry()
	bots := make([]*Bot, 0, benchBotCount)
	for i := range benchBotCount {
		bot, _ := registry.Get("bot" + strconv.Itoa(i))
		bots = append(bots, bot)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for _, bot := range bots {
			if snap := bot.Snapshot(); len(snap.Objects) == 0 {
				b.Fatal("empty snapshot")
			}
		}
	}
}

// BenchmarkHundredBotsTickTraffic measures the aggregate apply
// load of the fleet: every session receives its packets while all
// trackers live in one process (the channel receive fan in shape).
func BenchmarkHundredBotsTickTraffic(b *testing.B) {
	registry := benchRegistry()
	bots := make([]*Bot, 0, benchBotCount)
	for i := range benchBotCount {
		bot, _ := registry.Get("bot" + strconv.Itoa(i))
		bots = append(bots, bot)
	}
	movements := make([]Movement, len(bots))
	for i := range movements {
		movements[i] = benchMovement(int32(1_000_000 + i*10))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for i, bot := range bots {
			bot.ApplyMovement(movements[i])
		}
	}
}
