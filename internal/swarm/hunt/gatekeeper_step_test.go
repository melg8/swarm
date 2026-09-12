// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"strings"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// gatekeeperTestHTML is the teleport list the fake server returns for
// Mirabel (npc 30146): two NORMAL destinations, the first one Gludio.
const gatekeeperTestHTML = "<html><body>Region where teleporting is possible<br><br>" +
	"<a action=\"bypass -h npc_30146_teleport NORMAL 0\" msg=\"811;The Town of Gludio\">" +
	"The Town of Gludio - 9200 Adena</a><br1>" +
	"<a action=\"bypass -h npc_30146_teleport NORMAL 1\" msg=\"811;Dwarven Village\">" +
	"Dwarven Village - 23000 Adena</a><br1>" +
	"<br></body></html>"

// TestDriveGatekeeperTeleportHappyPath pins the full step: the bot
// sends the showTeleports bypass, the fake server delivers the teleport
// list html, the step finds the Gludio button and sends the teleport
// bypass. The connection layer (the arrival TeleportToLocation + the
// Appearing) is out of scope of this unit test.
func TestDriveGatekeeperTeleportHappyPath(t *testing.T) {
	game := &fakeGame{}
	// Simulate the server delivering the teleport list a moment after
	// the showTeleports bypass: the awaitDialog loop reads the html.
	go func() {
		time.Sleep(100 * time.Millisecond)
		game.htmlNPC = 30146
		game.htmlBody = gatekeeperTestHTML
	}()
	loop := NewLoop(game, state.NewBot("gatekeeper"))

	err := loop.DriveGatekeeperTeleport(30146, "NORMAL", "The Town of Gludio")
	require.NoError(t, err)
	require.Len(t, game.bypasses, 2,
		"the step sends the showTeleports then the teleport bypass")
	require.Equal(t, "npc_30146_showTeleports", game.bypasses[0])
	require.Equal(t, "npc_30146_teleport NORMAL 0", game.bypasses[1])
}

// TestDriveGatekeeperTeleportDialogTimeout pins the wait guard: when
// the server never delivers the teleport list, the step fails after
// the dialog wait instead of hanging.
func TestDriveGatekeeperTeleportDialogTimeout(t *testing.T) {
	// Shorten the wait so the test runs in under a second.
	original := gatekeeperDialogWait
	gatekeeperDialogWait = 200 * time.Millisecond
	t.Cleanup(func() { gatekeeperDialogWait = original })

	game := &fakeGame{}
	loop := NewLoop(game, state.NewBot("gatekeeper"))
	err := loop.DriveGatekeeperTeleport(30146, "NORMAL", "Gludio")
	require.Error(t, err)
	require.Contains(t, err.Error(), "dialog never arrived")
	require.Len(t, game.bypasses, 1,
		"the showTeleports bypass was sent before the wait failed")
}

// TestDriveGatekeeperTeleportDestinationMissing pins the lookup guard:
// when the teleport list has no button matching the wanted
// destination, the step fails with a clear reason.
func TestDriveGatekeeperTeleportDestinationMissing(t *testing.T) {
	game := &fakeGame{}
	game.htmlNPC = 30146
	game.htmlBody = gatekeeperTestHTML
	loop := NewLoop(game, state.NewBot("gatekeeper"))

	err := loop.DriveGatekeeperTeleport(30146, "NORMAL", "Giran")
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "Giran") ||
		strings.Contains(err.Error(), "no teleport button"),
		"the error names the missing destination")
}

// TestDriveGatekeeperTeleportEmptyCommandRefused pins the bypass guard
// inherited from SendBypass: the step never sends an empty command.
func TestDriveGatekeeperTeleportStaleHtmlIgnored(t *testing.T) {
	// A stale html from a previous npc (30000) must not satisfy the
	// await of npc 30146; the step waits for the matching dialog.
	game := &fakeGame{
		htmlNPC:  30000,
		htmlBody: "<a action=\"bypass npc_30000_Chat\">old</a>",
	}
	go func() {
		time.Sleep(100 * time.Millisecond)
		game.htmlNPC = 30146
		game.htmlBody = gatekeeperTestHTML
	}()
	loop := NewLoop(game, state.NewBot("gatekeeper"))
	err := loop.DriveGatekeeperTeleport(30146, "NORMAL", "The Town of Gludio")
	require.NoError(t, err)
}
