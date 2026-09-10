// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package proxy

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The client keepalive of the proxy: the RequestNetPing of an attached
// C1 client must be answered by the proxy itself and never transit to
// the real server, and the NetPing answers of the bot session must
// never reach the client. The transit plus the relay of the answers
// closed the ping feedback loop behind the reported packet explosion
// (an answer driven client re-pings per delivered answer, the transit
// rides the bot session, the new answer is recorded and relayed again,
// and the loop accelerates to tens of thousands of packets per second
// on the attached bot - see packet_storm_test.go for the live repro).

// buildTestNetPingAnswer builds a NetPing answer with the given game
// time, the exact layout of the Mobius server packet: [0xEC][time: 4].
func buildTestNetPingAnswer(gameTime int32) []byte {
	payload := make([]byte, 5)
	payload[0] = serverOpNetPing
	binary.LittleEndian.PutUint32(payload[1:], uint32(gameTime))

	return payload
}

// requireNoServerPing checks that no ping request reached the fake bot
// session (the keepalive never transits).
func requireNoServerPing(t *testing.T, sender *fakeRawSender) {
	t.Helper()
	select {
	case received := <-sender.sent:
		t.Fatalf("the client keepalive reached the server: 0x%02x", received[0])
	default:
	}
}

// TestProxyClientKeepaliveAnsweredLocally verifies the full keepalive
// contract of one in-world client: the recorded NetPing answers of the
// bot session stay out of the replay, the live answers stay out of the
// feed, every client RequestNetPing is answered by the proxy (even a
// burst, even while held), the answers never transit, and the harvested
// game time rides the synthesized answers.
func TestProxyClientKeepaliveAnsweredLocally(t *testing.T) {
	server := startTestServer(t)
	recorder, _, sender := registerFakeBot(t, server, "PingChar")

	recorder.Record(buildTestCharList("PingChar"))
	recorder.Record(buildTestCharSelected("PingChar"))
	recorder.Record(buildTestUserInfoPacket())
	// The recorded history carries the bot session's own keepalive
	// answers: the replay must skip them (they are no world state).
	recorder.Record(buildTestNetPingAnswer(1111))
	recorder.Record(buildTestWorldPacket(0x22, 42)) // the npc spawn

	client := enterWorldAsClient(t, server)

	// The replay: the user info, the npc - and no NetPing answer.
	require.Equal(t, byte(0x04), client.readPacket()[0], "the replayed user info")
	require.Equal(t, byte(0x22), client.readPacket()[0], "the replayed npc spawn")
	requireNoServerPing(t, sender)
	requireConnOpen(t, client, 150*time.Millisecond)

	// The client keepalive: one request, one locally synthesized answer
	// carrying the harvested game time of the skipped history answer.
	client.sendPacket([]byte{clientOpNetPing})
	answer := client.readPacket()
	require.Equal(t, byte(serverOpNetPing), answer[0], "the locally answered keepalive")
	require.Len(t, answer, 5)
	require.EqualValues(t, 1111,
		int32(binary.LittleEndian.Uint32(answer[1:5])),
		"the answer carries the harvested game time")
	requireNoServerPing(t, sender)

	// A burst of requests: every one is answered locally, none of them
	// reaches the server through the bot session.
	const burst = 10
	for range burst {
		client.sendPacket([]byte{clientOpNetPing})
	}
	for range burst {
		require.Equal(t, byte(serverOpNetPing), client.readPacket()[0])
	}
	requireNoServerPing(t, sender)

	// The live feed: a fresh NetPing answer of the bot session (the
	// server answering the bot's own 25 s keepalive) is filtered out -
	// the packet behind it still arrives - and its game time updates
	// the harvested value of the next synthesized answer.
	recorder.Record(buildTestNetPingAnswer(2222))
	recorder.Record(buildTestWorldPacket(0x59, 42)) // the marker behind it
	require.Equal(t, byte(0x59), client.readPacket()[0],
		"the live NetPing answer is filtered, the marker behind it arrives")
	requireNoServerPing(t, sender)

	client.sendPacket([]byte{clientOpNetPing})
	answer = client.readPacket()
	require.Equal(t, byte(serverOpNetPing), answer[0])
	require.EqualValues(t, 2222,
		int32(binary.LittleEndian.Uint32(answer[1:5])),
		"the answer carries the freshly harvested game time")
	requireNoServerPing(t, sender)
}

// TestProxyHeldClientKeepaliveStaysAnswered verifies the hold keeps the
// client keepalive alive: a client held for the bot relogin pings, the
// proxy answers locally (the character is offline, the real server
// cannot answer) instead of swallowing the request silently - the held
// connection must not time out on the client side.
func TestProxyHeldClientKeepaliveStaysAnswered(t *testing.T) {
	server := startTestServer(t)
	recorder, _, sender := registerFakeBot(t, server, "HeldPing")

	recorder.Record(buildTestCharList("HeldPing"))
	recorder.Record(buildTestCharSelected("HeldPing"))
	recorder.Record(buildTestUserInfoPacket())

	client := enterWorldAsClient(t, server)
	require.Equal(t, byte(0x04), client.readPacket()[0], "the replayed user info")

	// The session ends: the client is held for the relogin.
	server.UnregisterSession("testbot", recorder)
	requireConnOpen(t, client, 250*time.Millisecond)

	// The held client pings: the proxy answers locally, the request
	// stays away from the dead session link.
	client.sendPacket([]byte{clientOpNetPing})
	answer := client.readPacket()
	require.Equal(t, byte(serverOpNetPing), answer[0],
		"the held client keepalive is answered locally")
	select {
	case received := <-sender.sent:
		t.Fatalf("the held client keepalive reached the dead session: 0x%02x",
			received[0])
	case <-time.After(150 * time.Millisecond):
	}
}
