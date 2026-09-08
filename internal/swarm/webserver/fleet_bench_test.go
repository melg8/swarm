// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/melg8/swarm/internal/swarm/state"
)

// benchFleetSize is the stretch goal fleet size (AGENTS.md: up to
// 100 concurrent bot sessions). The fleet benchmarks measure the
// web server costs that multiply with the session count: the bot
// list endpoint every page polls and the SSE frame write path
// every open stream executes on every version change.
const benchFleetSize = 100

// benchFleetRegistry builds a 100 bot registry, every bot carrying
// a small live world (the multi bot shape, not one bot with a huge
// map).
func benchFleetRegistry(b *testing.B, npcCount int) *state.Registry {
	b.Helper()
	registry := state.NewRegistry()
	for i := range benchFleetSize {
		bot := state.NewBot("bot" + strconv.Itoa(i))
		bot.SetCharacter("char"+strconv.Itoa(i),
			int32(268473919+i), 18,
			benchSelfX, benchSelfY, benchSelfZ, 100, 50)
		bot.SetOnline("char" + strconv.Itoa(i))
		bot.SetPhase("engage")
		for j := range npcCount {
			bot.ApplyNpcInfo(state.NpcInfo{
				ObjectID:        int32(1_000_000 + j*10),
				TemplateID:      benchNpcKind,
				Attackable:      true,
				X:               benchSelfX + int32((j*97)%2200-1100),
				Y:               benchSelfY + int32((j*131)%2200-1100),
				Z:               benchSelfZ,
				Heading:         16384,
				RunSpeed:        165,
				WalkSpeed:       55,
				MoveSpeedMult:   1.15,
				CollisionRadius: 10,
				Running:         true,
				InCombat:        true,
				Dead:            false,
				Name:            "",
				Title:           "",
			})
		}
		registry.Add(bot)
	}

	return registry
}

// BenchmarkBotListEndpoint100 measures the /api/bots handler with
// the full fleet: the registry list walk plus the JSON encode of
// 100 bot infos per poll.
func BenchmarkBotListEndpoint100(b *testing.B) {
	logger := log.New(io.Discard, "", 0)
	server := NewServer(benchFleetRegistry(b, 20), "127.0.0.1:0", logger)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/bots", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		// The recorder reuse skips the per call allocation of a
		// fresh recorder: only the handler work itself is
		// measured. The body reset keeps the buffer bounded
		// without reallocating.
		recorder.Body.Reset()
		server.handleBotList(recorder, request)
		if recorder.Body.Len() == 0 {
			b.Fatal("empty body")
		}
	}
}

// BenchmarkSSEFrame measures the SSE event frame write path: the
// per event buffer assembly of writeEvent over a typical snapshot
// payload (the 100 npc stream frame).
func BenchmarkSSEFrame(b *testing.B) {
	bot := benchSnapshotBot(100)
	data := bot.Snapshot().AppendJSON(nil)
	discard := &discardWriter{}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		writeEvent(discard, discard, data)
	}
}

// discardWriter swallows the SSE bytes without the kernel write
// cost, isolating the frame allocation profile.
type discardWriter struct {
	header http.Header
}

func (d *discardWriter) Header() http.Header {
	if d.header == nil {
		d.header = http.Header{}
	}

	return d.header
}

func (d *discardWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

func (d *discardWriter) WriteHeader(_ int) {}

func (d *discardWriter) Flush() {}

// BenchmarkSSEEncodeAndFrame measures the full per event cost the
// stream pays on every version change: the snapshot build, the
// JSON append encode and the frame write together.
func BenchmarkSSEEncodeAndFrame(b *testing.B) {
	bot := benchSnapshotBot(100)
	discard := &discardWriter{}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		writeEvent(discard, discard, bot.Snapshot().AppendJSON(nil))
	}
}

// BenchmarkSSEStreamPoll100 measures the aggregate poll loop of
// the fleet streams: every streamEvents connection wakes every
// 300 ms and checks the version - the idle fleet overhead of the
// web view watching all 100 bots.
func BenchmarkSSEStreamPoll100(b *testing.B) {
	registry := benchFleetRegistry(b, 20)
	bots := make([]*state.Bot, 0, benchFleetSize)
	for i := range benchFleetSize {
		bot, _ := registry.Get("bot" + strconv.Itoa(i))
		bots = append(bots, bot)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for _, bot := range bots {
			if bot.Version() == 0 {
				b.Fatal("zero version")
			}
		}
	}
}
