// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package proxy

import (
	"encoding/binary"
	"os"
	"testing"
	"time"

	fromgameserver "github.com/melg8/swarm/internal/swarm/packets/from_game_server"
	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// testReadDeadline bounds one client read in the handoff tests: wide
// enough for the hold polls, tight enough to fail fast.
const testReadDeadline = 5 * time.Second

// tightenHoldTimings shortens the hold knobs for the tests and restores
// the production values afterwards (a 2 minute hold window and a 250 ms
// poll would make every assertion crawl). The writes take the knob
// mutex so the holds of still unwinding connections read consistent
// values (see holdKnobsMu).
func tightenHoldTimings(t *testing.T, timeout time.Duration) {
	t.Helper()
	holdKnobsMu.Lock()
	originalTimeout, originalPoll := clientHoldTimeout, holdPollPeriod
	clientHoldTimeout, holdPollPeriod = timeout, 10*time.Millisecond
	holdKnobsMu.Unlock()
	t.Cleanup(func() {
		holdKnobsMu.Lock()
		defer holdKnobsMu.Unlock()
		clientHoldTimeout, holdPollPeriod = originalTimeout, originalPoll
	})
}

// enterWorldAsClient walks a fake client through the full emulated game
// flow - handshake, auth login, the char list, the char selection, the
// enter world - and returns it ready to read the replay. The replay
// itself stays unread: the caller asserts it packet by packet.
func enterWorldAsClient(t *testing.T, server *Server) *fakeGameClient {
	t.Helper()
	client := dialGame(t, server.GameAddr())
	client.handshake()

	authLogin := []byte{0x08}
	authLogin = appendUTF16(authLogin, "helduser")
	for range 4 {
		authLogin = binary.LittleEndian.AppendUint32(authLogin, 1)
	}
	client.sendPacket(authLogin)

	require.Equal(t, byte(0x1F), client.readPacket()[0], "char list opcode")
	client.sendPacket([]byte{0x0D, 0x00, 0x00, 0x00, 0x00})
	require.Equal(t, byte(0x21), client.readPacket()[0], "char selected opcode")
	client.sendPacket([]byte{0x03})

	return client
}

// requireConnOpen asserts the client connection is alive: a short read
// deadline must expire (nothing was sent and the socket was not closed).
// The read deadline is reset for the following reads.
func requireConnOpen(t *testing.T, client *fakeGameClient, wait time.Duration) {
	t.Helper()
	require.NoError(t, client.conn.SetReadDeadline(time.Now().Add(wait)))
	_, err := client.conn.Read(make([]byte, 1))
	require.ErrorIs(t, err, os.ErrDeadlineExceeded,
		"the connection must stay open, the probe read failed differently")
	require.NoError(t, client.conn.SetReadDeadline(
		time.Now().Add(testReadDeadline)))
}

// requireConnClosed asserts the proxy closed the client connection
// within the window: the probe read must fail with a close error, not
// with the deadline timeout.
func requireConnClosed(t *testing.T, client *fakeGameClient, within time.Duration) {
	t.Helper()
	require.NoError(t, client.conn.SetReadDeadline(time.Now().Add(within)))
	_, err := client.conn.Read(make([]byte, 1))
	require.Error(t, err, "the connection must be closed")
	require.NotErrorIs(t, err, os.ErrDeadlineExceeded,
		"the connection must close within %s, the probe only timed out", within)
}

// closeHeldClient ends the client connection and waits until the proxy
// served its full teardown (the client counter drops): the relay
// goroutines - a hold polling for the relogin - unwind before the test
// returns, so no background reader touches the test knobs afterwards.
func closeHeldClient(t *testing.T, server *Server, client *fakeGameClient) {
	t.Helper()
	require.NoError(t, client.conn.Close())
	deadline := time.Now().Add(3 * time.Second)
	for server.ClientCount() > 0 {
		if time.Now().After(deadline) {
			t.Fatalf("the client connection teardown never finished "+
				"(%d clients remain)", server.ClientCount())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestHandoffPacketBuilders pins the byte layouts of the synthesized
// handoff packets against the Mobius C1 writeImpl bodies.
func TestHandoffPacketBuilders(t *testing.T) {
	require.Equal(t, []byte{0x74, 0x01, 0x00, 0x00, 0x00},
		buildRestartResponsePacket(),
		"the restart response: opcode plus the true result int")

	require.Equal(t, []byte{0x96}, buildLeaveWorldPacket())
}

// TestGameServerHoldsClientThroughBotRelogin is the core handoff
// scenario: the bot logs out (an emergency logout of the hunt loop) and
// relogs while the user client sits in the world. The client must not
// drop to the login screen: the LeaveWorld of the bot logout is
// suppressed, the connection is held and swallows the client packets,
// and the replacement session takes the client through the restart
// dance - the client returns to its char select screen, is auto
// selected onto the same character (live patched to the fresh place)
// and re-enters the world of the replacement session: its enter world
// burst replays, followed by its live feed and the client packet
// transit.
func TestGameServerHoldsClientThroughBotRelogin(t *testing.T) {
	tightenHoldTimings(t, 2*time.Second)
	tightenAutoSelectTimings(t, 150*time.Millisecond)
	server := startTestServer(t)
	recorder, tracker, sender := registerFakeBot(t, server, "HeldChar")

	// The known world the client sees: two npcs around the character.
	tracker.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 42, Attackable: true,
		X: 45200, Y: 50200, Z: -3500, Name: "Keltir",
	})
	tracker.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 43, Attackable: true,
		X: 45100, Y: 50100, Z: -3500, Name: "Gremlin",
	})

	recorder.Record(buildTestCharList("HeldChar"))
	recorder.Record(buildTestCharSelected("HeldChar"))
	recorder.Record(buildTestUserInfoPacket())
	// A LeaveWorld inside the recorded history must not replay either.
	recorder.Record(buildLeaveWorldPacket())
	recorder.Record(buildTestWorldPacket(0x22, 42)) // the npc spawn

	client := enterWorldAsClient(t, server)

	// The replay: the user info, the LeaveWorld skipped, the npc.
	require.Equal(t, byte(0x04), client.readPacket()[0], "the replayed user info")
	npcSpawn := client.readPacket()
	require.Equal(t, byte(0x22), npcSpawn[0],
		"the packet behind the recorded LeaveWorld replays, it does not")

	// The bot logs out: the LeaveWorld answer must be suppressed in the
	// live feed as well - the client stays in the world.
	recorder.Record(buildLeaveWorldPacket())
	recorder.Record(buildTestWorldPacket(0x59, 42)) // the marker behind it
	marker := client.readPacket()
	require.Equal(t, byte(0x59), marker[0],
		"the live LeaveWorld is suppressed, the marker behind it arrives")

	// The session ends (the recorder closes): the client is held, not
	// disconnected.
	server.UnregisterSession("testbot", recorder)
	requireConnOpen(t, client, 250*time.Millisecond)

	// The held client packets are swallowed: nothing reaches the dead
	// session link.
	client.sendPacket([]byte{0xA8})
	select {
	case received := <-sender.sent:
		t.Fatalf("a held client packet reached the dead session: 0x%02x",
			received[0])
	case <-time.After(150 * time.Millisecond):
	}

	// The relogin: the same tracker, a fresh recorder and a fresh send
	// path - the character spawns at a far away place with a heading.
	tracker.ResetSession()
	tracker.SetCharacter("HeldChar", 1055, 18, 60000, 61000, -2500, 80, 35)
	tracker.ApplyUserInfo(state.UserInfo{
		Name: "HeldChar", Level: 8, Race: 1, ClassID: 18,
		X: 60000, Y: 61000, Z: -2500,
		CurHP: 80, MaxHP: 130, CurMP: 35, MaxMP: 55,
		Sp: 9, Exp: 2000,
	})
	tracker.ApplyPlacement(state.Placement{
		ObjectID: 1055, X: 60000, Y: 61000, Z: -2500, Heading: 1000,
	})
	tracker.SetOnline("HeldChar")

	reloginSender := newFakeRawSender()
	reloginRecorder := server.RegisterSession("testbot", reloginSender, tracker)
	t.Cleanup(func() { server.UnregisterSession("testbot", reloginRecorder) })

	reloginRecorder.Record(buildTestCharSelected("HeldChar"))
	reloginRecorder.Record(buildTestUserInfoPacket())
	reloginRecorder.Record(buildTestWorldPacket(0x22, 77)) // a new npc

	// The restart dance of the replacement: the restart answer moves
	// the client to the char select screen, the char list of the same
	// character is offered and the auto select answers with the
	// CharSelected live patched to the fresh place.
	list, selected := readDance(t, client)
	require.Len(t, list.Characters, 1)
	require.Equal(t, "HeldChar", list.Characters[0].Name,
		"the dance of a relogin offers the same character")
	require.EqualValues(t, 60000, charSelectedField(t, selected, 0),
		"the live patched x of the relogin answer")
	require.EqualValues(t, 61000, charSelectedField(t, selected, 4),
		"the live patched y of the relogin answer")
	require.EqualValues(t, -2500, charSelectedField(t, selected, 8),
		"the live patched z of the relogin answer")

	// The client re-enters the world: the enter world burst of the
	// replacement session replays with the live self state.
	client.sendPacket([]byte{0x03})
	replayedUserInfo := fromgameserver.NewUserInfoPacket()
	require.NoError(t, fromgameserver.ParseUserInfoPacket(
		replayedUserInfo, client.readPacket()))
	require.EqualValues(t, 60000, replayedUserInfo.X, "the live replay x")
	require.EqualValues(t, 61000, replayedUserInfo.Y, "the live replay y")
	require.EqualValues(t, -2500, replayedUserInfo.Z, "the live replay z")
	require.EqualValues(t, 8, replayedUserInfo.Level, "the live replay level")

	newNpc := client.readPacket()
	require.Equal(t, byte(0x22), newNpc[0], "the new session npc replays")
	require.EqualValues(t, 77, int32(binary.LittleEndian.Uint32(newNpc[1:5])))

	// The live feed of the replacement session flows to the held client.
	reloginRecorder.Record(buildTestWorldPacket(0x59, 77))
	require.Equal(t, byte(0x59), client.readPacket()[0], "the live feed")

	// The client packets transit through the replacement session now.
	move := []byte{0x01}
	move = binary.LittleEndian.AppendUint32(move, 60000)
	move = binary.LittleEndian.AppendUint32(move, 61000)
	move = binary.LittleEndian.AppendUint32(move, 0xFFFFF63C) // z -2500
	client.sendPacket(move)
	select {
	case received := <-reloginSender.sent:
		require.Equal(t, move, received,
			"the client packet must reach the replacement session")
	case <-time.After(time.Second):
		t.Fatal("the client packet never reached the replacement session")
	}

	// The second bot logout keeps the hold: the replacement session ends
	// without a user logout, the client is held again.
	server.UnregisterSession("testbot", reloginRecorder)
	requireConnOpen(t, client, 250*time.Millisecond)

	// The test ends with the second hold live: unwind it fully.
	closeHeldClient(t, server, client)
}

