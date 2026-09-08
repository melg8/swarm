// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package proxy

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/connection"
	fromgameserver "github.com/melg8/swarm/internal/swarm/packets/from_game_server"
	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// The E2E suite drives a fake C1 client through the whole proxy against
// the deployed Mobius C1 stack: the bot session enters the world, the
// client logs in with arbitrary credentials, sees the one character,
// enters the world through the replay and exchanges live packets with
// the real server through the relay. It needs the deployed stack
// (tools/swarm_fast_deploy.sh, ports 2106/7777/3306) and the explicit
// opt in:
//
//	SWARM_PROXY_E2E=1 go test ./internal/swarm/proxy/ \
//	    -run TestProxyE2E -v -timeout 5m
//
// tools/proxy_e2e.sh wraps the whole procedure.
const (
	e2eLoginAddress = "127.0.0.1:2106"
	e2eAccount      = "proxye2e"
	e2ePassword     = "test"
	e2eOnlineWait   = 60 * time.Second
	e2eReadTimeout  = 60 * time.Second
	e2eLiveWait     = 30 * time.Second
)

// TestProxyE2ERealStackClientFlow verifies the full MITM path against
// the live server: the login emulation, the one character list, the
// recorded char selected packet, the replay, the live relay and the
// client packet transit through the bot session.
func TestProxyE2ERealStackClientFlow(t *testing.T) {
	if os.Getenv("SWARM_PROXY_E2E") != "1" {
		t.Skip("set SWARM_PROXY_E2E=1 and deploy the stack with " +
			"tools/swarm_fast_deploy.sh to run the live proxy E2E")
	}
	requireStackPort(t, 2106)
	requireStackPort(t, 7777)

	logDir := t.TempDir()
	logPath := filepath.Join(logDir, "proxy.log")
	logFile, err := os.Create(logPath)
	require.NoError(t, err)
	logger := log.New(logFile, "proxy ", log.LstdFlags|log.Lmicroseconds)
	t.Cleanup(func() {
		if t.Failed() {
			content, readErr := os.ReadFile(logPath)
			if readErr == nil {
				t.Logf("---- proxy.log ----\n%s\n---- end ----", content)
			}
		}
	})

	server := NewServer(logger,
		WithLoginAddresses("127.0.0.1:0"),
		WithGameAddresses("127.0.0.1:0"))
	require.NoError(t, server.Listen())
	go func() {
		_ = server.Serve()
	}()
	t.Cleanup(func() { _ = server.Shutdown(context.Background()) })

	// The bot session: the same wiring runBot uses (login, game
	// handshake, character, world entry, proxy registration).
	bot := state.NewBot(e2eAccount)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	sessionDone := make(chan error, 1)
	go func() {
		sessionDone <- runE2EBotSession(ctx, server, bot, logger)
	}()
	requireOnline(t, bot)

	// --- the client side: login through the emulated login server ---
	loginClient := dialLogin(t, server.LoginAddr())
	initPacket := loginClient.readInit()
	loginOk := loginClient.login("whatever", "credentials")
	require.NotZero(t, loginOk.LoginOkID1)
	list := loginClient.requestServerList()
	require.Len(t, list.Servers, 1)
	entry := list.Servers[0]
	require.NotZero(t, entry.Status)
	playOk := loginClient.requestServerLogin(byte(entry.ServerID))
	require.NotZero(t, playOk.PlayOkID1)
	require.NotZero(t, initPacket.ProtocolVersion)

	gameAddress := fmt.Sprintf("%d.%d.%d.%d:%d",
		entry.IP[0], entry.IP[1], entry.IP[2], entry.IP[3], entry.Port)
	gameClient := dialGame(t, gameAddress)
	require.NoError(t, gameClient.conn.SetDeadline(
		time.Now().Add(e2eReadTimeout)))
	gameClient.handshake()

	// AuthLogin: the keys of the emulated login flow (any pair works).
	authLogin := []byte{0x08}
	authLogin = appendUTF16(authLogin, "whatever")
	authLogin = binary.LittleEndian.AppendUint32(authLogin,
		uint32(playOk.PlayOkID2))
	authLogin = binary.LittleEndian.AppendUint32(authLogin,
		uint32(playOk.PlayOkID1))
	authLogin = binary.LittleEndian.AppendUint32(authLogin,
		uint32(loginOk.LoginOkID1))
	authLogin = binary.LittleEndian.AppendUint32(authLogin,
		uint32(loginOk.LoginOkID2))
	gameClient.sendPacket(authLogin)

	// The char list offers exactly the played character of the bot.
	charListPayload := gameClient.readPacket()
	require.Equal(t, byte(0x1F), charListPayload[0])
	charList := fromgameserver.NewCharSelectInfoPacket()
	require.NoError(t, fromgameserver.ParseCharSelectInfoPacket(
		charList, charListPayload))
	require.Len(t, charList.Characters, 1)
	require.Equal(t, e2eAccount, charList.Characters[0].Name)
	require.Positive(t, charList.Characters[0].Level)

	// Character selection answers with the recorded packet of the bot.
	gameClient.sendPacket([]byte{0x0D, 0x00, 0x00, 0x00, 0x00})
	selectedPayload := gameClient.readPacket()
	require.Equal(t, byte(0x21), selectedPayload[0])
	selected := fromgameserver.NewCharSelectedPacket()
	require.NoError(t, fromgameserver.ParseCharSelectedPacket(
		selected, selectedPayload))
	require.Equal(t, e2eAccount, selected.Name)

	// Enter world: the replay must contain the UserInfo of the bot.
	gameClient.sendPacket([]byte{0x03})
	userInfo := readPacketUntil(t, gameClient, e2eReadTimeout,
		func(payload []byte) bool { return payload[0] == 0x04 },
		"the replayed user info")
	require.GreaterOrEqual(t, len(userInfo), 5)

	// The character snapshot position feeds the client move request.
	snapshot := bot.Snapshot()
	require.Equal(t, e2eAccount, snapshot.Character.Name)
	require.NotZero(t, snapshot.Character.ObjectID)

	// Live relay: a client MoveToLocation transits to the real server,
	// which broadcasts the walk back to the session (and the relay
	// delivers it to the client).
	move := []byte{0x01}
	move = binary.LittleEndian.AppendUint32(move,
		uint32(snapshot.Character.X+100))
	move = binary.LittleEndian.AppendUint32(move,
		uint32(snapshot.Character.Y+100))
	move = binary.LittleEndian.AppendUint32(move,
		uint32(snapshot.Character.Z))
	move = binary.LittleEndian.AppendUint32(move,
		uint32(snapshot.Character.X))
	move = binary.LittleEndian.AppendUint32(move,
		uint32(snapshot.Character.Y))
	move = binary.LittleEndian.AppendUint32(move,
		uint32(snapshot.Character.Z))
	// The movement mode is a full int (1 = mouse click, see
	// MoveToLocation.readImpl).
	move = binary.LittleEndian.AppendUint32(move, 1)
	gameClient.sendPacket(move)

	echo := readPacketUntil(t, gameClient, e2eLiveWait,
		func(payload []byte) bool {
			return payload[0] == 0x01 && len(payload) >= 5 &&
				int32(binary.LittleEndian.Uint32(payload[1:5])) ==
					snapshot.Character.ObjectID
		}, "the own movement echo through the live relay")
	require.Equal(t, byte(0x01), echo[0])

	// --- the client keepalive: the C1 client pings continuously ---
	// A burst of RequestNetPing packets transits through the bot
	// session and every answer must return through the relay (the
	// connection stays alive). The session log must stay silent
	// about them: one log line per answer used to flood the
	// process log with several "Net ping with game time" lines
	// per second whenever a real client was attached.
	const pingBurst = 10
	for range pingBurst {
		gameClient.sendPacket([]byte{0xA8})
	}
	for range pingBurst {
		reply := readPacketUntil(t, gameClient, e2eLiveWait,
			func(payload []byte) bool { return payload[0] == 0xEC },
			"the net ping answer through the live relay")
		require.Equal(t, byte(0xEC), reply[0])
	}

	// --- the reconnection: the client drops and a new one enters while
	// the bot stands at the walked-to place far from its login spot ---
	require.NoError(t, gameClient.conn.Close())
	moved := waitBotArrived(t, bot, snapshot.Character.X+100,
		snapshot.Character.Y+100)
	t.Logf("the bot walked to %d %d %d", moved.X, moved.Y, moved.Z)

	reconnectClient := dialGame(t, gameAddress)
	require.NoError(t, reconnectClient.conn.SetDeadline(
		time.Now().Add(e2eReadTimeout)))
	reconnectClient.handshake()

	reconnectClient.sendPacket(authLogin)

	// The char list of the reconnection carries the live paperdoll: the
	// selection screen must render the equipped gear, not a naked model.
	reconnectListPayload := reconnectClient.readPacket()
	require.Equal(t, byte(0x1F), reconnectListPayload[0])
	reconnectList := fromgameserver.NewCharSelectInfoPacket()
	require.NoError(t, fromgameserver.ParseCharSelectInfoPacket(
		reconnectList, reconnectListPayload))
	require.Len(t, reconnectList.Characters, 1)
	expectedPaperdoll := expectedPaperdollItems(t, bot)
	require.Equal(t, expectedPaperdoll,
		reconnectList.Characters[0].PaperdollItemIDs,
		"the char list paperdoll must mirror the live equipment")
	liveEntry := reconnectList.Characters[0]
	require.InDelta(t, moved.X, liveEntry.X, 300,
		"the char list carries the walked-to position")
	require.InDelta(t, moved.Y, liveEntry.Y, 300)

	// The char selected answer is patched to the live position: the
	// client spawns where the character actually stands.
	reconnectClient.sendPacket([]byte{0x0D, 0x00, 0x00, 0x00, 0x00})
	reconnectSelectedPayload := reconnectClient.readPacket()
	require.Equal(t, byte(0x21), reconnectSelectedPayload[0])
	reconnectSelected := fromgameserver.NewCharSelectedPacket()
	require.NoError(t, fromgameserver.ParseCharSelectedPacket(
		reconnectSelected, reconnectSelectedPayload))
	require.Equal(t, e2eAccount, reconnectSelected.Name)
	current := bot.SelfSnapshot()
	require.InDelta(t, current.X, reconnectSelected.X, 300,
		"the char selected answer carries the live x")
	require.InDelta(t, current.Y, reconnectSelected.Y, 300,
		"the char selected answer carries the live y")
	require.InDelta(t, current.Z, reconnectSelected.Z, 600,
		"the char selected answer carries the live z")

	// The replayed UserInfo is live-patched too: the entering world view
	// of the played character matches the walked-to place.
	reconnectClient.sendPacket([]byte{0x03})
	replayUserInfoPayload := readPacketUntil(t, reconnectClient, e2eReadTimeout,
		func(payload []byte) bool { return payload[0] == 0x04 },
		"the live-patched replayed user info")
	replayUserInfo := fromgameserver.NewUserInfoPacket()
	require.NoError(t, fromgameserver.ParseUserInfoPacket(
		replayUserInfo, replayUserInfoPayload))
	require.Equal(t, current.ObjectID, replayUserInfo.ObjectID)
	require.InDelta(t, current.X, replayUserInfo.X, 300,
		"the replayed user info carries the live x")
	require.InDelta(t, current.Y, replayUserInfo.Y, 300,
		"the replayed user info carries the live y")
	require.InDelta(t, current.Level, replayUserInfo.Level, 0)
	require.Positive(t, replayUserInfo.Level)
	require.NoError(t, reconnectClient.conn.Close())

	// The proxy log file carries the client session for the debugging
	// workflow of the real client (the whole point of the file).
	logContent, err := os.ReadFile(logPath)
	require.NoError(t, err)
	logText := string(logContent)
	require.Contains(t, logText, "auth login accepted")
	require.Contains(t, logText, "replaying")
	require.Contains(t, logText, "client -> server 0x01")
	require.Contains(t, logText, "client -> server 0xa8")
	require.NotContains(t, logText, "Net ping with game time",
		"the net ping answers must stay silent in the session log")

	// The shutdown: cancel the bot, the relay winds the client down.
	cancel()
	select {
	case err := <-sessionDone:
		require.NoError(t, err)
	case <-time.After(30 * time.Second):
		t.Fatal("the bot session did not stop")
	}
}

