// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
	"encoding/json"
	"testing"

	"github.com/melg8/swarm/internal/swarm/state"
)

// The event stream marshals the full snapshot through encoding/json
// every time the bot state version changes - with the per packet
// version bumps of a live session that is the JSON cost of every
// received packet. The benchmark builds the same world shape as the
// state tracker benchmarks: 200 npcs around the character and a 55
// slot inventory.
const (
	benchSelfX   = 45000
	benchSelfY   = 50000
	benchSelfZ   = -3500
	benchNpcKind = 1000003
)

// benchSnapshotBot builds a loaded bot for the JSON measurements.
func benchSnapshotBot(npcCount int) *state.Bot {
	bot := state.NewBot("test1")
	bot.SetCharacter("test1", 268473919, 18,
		benchSelfX, benchSelfY, benchSelfZ, 100, 50)
	bot.SetHuntingZone(benchSelfX, benchSelfY, 1650)
	for i := range npcCount {
		bot.ApplyNpcInfo(state.NpcInfo{
			ObjectID:        int32(1_000_000 + i*10),
			TemplateID:      benchNpcKind,
			Attackable:      true,
			X:               benchSelfX + int32((i*97)%2200-1100),
			Y:               benchSelfY + int32((i*131)%2200-1100),
			Z:               benchSelfZ,
			Heading:         16384,
			RunSpeed:        165,
			WalkSpeed:       55,
			MoveSpeedMult:   1.15,
			CollisionRadius: 10,
			Running:         true,
			InCombat:        false,
			Dead:            false,
			Name:            "",
			Title:           "",
		})
	}
	items := make([]state.InventoryItem, 0, 55)
	for i := range 55 {
		items = append(items, state.InventoryItem{
			ObjectID: int32(900_001 + i),
			ItemID:   int32(i%80) + 1,
			Count:    1,
			Type1:    0,
			Type2:    0,
			Equipped: i < 6,
			BodyPart: 0,
			Enchant:  0,
			Change:   0,
		})
	}
	bot.ApplyItemList(items)

	return bot
}

// BenchmarkSnapshotJSON measures the reflection marshal of the full
// snapshot the event stream sends per version change.
func BenchmarkSnapshotJSON(b *testing.B) {
	bot := benchSnapshotBot(200)
	snapshot := bot.Snapshot()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		data, err := json.Marshal(snapshot)
		if err != nil {
			b.Fatal(err)
		}
		if len(data) == 0 {
			b.Fatal("expected json output")
		}
	}
}

// BenchmarkSnapshotJSONBuild measures the snapshot copy plus the
// marshal together - the true per event cost of the stream.
func BenchmarkSnapshotJSONBuild(b *testing.B) {
	bot := benchSnapshotBot(200)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		data, err := json.Marshal(bot.Snapshot())
		if err != nil {
			b.Fatal(err)
		}
		if len(data) == 0 {
			b.Fatal("expected json output")
		}
	}
}
