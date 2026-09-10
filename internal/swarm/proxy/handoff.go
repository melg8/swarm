// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package proxy

import (
	"fmt"
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
//     held client is taken through the restart dance (see
//     beginRestartSwitch): the proxy answers the restart the user never
//     clicked, offers the character list of the replacement and serves
//     the char selected answer on its own, so the client re-enters the
//     world of the same character without anyone clicking anything.
//
// A user initiated logout keeps the classic flow: the client's own Logout
// packet transits to the real server, the LeaveWorld answer is relayed so
//     the client returns to the login screen itself, and the session end
// then closes the client connection.

// Server packet opcodes the handoff synthesizes or suppresses (the C1
// values of the Mobius ServerPackets enum).
const (
	handoffOpLeaveWorld      = 0x96
	handoffOpRestartResponse = 0x74
)

// Client game packet opcode of the logout request.
const clientOpLogout = 0x09

// Client game packet opcode of the keepalive request (RequestNetPing)
// and the server packet opcode of its answer (NetPing) - the C1 values
// of the Mobius ClientPackets/ServerPackets enums.
const (
	clientOpNetPing = 0xA8
	serverOpNetPing = 0xEC
)

// isNetPingAnswerPayload reports whether the payload is the NetPing
// answer of the bot session's own keepalive. Those answers are per
// connection round trips between the bot and the real server: they
// describe no world state, so the relay never forwards them to a
// client (the client keepalive is answered locally instead, see
// transitToServer - relaying them arms the ping feedback loop where an
// answer driven client re-pings per delivered answer, the transit
// reaches the server through the bot session, the new answer is
// recorded and relayed again, and the loop accelerates to tens of
// thousands of packets per second on the attached bot).
func isNetPingAnswerPayload(payload []byte) bool {
	return len(payload) > 0 && payload[0] == serverOpNetPing
}

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

// botSwitchAutoSelectDelay is how long the restart dance waits before
// the proxy serves the char selected answer of the offered character
// itself: the client needs the time to tear its world down and render
// the char select screen, and the unsolicited answer lands exactly like
// the user's own double click of the only listed character. A variable
// so the tests can shorten it.
var botSwitchAutoSelectDelay = 1500 * time.Millisecond

// switchTimings snapshots the switch tunables under the knob mutex (the
// same guard pattern as holdTimings: the tests rewrite the delay while a
// relay of a previous connection may still be arming a timer).
func switchTimings() time.Duration {
	holdKnobsMu.Lock()
	defer holdKnobsMu.Unlock()

	return botSwitchAutoSelectDelay
}

// holdForRelogin parks the relay of a client whose bot session ended:
// it waits for a replacement session of the same bot to enter the world
// with its char selected answer recorded (the login handshake precedes
// the world entry, so an online session always has it). It returns the
// replacement session, or nil when the client went away or the hold
// timed out (the connection is already closed then).
func (gc *gameConn) holdForRelogin(old *botSession) *botSession {
	timeout, poll := holdTimings()
	gc.setHolding(true)
	defer gc.setHolding(false)

	gc.server.logger.Printf(
		"game#%d: the bot session %q ended, holding the client for its relogin "+
			"(up to %s)",
		gc.id, old.id, timeout)

	deadline := time.Now().Add(timeout)
	for {
		if next := gc.server.sessionByID(old.id); next != nil &&
			next != old && next.recorder != old.recorder &&
			switchReady(next) {
			gc.server.logger.Printf(
				"game#%d: bot %q is back online, restarting the held client onto it",
				gc.id, next.id)

			return next
		}
		select {
		case <-gc.done:
			return nil
		case <-time.After(poll):
		}
		if time.Now().After(deadline) {
			gc.shutdown(fmt.Sprintf(
				"the bot %q did not return within %s, releasing the held client",
				old.id, timeout))

			return nil
		}
	}
}

// switchReady reports whether a bot session can take a client through
// the restart dance right now: the character is in the world and the
// char selected answer of its login is recorded (the dance serves it to
// the client as the double click answer, so it must exist).
func switchReady(session *botSession) bool {
	return session.tracker.Status() == state.StatusOnline &&
		session.recorder.FirstPacketSeq(charSelectedOpcode) != 0
}

// buildLeaveWorldPacket builds the C1 LeaveWorld packet: the bare opcode
// 0x96, no body (see LeaveWorld.writeImpl - STATIC_PACKET). The client
// that processes it returns to the login screen on its own.
func buildLeaveWorldPacket() []byte {
	return []byte{handoffOpLeaveWorld}
}

// buildRestartResponsePacket builds the C1 RestartResponse packet the
// real server answers the in-game Restart button with (see
// RequestRestart.handlePacket and RestartResponse.writeImpl of the
// Mobius C1 server): [opcode 0x74][result: 1]. The client that processes
// it tears its world down and returns to the character select screen on
// its own - the one mid-session state transition the C1 client is
// designed to make, and the carrier of every character switch the proxy
// performs.
func buildRestartResponsePacket() []byte {
	return []byte{handoffOpRestartResponse, 0x01, 0x00, 0x00, 0x00}
}

// isLeaveWorldPayload reports whether the payload is the LeaveWorld
// packet the server sends when the bot session is leaving.
func isLeaveWorldPayload(payload []byte) bool {
	return len(payload) > 0 && payload[0] == handoffOpLeaveWorld
}