// runE2EBotSession mirrors the runBot wiring of cmd/swarm against the
// live stack: login, game handshake, character creation, world entry
// and the proxy registration (tap, raw send path, modulus).
func runE2EBotSession(
	ctx context.Context, server *Server, tracker *state.Bot, logger *log.Logger,
) error {
	tracker.ResetSession()

	loginConn, err := net.DialTimeout("tcp", e2eLoginAddress, 10*time.Second)
	if err != nil {
		return fmt.Errorf("login dial: %w", err)
	}
	auth, err := connection.Authenticate(loginConn, e2eAccount, e2ePassword)
	if err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}
	server.SetRsaModulus(auth.RsaPublicKey)

	gameAddress := fmt.Sprintf("%d.%d.%d.%d:%d",
		auth.ServerIP[0], auth.ServerIP[1], auth.ServerIP[2], auth.ServerIP[3],
		auth.ServerPort)
	gameConn, err := net.DialTimeout("tcp", gameAddress, 10*time.Second)
	if err != nil {
		return fmt.Errorf("game dial: %w", err)
	}

	game, err := connection.NewGameClient(gameConn)
	if err != nil {
		return fmt.Errorf("game handshake: %w", err)
	}
	game.SetTracker(tracker)
	game.SetLogger(logger)

	recorder := server.RegisterSession(e2eAccount, game, tracker)
	game.SetTap(recorder.Record)
	defer server.UnregisterSession(e2eAccount, recorder)

	charList, err := game.Authenticate(connection.GameSessionParams{
		Account:    auth.Account,
		LoginOkID1: auth.LoginOkID1,
		LoginOkID2: auth.LoginOkID2,
		PlayOkID1:  auth.PlayOkID1,
		PlayOkID2:  auth.PlayOkID2,
	})
	if err != nil {
		return fmt.Errorf("game auth: %w", err)
	}
	charList, err = game.EnsureCharacter(connection.CharacterParams{
		Name:      e2eAccount,
		Race:      1,
		Female:    0,
		ClassID:   18,
		HairStyle: 0,
		HairColor: 0,
		Face:      0,
	}, charList)
	if err != nil {
		return fmt.Errorf("ensure character: %w", err)
	}
	slot, _, found := charList.FindCharacterByName(e2eAccount)
	if !found {
		return fmt.Errorf("character %s not found", e2eAccount)
	}
	if err := game.EnterWorld(int32(slot)); err != nil {
		return fmt.Errorf("enter world: %w", err)
	}

	return game.Run(ctx, e2eAccount)
}

