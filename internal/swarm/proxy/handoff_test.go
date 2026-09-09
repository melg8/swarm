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
	teleport := buildTeleportToLocationPacket(1055, 60000, 61000, -2500, 1000)
	require.Equal(t, []byte{0x38}, teleport[:1], "the teleport opcode")
	require.Len(t, teleport, 25, "opcode plus six int32 fields")
	require.EqualValues(t, 1055, int32(binary.LittleEndian.Uint32(teleport[1:5])))
	require.EqualValues(t, 60000, int32(binary.LittleEndian.Uint32(teleport[5:9])))
	require.EqualValues(t, 61000, int32(binary.LittleEndian.Uint32(teleport[9:13])))
	require.EqualValues(t, -2500, int32(binary.LittleEndian.Uint32(teleport[13:17])))
	require.Zero(t, binary.LittleEndian.Uint32(teleport[17:21]), "the fade field")
	require.EqualValues(t, 1000, int32(binary.LittleEndian.Uint32(teleport[21:25])))

	deleted := buildDeleteObjectPacket(42)
	require.Equal(t, []byte{0x1E, 0x2A, 0x00, 0x00, 0x00}, deleted)

	require.Equal(t, []byte{0x96}, buildLeaveWorldPacket())
}

// TestGameServerHoldsClientThroughBotRelogin is the core handoff
// scenario: the bot logs out (an emergency logout of the hunt loop) and
// relogs while the user client sits in the world. The client must not
// drop to the login screen: the LeaveWorld of the bot logout is
// suppressed, the connection is held and swallows the client packets,
// and the replacement session resyncs the view - the character
// teleports to its live position, the old world objects are swept and
// the enter world burst of the new session replays, followed by its
// live feed and the client packet transit.
func TestGameServerHoldsClientThroughBotRelogin(t *testing.T) {
	tightenHoldTimings(t, 2*time.Second)
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

	// The resync: the character teleports to the live position.
	teleport := client.readPacket()
	require.Equal(t, byte(0x38), teleport[0], "the teleport opcode")
	require.Len(t, teleport, 25)
	require.EqualValues(t, 1055, int32(binary.LittleEndian.Uint32(teleport[1:5])),
		"the self object id")
	require.EqualValues(t, 60000, int32(binary.LittleEndian.Uint32(teleport[5:9])),
		"the live x")
	require.EqualValues(t, 61000, int32(binary.LittleEndian.Uint32(teleport[9:13])),
		"the live y")
	require.EqualValues(t, -2500, int32(binary.LittleEndian.Uint32(teleport[13:17])),
		"the live z")
	require.Zero(t, binary.LittleEndian.Uint32(teleport[17:21]), "the fade field")
	require.EqualValues(t, 1000, int32(binary.LittleEndian.Uint32(teleport[21:25])),
		"the live heading")

	// The old world is swept: the known npcs are deleted (the order of
	// the sweep follows the world store, so both ids must simply
	// appear).
	swept := map[int32]bool{}
	for range 2 {
		deleted := client.readPacket()
		require.Equal(t, byte(0x1E), deleted[0], "the delete object opcode")
		require.Len(t, deleted, 5)
		swept[int32(binary.LittleEndian.Uint32(deleted[1:5]))] = true
	}
	require.True(t, swept[42] && swept[43],
		"the known npcs 42 and 43 must both be swept, got %v", swept)

	// The enter world burst of the replacement session replays with the
	// live self state: the played character renders at its new place.
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

// TestGameServerReloginWithoutCharSelectedServesLiveFeedOnly pins the
// defensive path of the resync: a replacement session whose CharSelected
// was never recorded cannot replay its enter world burst, so the held
// client keeps the teleport resync and follows the live feed alone.
func TestGameServerReloginWithoutCharSelectedServesLiveFeedOnly(t *testing.T) {
	tightenHoldTimings(t, 2*time.Second)
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
	// CharSelected at all.
	tracker.ResetSession()
	tracker.SetCharacter("LiveChar", 1055, 18, 61000, 62000, -2600, 80, 35)
	tracker.SetOnline("LiveChar")

	reloginSender := newFakeRawSender()
	reloginRecorder := server.RegisterSession("testbot", reloginSender, tracker)
	t.Cleanup(func() { server.UnregisterSession("testbot", reloginRecorder) })

	// The resync: the teleport and the sweep, then nothing else - the
	// history is not replayable.
	teleport := client.readPacket()
	require.Equal(t, byte(0x38), teleport[0], "the teleport opcode")
	deleted := client.readPacket()
	require.Equal(t, byte(0x1E), deleted[0], "the delete object opcode")

	// Give the relay a moment to finish the attach, then the live feed
	// alone carries the new world.
	requireConnOpen(t, client, 100*time.Millisecond)
	reloginRecorder.Record(buildTestWorldPacket(0x22, 99))
	liveSpawn := client.readPacket()
	require.Equal(t, byte(0x22), liveSpawn[0], "the live feed only")
	require.EqualValues(t, 99, int32(binary.LittleEndian.Uint32(liveSpawn[1:5])))

	// Unwind the live relay before the test returns.
	closeHeldClient(t, server, client)
}
