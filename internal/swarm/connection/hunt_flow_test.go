// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package connection

import (
	"context"
	"encoding/binary"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/crypt"
	"github.com/melg8/swarm/internal/swarm/hunt"
	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// The hunt flow reproduction: the fake server below implements the exact
// Mobius AttackRequest semantics extracted from the server sources (see
// docs/development_log.md):
//
//   AttackRequest.runImpl: when the requested target is not the current
//   target, the server resolves NpcClick.onAction, which only selects the
//   target (player.setTarget + MyTargetSelected); when the requested
//   target is already the current target, the server calls
//   onForcedAttack, which notifies the player AI with the ATTACK
//   intention and the fight begins.
//
// A bot that sends exactly one AttackRequest therefore never starts
// fighting: it selects the target and stands still, which is the reported
// -hunt bug. The test drives the real GameClient with the hunt loop and
// asserts the bot performs the second request that starts the combat.

// huntFlowServer is a scripted game server with the Mobius click
// semantics and a fight that only starts on the second request. It
// embeds the fakeGameServer helpers for the handshake, the character
// flow and the framed encrypted reads and writes.
type huntFlowServer struct {
	*fakeGameServer

	mu                sync.Mutex
	attackRequests    []int32 // target ids of every AttackRequest, in order
	combatStarted     bool
	requestsAfterStop int32 // requests received after the combat started
	ready             chan struct{}
}

// startHuntFlowServer starts the hunt flow server on a random port.
func startHuntFlowServer(t *testing.T) *huntFlowServer {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := &huntFlowServer{
		fakeGameServer: &fakeGameServer{listener: listener, t: t},
		ready:          make(chan struct{}),
	}
	go server.serveHunt()

	return server
}

// Addr returns the address of the fake server.
func (s *huntFlowServer) Addr() string {
	return s.listener.Addr().String()
}

// requests returns a copy of the received AttackRequest target ids.
func (s *huntFlowServer) requests() []int32 {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]int32(nil), s.attackRequests...)
}

// fighting reports whether the forced attack started the combat.
func (s *huntFlowServer) fighting() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.combatStarted
}

// serveHunt runs the single scripted session.
func (s *huntFlowServer) serveHunt() {
	conn, err := s.listener.Accept()
	if err != nil {
		return
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(60 * time.Second))

	serverCrypt := s.handshake(conn)
	s.characterFlow(conn, serverCrypt)
	s.huntFlow(conn, serverCrypt)
}

// huntFlow exchanges the enter world packets and implements the Mobius
// AttackRequest semantics until the bot leaves the world.
func (s *huntFlowServer) huntFlow(conn net.Conn, cipher *crypt.GameCrypt) {
	s.writeEncrypted(conn, cipher, buildCharSelected())

	payload := s.readEncrypted(conn, cipher)
	require.Equal(s.t, byte(0x03), payload[0]) // EnterWorld

	s.writeEncrypted(conn, cipher,
		append([]byte{0xEC}, 0x0A, 0x00, 0x00, 0x00))

	// The world around the character: the self placement and one
	// attackable npc well inside the attack search radius.
	s.writeEncrypted(conn, cipher,
		buildValidateLocation(100, 45050, 50050, 16384))
	s.writeEncrypted(conn, cipher, buildNpcInfo(1, "Keltir", 45100, 50100, 8192))
	close(s.ready)

	currentTarget := int32(0)
	for {
		payload, err := s.readEncryptedResult(conn, cipher)
		if err != nil || len(payload) == 0 {
			return
		}
		switch payload[0] {
		case 0x09: // Logout
			return
		case 0x0A: // AttackRequest
			target := int32(binary.LittleEndian.Uint32(payload[1:5]))
			s.mu.Lock()
			s.attackRequests = append(s.attackRequests, target)
			started := s.combatStarted
			if started {
				s.requestsAfterStop++
			}
			s.mu.Unlock()
			if currentTarget != target {
				// First click: the server only selects the target
				// (NpcClick.onAction -> setTarget + MyTargetSelected).
				currentTarget = target
				s.writeEncrypted(conn, cipher, buildMyTargetSelected(target))
			} else if !started {
				// Second click on the already selected target:
				// onForcedAttack -> the AI starts the attack.
				s.mu.Lock()
				s.combatStarted = true
				s.mu.Unlock()
				s.writeEncrypted(conn, cipher, buildAutoAttackStart(100))
				s.writeEncrypted(conn, cipher, buildSelfAttack(1))
			}
		default:
			// Net ping and other client packets are ignored.
		}
	}
}

