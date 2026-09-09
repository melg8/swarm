// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package proxy

import (
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
)

// The bot relogin handoff: a client sitting in the world must survive the
// session cycle of its bot. The hunt loop logs the character out when the
// situation turns hopeless (the emergency logout) and the supervisor logs
// it back in a few seconds later; the client user did nothing and must not
// be kicked to the login screen for that. The pieces:
//
//   - the relay suppresses the LeaveWorld the real server answers to the
//     bot's logout (a client that processes it drops to the login screen
//     by itself), unless the client asked for the logout itself;
//   - when the session's recorder closes, the relay holds the connection
//     open and swallows the client packets (the character is offline, the
//     world behind the client is frozen);
//   - once the replacement session of the same bot enters the world, the
//     held client is resynced: the played character teleports to its live
//     position, every object of the old known list is deleted, and the
//     enter world stream of the new session replays with the live self
//     state patch - exactly the view a fresh client would get, minus the
//     login screens.
//
// A user initiated logout keeps the classic flow: the client's own Logout
// packet transits to the real server, the LeaveWorld answer is relayed so
// the client returns to the login screen itself, and the session end then
// closes the client connection.

// Server packet opcodes the handoff synthesizes or suppresses (the C1
// values of the Mobius ServerPackets enum).
const (
	handoffOpLeaveWorld    = 0x96
	handoffOpDeleteObject  = 0x1E
	handoffOpTeleportToLoc = 0x38
)

// Client game packet opcode of the logout request.
const clientOpLogout = 0x09

// clientHoldTimeout bounds how long a held client waits for its bot to
// come back before the connection is closed. The supervisor reconnects
// with a 2..30 s backoff plus the emergency logout cooldown, so a healthy
// relogin lands within seconds; a dead server keeps the client waiting
// for the full window (a frozen world beats a login screen the user did
// not ask for). A variable so the tests can shorten it.
var clientHoldTimeout = 2 * time.Minute

// holdPollPeriod is how often the hold checks for the replacement
// session. A variable so the tests can tighten it.
var holdPollPeriod = 250 * time.Millisecond

// holdKnobsMu guards the two tunables above: the tests rewrite them at
// the cleanup time of one test while the relay goroutine of a previous
// connection (its hold still unwinding) may be reading them - a mutex
// keeps the knob access race-free for the detector and honest for the
// production code (mutable package knobs read from goroutines).
var holdKnobsMu sync.Mutex

// holdTimings snapshots the hold tunables under the knob mutex.
func holdTimings() (timeout time.Duration, poll time.Duration) {
	holdKnobsMu.Lock()
	defer holdKnobsMu.Unlock()

	return clientHoldTimeout, holdPollPeriod
}

// maxReplaySeq is the sequence sentinel that starts a stream at the live
// feed alone (no recorded entry can follow it): the defensive path of
// the handoff for a replacement session whose CharSelected was never
// recorded.
const maxReplaySeq = math.MaxInt64

// holdForRelogin parks the relay of a client whose bot session ended:
// it snapshots the old known list (the tracker of the old session is
// cleared by the next login, and the sweep must run against what the
// client actually saw), then waits for a replacement session of the same
// bot to enter the world. It returns the replacement session together
// with the old known object ids, or nil when the client went away or the
// hold timed out (the connection is already closed then).
func (gc *gameConn) holdForRelogin(old *botSession) (*botSession, []int32) {
	timeout, poll := holdTimings()
	oldKnowns := old.tracker.KnownObjectIDs()
	gc.setHolding(true)
	defer gc.setHolding(false)

	gc.server.logger.Printf(
		"game#%d: the bot session %q ended, holding the client for its relogin "+
			"(%d known objects, up to %s)",
		gc.id, old.id, len(oldKnowns), timeout)

	deadline := time.Now().Add(timeout)
	for {
		if next := gc.server.sessionByID(old.id); next != nil &&
			next != old && next.recorder != old.recorder &&
			next.tracker.Status() == state.StatusOnline {
			gc.server.logger.Printf(
				"game#%d: bot %q is back online, resyncing the held client",
				gc.id, next.id)

			return next, oldKnowns
		}
		select {
		case <-gc.done:
			return nil, nil
		case <-time.After(poll):
		}
		if time.Now().After(deadline) {
			gc.shutdown(fmt.Sprintf(
				"the bot %q did not return within %s, releasing the held client",
				old.id, timeout))

			return nil, nil
		}
	}
}