// TestGameServerUserLogoutReturnsToLoginScreen pins the classic flow the
// user initiates: the client's own Logout transits to the real server,
// the LeaveWorld answer is relayed (not suppressed) so the client drops
// to the login screen, and the session end then closes the connection.
func TestGameServerUserLogoutReturnsToLoginScreen(t *testing.T) {
	server := startTestServer(t)
	recorder, _, sender := registerFakeBot(t, server, "LeaverChar")

	recorder.Record(buildTestCharList("LeaverChar"))
	recorder.Record(buildTestCharSelected("LeaverChar"))
	recorder.Record(buildTestUserInfoPacket())

	client := enterWorldAsClient(t, server)
	require.Equal(t, byte(0x04), client.readPacket()[0], "the replayed user info")

	// The user logs out: the request transits like any other packet.
	client.sendPacket([]byte{0x09})
	select {
	case received := <-sender.sent:
		require.Equal(t, []byte{0x09}, received, "the logout request transits")
	case <-time.After(time.Second):
		t.Fatal("the logout request never reached the bot session")
	}

	// The server answer is relayed: the client returns to the login
	// screen on its own.
	recorder.Record(buildLeaveWorldPacket())
	require.Equal(t, byte(0x96), client.readPacket()[0],
		"the LeaveWorld of a user logout must be relayed")

	// The session end closes the client connection (no hold: the user
	// is gone).
	server.UnregisterSession("testbot", recorder)
	requireConnClosed(t, client, 2*time.Second)

	// Let the proxy finish the teardown before the test returns.
	closeHeldClient(t, server, client)
}

