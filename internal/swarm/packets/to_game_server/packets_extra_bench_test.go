// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
	"testing"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

// BenchmarkMoveToLocationToBytes measures the serialization of the
// most frequent outbound packet: the hunt loop sends one per movement
// leg per bot, so the 100 bot fleet pays this thousands of times per
// second during walks.
func BenchmarkMoveToLocationToBytes(b *testing.B) {
	move := &MoveToLocationRequestPacket{
		TargetX: 45000,
		TargetY: 50000,
		TargetZ: -3500,
		OriginX: 44000,
		OriginY: 49000,
		OriginZ: -3500,
		Mode:    MoveModeMouse,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		writer := packet.NewWriter()
		if err := move.ToBytes(writer); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkAttackRequestToBytes measures the attack request serialization:
// the hunt loop sends one per swing per bot.
func BenchmarkAttackRequestToBytes(b *testing.B) {
	attack := &AttackRequestPacket{
		TargetID: 268473919,
		X:        45000,
		Y:        50000,
		Z:        -3500,
		Shift:    0,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		writer := packet.NewWriter()
		if err := attack.ToBytes(writer); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRequestActionUseToBytes measures the sit/stand toggle
// serialization: the rest loop sends one per rest cycle per bot.
func BenchmarkRequestActionUseToBytes(b *testing.B) {
	action := &RequestActionUsePacket{
		ActionID: ActionSitStand,
		Ctrl:     false,
		Shift:    false,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		writer := packet.NewWriter()
		if err := action.ToBytes(writer); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRequestBuyItemToBytes measures the buy request serialization
// with a realistic 5 item purchase batch.
func BenchmarkRequestBuyItemToBytes(b *testing.B) {
	items := make([]BuyItemEntry, 0, 5)
	for i := range 5 {
		items = append(items, BuyItemEntry{
			ItemID: int32(1 + i),
			Count:  int32(1),
		})
	}
	buy := &RequestBuyItemPacket{
		ListID: 1,
		Items:  items,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		writer := packet.NewWriter()
		if err := buy.ToBytes(writer); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRequestDestroyItemToBytes measures the destroy item
// serialization: the inventory cleanup loop sends one per destroyed item.
func BenchmarkRequestDestroyItemToBytes(b *testing.B) {
	destroy := &RequestDestroyItem{
		ObjectID: 268473919,
		Count:    1,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		writer := packet.NewWriter()
		if err := destroy.ToBytes(writer); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRequestItemListToBytes measures the inventory list request:
// the web UI refresh and the manual inventory check send one per query.
func BenchmarkRequestItemListToBytes(b *testing.B) {
	req := &RequestItemList{}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		writer := packet.NewWriter()
		if err := req.ToBytes(writer); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCharacterSelectToBytes measures the character select
// serialization: the login flow sends one per session start.
func BenchmarkCharacterSelectToBytes(b *testing.B) {
	sel := &CharacterSelect{CharSlot: 0}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		writer := packet.NewWriter()
		if err := sel.ToBytes(writer); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSessionPacketsToBytes measures the enter world, net ping
// and logout packet serializations together: the session lifecycle
// packets the supervisor sends on every bot start and shutdown.
func BenchmarkSessionPacketsToBytes(b *testing.B) {
	enter := &EnterWorld{}
	ping := &RequestNetPing{}
	logout := &Logout{}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		w1 := packet.NewWriter()
		_ = enter.ToBytes(w1)
		w2 := packet.NewWriter()
		_ = ping.ToBytes(w2)
		w3 := packet.NewWriter()
		_ = logout.ToBytes(w3)
	}
}

// BenchmarkFleetOutboundTick measures the aggregate outbound
// serialization cost of one hunt tick for one bot: the move request,
// the attack request, the action use (sit/stand) and the item list
// request together approximate the per tick outbound traffic. The
// 100 bot fleet pays this 100 times per tick, so the per packet
// allocation cost multiplies directly into GC pressure.
func BenchmarkFleetOutboundTick(b *testing.B) {
	move := &MoveToLocationRequestPacket{
		TargetX: 45000, TargetY: 50000, TargetZ: -3500,
		OriginX: 44000, OriginY: 49000, OriginZ: -3500,
		Mode: MoveModeMouse,
	}
	attack := &AttackRequestPacket{
		TargetID: 268473919,
		X:        45000, Y: 50000, Z: -3500,
		Shift: 0,
	}
	action := &RequestActionUsePacket{ActionID: ActionSitStand}
	list := &RequestItemList{}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		w := packet.NewWriter()
		_ = move.ToBytes(w)
		w = packet.NewWriter()
		_ = attack.ToBytes(w)
		w = packet.NewWriter()
		_ = action.ToBytes(w)
		w = packet.NewWriter()
		_ = list.ToBytes(w)
	}
}
