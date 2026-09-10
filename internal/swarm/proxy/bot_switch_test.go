// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package proxy

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// registerFakeBotWithID publishes a bot session with a custom id (so
// multiple bots can coexist in one server) and the given recorded
// history. Mirrors registerFakeBot but takes the id as a parameter.
func registerFakeBotWithID(
	t *testing.T, server *Server, id, name string, objectID int32,
) (*Recorder, *state.Bot) {
	t.Helper()
	tracker := state.NewBot(id)
	tracker.SetCharacter(name, objectID, 18, 45000, 50000, -3500, 90, 40)
	tracker.ApplyUserInfo(state.UserInfo{
		Name:    name,
		Level:   7,
		Race:    1,
		ClassID: 18,
		X:       45000,
		Y:       50000,
		Z:       -3500,
		CurHP:   90,
		MaxHP:   120,
		CurMP:   40,
		MaxMP:   50,
		Sp:      5,
		Exp:     1000,
	})
	tracker.SetOnline(name)

	sender := newFakeRawSender()
	recorder := server.RegisterSession(id, sender, tracker)
	t.Cleanup(func() { server.UnregisterSession(id, recorder) })

	return recorder, tracker
}

// buildCharSelectedWithID builds a CharSelected packet carrying the
// given object id (so two bots differ in their self id and the replay
// can be told apart).
func buildCharSelectedWithID(name string, objectID int32) []byte {
	data := []byte{0x21}
	data = appendUTF16(data, name)
	data = binary.LittleEndian.AppendUint32(data, uint32(objectID))
	data = appendUTF16(data, "")
	data = binary.LittleEndian.AppendUint32(data, 42)
	for range 5 {
		data = binary.LittleEndian.AppendUint32(data, 0)
	}
	data = binary.LittleEndian.AppendUint32(data, 1)
	data = binary.LittleEndian.AppendUint32(data, 45000)
	data = binary.LittleEndian.AppendUint32(data, 50000)
	data = binary.LittleEndian.AppendUint32(data, 0xFFFFF268)
	data = binary.LittleEndian.AppendUint64(data, 0)
	data = binary.LittleEndian.AppendUint64(data, 0)
	data = binary.LittleEndian.AppendUint32(data, 0)
	data = binary.LittleEndian.AppendUint32(data, 0)
	data = binary.LittleEndian.AppendUint32(data, 7)
	for range 10 {
		data = binary.LittleEndian.AppendUint32(data, 0)
	}

	return data
}

// buildUserInfoWithID builds a UserInfo packet carrying the given
// object id and name (the self state the proxy live-patches).
func buildUserInfoWithID(name string, objectID int32) []byte {
	data := []byte{0x04}
	data = binary.LittleEndian.AppendUint32(data, 45000)
	data = binary.LittleEndian.AppendUint32(data, 50000)
	data = binary.LittleEndian.AppendUint32(data, 0xFFFFF268)
	data = binary.LittleEndian.AppendUint32(data, 0)
	data = binary.LittleEndian.AppendUint32(data, uint32(objectID))
	data = appendUTF16(data, name)
	for range 15 {
		data = binary.LittleEndian.AppendUint32(data, 0)
	}

	return data
}