// buildCharSelected builds the CharSelected packet of the test character.
func buildCharSelected() []byte {
	selected := []byte{0x21}
	selected = append(selected, utf16Bytes("test1")...)
	selected = binary.LittleEndian.AppendUint32(selected, 100)
	selected = append(selected, utf16Bytes("")...)
	selected = binary.LittleEndian.AppendUint32(selected, 42)
	for range 5 {
		selected = binary.LittleEndian.AppendUint32(selected, 0)
	}
	selected = binary.LittleEndian.AppendUint32(selected, 1) // active
	selected = binary.LittleEndian.AppendUint32(selected, 45000)
	selected = binary.LittleEndian.AppendUint32(selected, 50000)
	selected = append(selected, 0x68, 0xF2, 0xFF, 0xFF)       // z = -3500
	selected = binary.LittleEndian.AppendUint64(selected, 50) // cur hp
	selected = binary.LittleEndian.AppendUint64(selected, 30) // cur mp

	return selected
}

// buildMyTargetSelected builds the MyTargetSelected answer of the first
// click: [0xBF][objectId: 4][color: 2].
func buildMyTargetSelected(objectID int32) []byte {
	data := []byte{0xBF}
	data = appendInt32(data, objectID)

	return append(data, 0, 0)
}

// buildAutoAttackStart builds the AutoAttackStart broadcast:
// [0x3B][objectId: 4].
func buildAutoAttackStart(objectID int32) []byte {
	data := []byte{0x3B}

	return appendInt32(data, objectID)
}

// buildSelfAttack builds the Attack packet of the played character
// hitting the target once: [0x06][attackerId: 4][targetId: 4][damage: 4]
// [flags: 1][x: 4][y: 4][z: 4][hitsLeft: 2][targetX: 4][targetY: 4]
// [targetZ: 4].
func buildSelfAttack(targetID int32) []byte {
	data := []byte{0x06}
	data = appendInt32(data, 100)
	data = appendInt32(data, targetID)
	data = appendInt32(data, 5) // damage
	data = append(data, 0)      // hit flags
	data = appendInt32(data, 45050)
	data = appendInt32(data, 50050)
	data = appendInt32(data, -3500)
	data = binary.LittleEndian.AppendUint16(data, 0) // remaining hits
	data = appendInt32(data, 45100)
	data = appendInt32(data, 50100)
	data = appendInt32(data, -3500)

	return data
}

// startHuntBot connects a bot with the hunt loop against the server.
func startHuntBot(
	t *testing.T, address string,
) (*state.Bot, context.CancelFunc, chan error) {
	t.Helper()

	conn, err := net.Dial("tcp", address)
	require.NoError(t, err)

	client, err := NewGameClient(conn)
	require.NoError(t, err)

	tracker := state.NewBot("test1")
	client.SetTracker(tracker)

	charList, err := client.Authenticate(GameSessionParams{
		Account:    "test1",
		LoginOkID1: 1,
		LoginOkID2: 2,
		PlayOkID1:  3,
		PlayOkID2:  4,
	})
	require.NoError(t, err)

	updated, err := client.EnsureCharacter(CharacterParams{
		Name:      "test1",
		Race:      1,
		Female:    0,
		ClassID:   18,
		HairStyle: 0,
		HairColor: 0,
		Face:      0,
	}, charList)
	require.NoError(t, err)

	slot, _, found := updated.FindCharacterByName("test1")
	require.True(t, found)
	require.NoError(t, client.EnterWorld(int32(slot))) //nolint:gosec // small

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		hunt.NewLoop(client, tracker).Run(ctx)
	}()
	go func() {
		done <- client.Run(ctx, "test1")
	}()

	return tracker, cancel, done
}

func TestGameClientHuntFlowStartsAttack(t *testing.T) {
	server := startHuntFlowServer(t)

	tracker, cancel, done := startHuntBot(t, server.Addr())
	defer cancel()

	// The fight must start within a few hunt ticks: the first request
	// selects the target, the second one triggers the forced attack.
	deadline := time.Now().Add(10 * time.Second)
	for !server.fighting() && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}

	cancel()
	_ = <-done

	t.Logf("attack requests received: %v", server.requests())
	require.True(t, server.fighting(),
		"the hunt loop must send the second attack request that starts "+
			"the attack after MyTargetSelected confirmed the target")

	requests := server.requests()
	require.GreaterOrEqual(t, len(requests), 2,
		"the Mobius AttackRequest semantics need two requests: one to "+
			"select, one to attack")
	require.Equal(t, int32(1), requests[0])
	for _, target := range requests {
		require.Equal(t, int32(1), target,
			"every attack request must target the selected npc")
	}

	// The combat state must be visible in the bot snapshot.
	snapshot := tracker.Snapshot()
	require.True(t, snapshot.Character.InCombat,
		"the played character must count as fighting")

	// Once the fight runs, the loop must not keep spamming requests.
	server.mu.Lock()
	after := server.requestsAfterStop
	server.mu.Unlock()
	require.LessOrEqual(t, after, int32(1),
		"the loop must stop re-requesting once the combat started")
}
