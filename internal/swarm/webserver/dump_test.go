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
	bot.SetWalkPlan([]state.WalkPoint{
		{X: 46000, Y: 41600, Z: -3500},
		{X: 46112, Y: 41500, Z: -3510},
	})

	recorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet, "/api/bots/test1/dump", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "text/plain; charset=utf-8",
		recorder.Header().Get("Content-Type"))
	report := recorder.Body.String()

	// The header identifies the session.
	require.Contains(t, report, "swarm state dump")
	require.Contains(t, report, "bot: test1 (status online")

	// The character sheet: the position, the vitals, the sit state.
	require.Contains(t, report, "name: test1 (object 100)")
	require.Contains(t, report, "position: x 45000, y 50000, z -3500")
	require.Contains(t, report, "sitting true")

	// The objects around, the walk plan and the zone.
	require.Contains(t, report, "Keltir")
	require.Contains(t, report, "walk plan (2 waypoints)")
	require.Contains(t, report, "wp 0: 46000 41600 -3500")
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
