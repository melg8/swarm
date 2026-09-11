// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// TestBotDumpEndpoint pins the state dump report: the plain text
// carries the character sheet, the position, the inventory, the
// objects around, the walk plan and the deep event window - the
// debugging material the HUD button copies to the clipboard.
func TestBotDumpEndpoint(t *testing.T) {
	server, bot := newTestServer(t)
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 40},
	})
	bot.ApplyWaitType(state.WaitType{ObjectID: 100, Sitting: true})
	bot.RecordEvent("sat down to rest")
	bot.RecordEvent("Hunt: outside the hunting zone, pathfinding back")
	bot.SetHuntingZone(46112, 41500, 450)
	bot.SetWalkPlan(state.WalkPlan{
		Origin: &state.WalkPoint{X: 45800, Y: 41700, Z: -3500},
		Points: []state.WalkPoint{
			{X: 46000, Y: 41600, Z: -3500},
			{X: 46112, Y: 41500, Z: -3510},
		},
		Index: 1,
		Dest:  &state.WalkPoint{X: 46150, Y: 41480, Z: -3512},
	})

	recorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet, "/api/bots/test1/dump", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "text/plain; charset=utf-8",
		recorder.Header().Get("Content-Type"))
	report := recorder.Body.String()

	// The header identifies the code state and the session.
	require.Contains(t, report, "swarm state dump")
	require.Contains(t, report, "build: ")
	require.Contains(t, report, "bot: test1 (status online")

	// The character sheet: the position, the vitals, the sit state.
	require.Contains(t, report, "name: test1 (object 100)")
	require.Contains(t, report, "position: x 45000, y 50000, z -3500")
	require.Contains(t, report, "sitting true")

	// The objects around, the walk plan and the zone.
	require.Contains(t, report, "Keltir")
	require.Contains(t, report,
		"walk plan (2 waypoints, aiming at wp 1):")
	require.Contains(t, report, "  from 45800 41700 -3500")
	require.Contains(t, report, "wp 0: 46000 41600 -3500 (passed)")
	require.Contains(t, report, "wp 1: 46112 41500 -3510  <-- TARGET")
	require.Contains(t, report, "  dest 46150 41480 -3512")
	require.Contains(t, report, "hunting zone: center 46112 41500, half 450")

	// The event log mirrors the game events and the hunt decisions.
	require.Contains(t, report, "sat down to rest")
	require.Contains(t, report, "Hunt: outside the hunting zone")
}

// TestBotDumpUnknownBot: the dump of an unknown bot is a 404 like the
// other per bot endpoints.
func TestBotDumpUnknownBot(t *testing.T) {
	server, _ := newTestServer(t)
	recorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet, "/api/bots/nobody/dump", nil))

	require.Equal(t, http.StatusNotFound, recorder.Code)
}

// TestBuildStateDumpEventWindow pins the deep event window: the dump
// reads up to dumpEventLimit events where the snapshot streams one
// hundred - bounded by the ring capacity (512) when more landed, the
// full story of a stuck session stays readable.
func TestBuildStateDumpEventWindow(t *testing.T) {
	bot := state.NewBot("acc1")
	bot.SetCharacter("acc1", 100, 18, 45000, 50000, -3500, 50, 30)
	for i := range 700 {
		bot.RecordEvent("event " + strconv.Itoa(i) + " " +
			strings.Repeat("x", 300))
	}
	report := BuildStateDump(bot)
	require.Contains(t, report, "events (512):")
	require.Contains(t, report, "event 699 ",
		"the newest events survive")
	require.Contains(t, report, "event 188 ",
		"the ring head is the oldest event kept")
	require.NotContains(t, report, "event 187 ",
		"the ring evicted the older half")
	require.NotContains(t, report, "event 0 ",
		"the first events rolled out of the ring")
}

// TestDumpSlotNames pins the body part mask labels of the equipment
// lines against the Mobius BodyPart enum: the jewel and armor masks
// (0x40 head, 0x08 neck, 0x800 legs, 0x1000 feet) once printed as
// shifted slot names (a Cloth Cap read as lfinger, a Necklace of Magic
// as lear ear) and made a healthy paperdoll look corrupted in the
// field reports.
func TestDumpSlotNames(t *testing.T) {
	require.Equal(t, "underwear", dumpSlotName(0x01))
	require.Equal(t, "rear ear", dumpSlotName(0x02))
	require.Equal(t, "lear ear", dumpSlotName(0x04))
	require.Equal(t, "earring", dumpSlotName(0x06))
	require.Equal(t, "necklace", dumpSlotName(0x08))
	require.Equal(t, "rfinger", dumpSlotName(0x10))
	require.Equal(t, "lfinger", dumpSlotName(0x20))
	require.Equal(t, "ring", dumpSlotName(0x30))
	require.Equal(t, "head", dumpSlotName(0x40))
	require.Equal(t, "rhand", dumpSlotName(0x80))
	require.Equal(t, "lhand", dumpSlotName(0x100))
	require.Equal(t, "gloves", dumpSlotName(0x200))
	require.Equal(t, "chest", dumpSlotName(0x400))
	require.Equal(t, "legs", dumpSlotName(0x800))
	require.Equal(t, "feet", dumpSlotName(0x1000))
	require.Equal(t, "back", dumpSlotName(0x2000))
	require.Equal(t, "lrhand", dumpSlotName(0x4000))
	require.Equal(t, "full armor", dumpSlotName(0x8000))
	require.Equal(t, "hair", dumpSlotName(0x10000))
	require.Equal(t, "part 0x20000", dumpSlotName(0x20000))
}