// requireOnline waits for the bot session to enter the world.
func requireOnline(t *testing.T, bot *state.Bot) {
	t.Helper()
	deadline := time.Now().Add(e2eOnlineWait)
	for bot.Status() != state.StatusOnline {
		if time.Now().After(deadline) {
			t.Fatalf("the bot session is %s, not online after %s",
				bot.Status(), e2eOnlineWait)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// requireStackPort skips the test when the stack port is not held by
// the server: a bind attempt succeeding means nothing listens there
// (a connection probe would trip the login flood protector, so the
// readiness check binds instead, see AGENTS.md).
func requireStackPort(t *testing.T, port int) {
	t.Helper()
	address := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", address)
	if err == nil {
		if closeErr := listener.Close(); closeErr != nil {
			t.Fatalf("probe listener close: %v", closeErr)
		}
		t.Skipf("nothing listens on port %d, deploy the stack with "+
			"tools/swarm_fast_deploy.sh first", port)
	}
}

// readPacketUntil reads client packets until the predicate matches or
// the timeout expires; it fails the test with the given context.
func readPacketUntil(
	t *testing.T, client *fakeGameClient, timeout time.Duration,
	match func(payload []byte) bool, context string,
) []byte {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		require.NoError(t, client.conn.SetReadDeadline(deadline))
		payload, err := readWirePacket(client.conn, client.readBuf)
		if err != nil {
			t.Fatalf("failed to read the next packet (%s): %v", context, err)
		}
		client.readBuf = payload
		client.crypt.Decrypt(payload)
		if match(payload) {
			return payload
		}
	}
}

// waitBotArrived waits for the bot session to finish the walk the
// client transit started and returns the final character snapshot.
func waitBotArrived(
	t *testing.T, bot *state.Bot, destX int32, destY int32,
) state.CharacterSnapshot {
	t.Helper()
	deadline := time.Now().Add(e2eLiveWait)
	for {
		snapshot := bot.SelfSnapshot()
		arrived := !snapshot.Moving &&
			absDelta(snapshot.X, destX) <= 300 && absDelta(snapshot.Y, destY) <= 300
		if arrived {
			return snapshot
		}
		if time.Now().After(deadline) {
			t.Fatalf("the bot never arrived at %d %d: it stands at %d %d (moving %v)",
				destX, destY, snapshot.X, snapshot.Y, snapshot.Moving)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// expectedPaperdollItems resolves the live equipment of the bot into
// the paperdoll item id table the char list packet must carry.
func expectedPaperdollItems(t *testing.T, bot *state.Bot) [15]int32 {
	t.Helper()
	paperdoll := bot.PaperdollSlotObjectIDs()
	inventory := bot.InventoryItems()
	itemIDs := make(map[int32]int32, len(inventory))
	for _, item := range inventory {
		itemIDs[item.ObjectID] = item.ItemID
	}
	var expected [15]int32
	for i, objectID := range paperdoll {
		expected[i] = itemIDs[objectID]
	}

	return expected
}

// absDelta returns the absolute difference of two int32 values.
func absDelta(a int32, b int32) int32 {
	if a > b {
		return a - b
	}

	return b - a
}