// TestGameServerUserLogoutWhileHeldClosesClient pins the frozen world
// exit: a user that gives up on the held client (the bot is offline)
// receives the synthesized LeaveWorld and the connection closes - the
// login screen beats a swallowed intent.
func TestGameServerUserLogoutWhileHeldClosesClient(t *testing.T) {
	tightenHoldTimings(t, 10*time.Second)
	server := startTestServer(t)
	recorder, _, _ := registerFakeBot(t, server, "FrozenChar")

	recorder.Record(buildTestCharList("FrozenChar"))
	recorder.Record(buildTestCharSelected("FrozenChar"))
	recorder.Record(buildTestUserInfoPacket())

	client := enterWorldAsClient(t, server)
	require.Equal(t, byte(0x04), client.readPacket()[0], "the replayed user info")

	// The bot dies, the client is held.
	server.UnregisterSession("testbot", recorder)
	requireConnOpen(t, client, 250*time.Millisecond)

	// The user logs out while held: the proxy answers itself.
	client.sendPacket([]byte{0x09})
	require.Equal(t, byte(0x96), client.readPacket()[0],
		"the synthesized LeaveWorld for the held logout")
	requireConnClosed(t, client, 2*time.Second)

	// Let the proxy finish the teardown before the test returns.
	closeHeldClient(t, server, client)
}