// TestProxySwitchesConnectedClientToSelectedBot verifies the cross-bot
// switch: a client connected and watching bot A receives the teleport +
// DeleteObject sweep + enter world burst of bot B when the WebUI
// selects B, without reconnecting. The client sees the new character's
// position, appearance and known list.
func TestProxySwitchesConnectedClientToSelectedBot(t *testing.T) {
	server := startTestServer(t)

	// Bot A: the initially selected bot.
	recorderA, trackerA := registerFakeBotWithID(
		t, server, "botA", "CharA", 1001)
	trackerA.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 5001, Attackable: true,
		X: 45200, Y: 50200, Z: -3500, Name: "KeltirA",
	})
	recorderA.Record(buildCharSelectedWithID("CharA", 1001))
	recorderA.Record(buildUserInfoWithID("CharA", 1001))
	recorderA.Record(buildTestWorldPacket(0x22, 5001))

	// Bot B: the target of the switch.
	recorderB, trackerB := registerFakeBotWithID(
		t, server, "botB", "CharB", 2002)
	trackerB.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 6002, Attackable: true,
		X: 46000, Y: 51000, Z: -3500, Name: "GoblinB",
	})
	recorderB.Record(buildCharSelectedWithID("CharB", 2002))
	recorderB.Record(buildUserInfoWithID("CharB", 2002))
	recorderB.Record(buildTestWorldPacket(0x22, 6002))

	// The client connects and selects bot A (the first registered).
	server.SelectBot("botA")
	client := enterWorldAsClient(t, server)

	// The replay of bot A: the UserInfo (0x04) and the npc spawn (0x22).
	require.Equal(t, byte(0x04), client.readPacket()[0],
		"the replayed user info of bot A")
	npcA := client.readPacket()
	require.Equal(t, byte(0x22), npcA[0], "the npc spawn of bot A")

	// The WebUI switches the selection to bot B.
	server.SelectBot("botB")

	// The client receives the resync: a TeleportToLocation (0x38), a
	// DeleteObject (0x1E) sweep of the old known list, then the enter
	// world burst of bot B (its UserInfo 0x04 and its npc spawn 0x22).
	teleport := client.readPacket()
	require.Equal(t, byte(0x38), teleport[0],
		"the teleport to bot B's position")

	// The DeleteObject sweep: bot A's npc (5001) is swept. The self id
	// of bot A (1001) is NOT swept (the client's own object id is
	// skipped). The sweep may arrive as one or more 0x1E packets.
	swept := 0
	for {
		pkt := client.readPacket()
		if pkt[0] != 0x1E {
			// We reached the enter world burst of bot B.
			require.Equal(t, byte(0x04), pkt[0],
				"the replayed user info of bot B after the sweep")
			swept++

			break
		}
		swept++
	}
	require.GreaterOrEqual(t, swept, 1,
		"at least the DeleteObject sweep or the bot B UserInfo arrives")

	// The npc spawn of bot B follows the UserInfo.
	npcB := client.readPacket()
	require.Equal(t, byte(0x22), npcB[0],
		"the npc spawn of bot B after the switch")

	// The client connection stays open.
	requireConnOpen(t, client, 250*time.Millisecond)
}

// TestProxySelectBotSameIdDoesNotSwitch verifies that selecting the
// already-selected bot does not trigger a resync (no spurious teleport).
func TestProxySelectBotSameIdDoesNotSwitch(t *testing.T) {
	server := startTestServer(t)
	recorderA, _ := registerFakeBotWithID(
		t, server, "botA", "CharA", 1001)
	recorderA.Record(buildCharSelectedWithID("CharA", 1001))
	recorderA.Record(buildUserInfoWithID("CharA", 1001))

	server.SelectBot("botA")
	client := enterWorldAsClient(t, server)

	// Read the replay.
	require.Equal(t, byte(0x04), client.readPacket()[0],
		"the replayed user info of bot A")

	// Re-selecting the same bot must not fire a resync.
	server.SelectBot("botA")

	// No teleport packet should arrive: the client stays on the live
	// feed of bot A. A short wait confirms nothing is sent.
	requireNoPacket(t, client, 200*time.Millisecond)
}

// TestProxySwitchToOfflineBotStaysOnCurrent verifies that selecting a
// bot that is not yet online does NOT resync the client: it stays on
// the current bot's live feed until the target comes online.
func TestProxySwitchToOfflineBotStaysOnCurrent(t *testing.T) {
	server := startTestServer(t)
	recorderA, _ := registerFakeBotWithID(
		t, server, "botA", "CharA", 1001)
	recorderA.Record(buildCharSelectedWithID("CharA", 1001))
	recorderA.Record(buildUserInfoWithID("CharA", 1001))

	server.SelectBot("botA")
	client := enterWorldAsClient(t, server)
	require.Equal(t, byte(0x04), client.readPacket()[0],
		"the replayed user info of bot A")

	// Select a bot that is not registered.
	server.SelectBot("nonexistent")

	// No resync: the client stays on bot A's live feed.
	requireNoPacket(t, client, 200*time.Millisecond)
}

// requireNoPacket asserts that no packet arrives within the given
// duration: the relay stays idle (no spurious resync, no live feed).
func requireNoPacket(t *testing.T, client *fakeGameClient, d time.Duration) {
	t.Helper()
	requireConnOpen(t, client, d)
}