// resyncWorld brings the held client onto the replacement session: the
// played character teleports to its live position (the client also clears
// its own known list on the teleport, but the sweep below does not rely
// on it), every object of the old known list is deleted (a DeleteObject
// of an unknown id is a no-op on the client, so the sweep is safe either
// way), and the session reference of the connection swaps so the client
// packets transit to the live bot connection again. The enter world
// stream of the new session replays right after (streamSession), which
// re-populates the surroundings with the live objects.
func (gc *gameConn) resyncWorld(
	next *botSession, oldKnowns []int32,
) bool {
	live := next.tracker.SelfSnapshot()
	selfID := live.ObjectID
	if selfID == 0 {
		selfID = next.tracker.SelfObjectID()
	}

	teleport := buildTeleportToLocationPacket(
		selfID, live.X, live.Y, live.Z, live.Heading)
	if !gc.relayToClient(teleport) {
		return false
	}

	swept := 0
	for _, objectID := range oldKnowns {
		if objectID == selfID {
			continue
		}
		if !gc.relayToClient(buildDeleteObjectPacket(objectID)) {
			return false
		}
		swept++
	}

	gc.setSession(next)
	gc.server.logger.Printf(
		"game#%d: held client resynced to %d %d %d (self id %d), "+
			"swept %d old objects",
		gc.id, live.X, live.Y, live.Z, selfID, swept)

	return true
}

// buildTeleportToLocationPacket builds the C1 TeleportToLocation packet:
// [opcode 0x38][target object id][x][y][z][fade 0/instant 1][heading]
// (see TeleportToLocation.writeImpl of the Mobius C1 server).
func buildTeleportToLocationPacket(
	objectID int32, x int32, y int32, z int32, heading int32,
) []byte {
	payload := make([]byte, 0, 25)
	payload = append(payload, handoffOpTeleportToLoc)
	payload = binary.LittleEndian.AppendUint32(payload, uint32(objectID))
	payload = binary.LittleEndian.AppendUint32(payload, uint32(x))
	payload = binary.LittleEndian.AppendUint32(payload, uint32(y))
	payload = binary.LittleEndian.AppendUint32(payload, uint32(z))
	payload = binary.LittleEndian.AppendUint32(payload, 0) // fade
	payload = binary.LittleEndian.AppendUint32(payload, uint32(heading))

	return payload
}

// buildDeleteObjectPacket builds the C1 DeleteObject packet:
// [opcode 0x1E][object id] (see DeleteObject.writeImpl).
func buildDeleteObjectPacket(objectID int32) []byte {
	payload := make([]byte, 0, 5)
	payload = append(payload, handoffOpDeleteObject)
	payload = binary.LittleEndian.AppendUint32(payload, uint32(objectID))

	return payload
}

// buildLeaveWorldPacket builds the C1 LeaveWorld packet: the bare opcode
// 0x96, no body (see LeaveWorld.writeImpl - STATIC_PACKET). The client
// that processes it returns to the login screen on its own.
func buildLeaveWorldPacket() []byte {
	return []byte{handoffOpLeaveWorld}
}

// isLeaveWorldPayload reports whether the payload is the LeaveWorld
// packet the server sends when the bot session is leaving.
func isLeaveWorldPayload(payload []byte) bool {
	return len(payload) > 0 && payload[0] == handoffOpLeaveWorld
}