// TestGameServerHoldTimeoutReleasesClient pins the dead bot path: a bot
// that never comes back releases the held client once the hold window
// expires (a frozen world for minutes beats a silent forever).
func TestGameServerHoldTimeoutReleasesClient(t *testing.T) {
	tightenHoldTimings(t, 250*time.Millisecond)
	server := startTestServer(t)
	recorder, _, _ := registerFakeBot(t, server, "GhostChar")

	recorder.Record(buildTestCharList("GhostChar"))
	recorder.Record(buildTestCharSelected("GhostChar"))
	recorder.Record(buildTestUserInfoPacket())

	client := enterWorldAsClient(t, server)
	require.Equal(t, byte(0x04), client.readPacket()[0], "the replayed user info")

	server.UnregisterSession("testbot", recorder)
	requireConnOpen(t, client, 100*time.Millisecond)
	requireConnClosed(t, client, 3*time.Second)

	// Let the proxy finish the teardown before the test returns.
	closeHeldClient(t, server, client)
}

// TestGameServerReloginWithoutCharSelectedKeepsHold pins the defensive
// path of the restart dance: a replacement session whose CharSelected
// answer was never recorded cannot take a client through the dance
// (there is nothing to serve as the double click answer), so the held
// client keeps waiting until the login handshake lands in the recorder
// - which is the production order anyway: the char selected answer is
// recorded before the character enters the world.
func TestGameServerReloginWithoutCharSelectedKeepsHold(t *testing.T) {
	tightenHoldTimings(t, 2*time.Second)
	tightenAutoSelectTimings(t, 150*time.Millisecond)
	server := startTestServer(t)
	recorder, tracker, _ := registerFakeBot(t, server, "LiveChar")

	tracker.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 42, Attackable: true,
		X: 45200, Y: 50200, Z: -3500, Name: "Keltir",
	})

	recorder.Record(buildTestCharList("LiveChar"))
	recorder.Record(buildTestCharSelected("LiveChar"))
	recorder.Record(buildTestUserInfoPacket())

	client := enterWorldAsClient(t, server)
	require.Equal(t, byte(0x04), client.readPacket()[0], "the replayed user info")

	server.UnregisterSession("testbot", recorder)
	requireConnOpen(t, client, 250*time.Millisecond)

	// The replacement session enters the world with no recorded
	// CharSelected at all: the hold keeps waiting (no dance can be
	// served), the frozen world stays.
	tracker.ResetSession()
	tracker.SetCharacter("LiveChar", 1055, 18, 61000, 62000, -2600, 80, 35)
	tracker.SetOnline("LiveChar")

	reloginSender := newFakeRawSender()
	reloginRecorder := server.RegisterSession("testbot", reloginSender, tracker)
	t.Cleanup(func() { server.UnregisterSession("testbot", reloginRecorder) })

	requireNoPacket(t, client, 400*time.Millisecond)

	// The login handshake lands in the recorder: the dance completes
	// on its own.
	reloginRecorder.Record(buildTestCharSelected("LiveChar"))
	reloginRecorder.Record(buildTestUserInfoPacket())

	list, _ := readDance(t, client)
	require.Equal(t, "LiveChar", list.Characters[0].Name,
		"the dance of the late char selected")

	client.sendPacket([]byte{0x03})
	require.Equal(t, byte(0x04), client.readPacket()[0],
		"the replayed user info after the late dance")

	// Unwind the live relay before the test returns.
	closeHeldClient(t, server, client)
}
