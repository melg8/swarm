// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// newStatsServer builds a server with one online bot in a known
// vitals state for the statistics endpoint tests. The collector is
// sampled manually with deterministic times.
func newStatsServer(t *testing.T) (*Server, *state.Bot) {
	t.Helper()

	registry := state.NewRegistry()
	bot := state.NewBot("test1")
	// The production session order: the reset clears the tracker
	// world, then the login fills it back (runBot calls ResetSession
	// before the first login).
	bot.ResetSession()
	bot.SetCharacter("Test1", 100, 18, 45000, 50000, -3500, 80, 40)
	bot.ApplyUserInfo(state.UserInfo{
		Name:  "Test1",
		Level: 10,
		MaxHP: 100, CurHP: 80,
		MaxMP: 50, CurMP: 40,
		Exp: 5000,
	})
	bot.SetOnline("Test1")
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7, X: 45100, Y: 50100, Name: "Keltir", Attackable: true,
	})
	registry.Add(bot)

	return NewServer(registry, "127.0.0.1:0", log.New(io.Discard, "", 0)), bot
}

// statsKillMob spawns a fresh Keltir of the given object id, swings
// at it and kills it: one attributed kill lands in the tracker
// metrics (a dead mob cannot die twice, so every kill needs a fresh
// mob).
func statsKillMob(bot *state.Bot, objectID int32) {
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: objectID, X: 45100, Y: 50100,
		Name: "Keltir", Attackable: true,
	})
	bot.ApplyAttack(state.Attack{
		AttackerID:  100,
		TargetIDs:   [state.AttackTargets]int32{objectID},
		TargetCount: 1,
	})
	bot.ApplyStatusUpdate(objectID,
		[]state.Attribute{{ID: state.AttrCurHP, Value: 0}})
}

func TestStatsFleetEndpoint(t *testing.T) {
	server, bot := newStatsServer(t)
	statsKillMob(bot, 7)
	base := time.Now()
	server.stats.sample(base.Add(-time.Minute))
	statsKillMob(bot, 7)
	server.stats.sample(base)

	recorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet, "/api/stats", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	var response statsResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, int(15), response.SamplePeriodSec)
	require.Len(t, response.Bots, 1)
	require.Equal(t, "test1", response.Bots[0].ID)
	require.EqualValues(t, 2, response.Bots[0].Kills)
	require.EqualValues(t, 1, response.Bots[0].Sessions)
	require.EqualValues(t, 0, response.Bots[0].Rejoins)
	require.True(t, response.Bots[0].Online)
	require.Equal(t, "online", response.Bots[0].Status)
	require.NotEmpty(t, response.History.At)
	require.Len(t, response.History.Online, len(response.History.At))
	require.EqualValues(t, 1, response.History.Online[len(
		response.History.Online)-1])
	require.EqualValues(t, 2, response.History.Kills[len(
		response.History.Kills)-1])
	require.GreaterOrEqual(t, response.Process.Goroutines, 1)
}

func TestStatsFleetEndpointBeforeFirstSample(t *testing.T) {
	server, _ := newStatsServer(t)

	recorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet, "/api/stats", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	var response statsResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	// The live counters answer even without a collected sample; the
	// history stays empty until the sampler ran.
	require.Len(t, response.Bots, 1)
	require.Empty(t, response.History.At)
	require.Equal(t, int64(0), response.Fleet.Kills)
}

func TestStatsBotEndpoint(t *testing.T) {
	server, bot := newStatsServer(t)
	now := time.Now()
	server.stats.sample(now.Add(-2 * time.Minute))
	statsKillMob(bot, 7)
	bot.SetPhase("engage")
	server.stats.sample(now.Add(-time.Minute))
	statsKillMob(bot, 8)
	server.stats.sample(now)

	recorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet, "/api/stats/test1", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	var response botStatsResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "test1", response.ID)
	require.EqualValues(t, 2, response.Kills)
	require.Len(t, response.Events, 2)
	require.Equal(t, eventKill, response.Events[0].Kind)
	require.EqualValues(t, 1, response.Events[0].Value)
	require.Equal(t, eventKill, response.Events[1].Kind)
	require.NotEmpty(t, response.History.At)
	require.Len(t, response.History.Kills, len(response.History.At))
	require.EqualValues(t, 2, response.History.Kills[len(
		response.History.Kills)-1])
	require.Equal(t, "engage", response.Phase)
	require.NotEmpty(t, response.History.Phase)
	require.NotEmpty(t, response.Phases)
	var engageSeconds float64
	for _, share := range response.Phases {
		if share.Phase == "engage" {
			engageSeconds = share.Seconds
		}
	}
	// The engage phase of the second half of the window carries real
	// observed seconds.
	require.Greater(t, engageSeconds, 0.0)
}

func TestStatsBotEndpointUnknownBot(t *testing.T) {
	server, _ := newStatsServer(t)

	recorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet, "/api/stats/unknown", nil))

	require.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestStatsBotEndpointBeforeFirstSample(t *testing.T) {
	server, bot := newStatsServer(t)
	statsKillMob(bot, 7)

	recorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet, "/api/stats/test1", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	var response botStatsResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.EqualValues(t, 1, response.Kills)
	require.Empty(t, response.History.At)
	require.Empty(t, response.Events)
}

func TestStatsWindowFilter(t *testing.T) {
	server, bot := newStatsServer(t)
	base := time.Now().Add(-11 * time.Minute)
	for i := range 10 {
		server.stats.sample(base.Add(time.Duration(i) * time.Minute))
	}
	statsKillMob(bot, 7)
	server.stats.sample(base.Add(10 * time.Minute))

	recorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet, "/api/stats/test1?window=120", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	var response botStatsResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	// Only the last two samples (2 minutes) fall inside the window.
	require.Len(t, response.History.At, 2)
	require.EqualValues(t, 1, response.History.Kills[1])
}

func TestStatsHistoryStrideBound(t *testing.T) {
	server, _ := newStatsServer(t)
	base := time.Now().Add(-time.Hour)
	for i := range statsHistoryMaxPoints * 2 {
		server.stats.sample(base.Add(time.Duration(i) * time.Minute))
	}

	recorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet, "/api/stats?window=0", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	var response statsResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.LessOrEqual(t, len(response.History.At), statsHistoryMaxPoints+1)
}

func TestStatsRingCompaction(t *testing.T) {
	ring := sampleRing{}
	base := time.Unix(1000, 0)
	for i := range statsRingCapacity + 100 {
		ring.append(botSample{at: base.Unix() + int64(i), kills: int32(i)})
	}
	// The one compaction halved the ring (1024 kept), the 100 later
	// appends refilled it to 1124.
	require.Equal(t, statsRingCapacity/2+100, ring.count)
	last, ok := ring.last()
	require.True(t, ok)
	require.EqualValues(t, statsRingCapacity+99, last.kills)
	seen := 0
	ring.walk(base.Add(time.Hour), 0, func(sample botSample) {
		if seen == 0 {
			// The compaction keeps the first of every pair: the pair
			// of the samples 0 and 1 keeps the sample 0.
			require.EqualValues(t, 0, sample.kills)
		}
		seen++
	})
	require.Equal(t, statsRingCapacity/2+100, seen)
}

func TestStatsEventDeltas(t *testing.T) {
	server, bot := newStatsServer(t)
	now := time.Now()
	server.stats.sample(now.Add(-time.Minute))
	// A level up, a relogin and a death between the samples.
	bot.ApplyUserInfo(state.UserInfo{
		Name: "Test1", Level: 11,
		MaxHP: 100, CurHP: 60, MaxMP: 50, CurMP: 40, Exp: 6000,
	})
	bot.ResetSession()
	bot.SetCharacter("Test1", 100, 18, 45000, 50000, -3500, 60, 40)
	bot.ApplyUserInfo(state.UserInfo{
		Name: "Test1", Level: 11,
		MaxHP: 100, CurHP: 60, MaxMP: 50, CurMP: 40, Exp: 6000,
	})
	bot.ApplyStatusUpdate(100, []state.Attribute{
		{ID: state.AttrCurHP, Value: 0},
	})
	server.stats.sample(now)

	series := server.stats.bots["test1"]
	require.NotNil(t, series)
	kinds := make(map[string]int)
	for _, event := range series.events {
		kinds[event.Kind]++
	}
	require.Equal(t, 1, kinds[eventDeath])
	require.Equal(t, 1, kinds[eventRelogin])
	require.Equal(t, 1, kinds[eventLevel])
}

func TestStatsRateMath(t *testing.T) {
	// The floor keeps a young process from spiking.
	require.InDelta(t, 60.0, ratePerHour(1, 1), 0.001)
	require.InDelta(t, 0.0, ratePerHour(5, 0), 0.001)
	require.InDelta(t, 10.0, perMinute(10, 60), 0.001)
	require.InDelta(t, 0.0, perMinute(10, 0), 0.001)
	require.InDelta(t, 5.0, kdOf(10, 2), 0.001)
	require.InDelta(t, 7.0, kdOf(7, 0), 0.001)
	require.EqualValues(t, 0, rejoinsOf(0))
	require.EqualValues(t, 0, rejoinsOf(1))
	require.EqualValues(t, 3, rejoinsOf(4))
}

func TestStatsFleetExcludesAcceptanceBots(t *testing.T) {
	server, bot := newStatsServer(t)
	statsKillMob(bot, 7)
	acc := state.NewBot("temp1")
	acc.SetKind(state.KindAcceptance)
	acc.SetCharacter("Temp1", 200, 18, 45000, 50000, -3500, 80, 40)
	acc.SetOnline("Temp1")
	acc.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 8, X: 45100, Y: 50100, Name: "Orc", Attackable: true,
	})
	acc.ApplyAttack(state.Attack{
		AttackerID:  200,
		TargetIDs:   [state.AttackTargets]int32{8},
		TargetCount: 1,
	})
	acc.ApplyStatusUpdate(8, []state.Attribute{
		{ID: state.AttrCurHP, Value: 0},
	})
	server.registry.Add(acc)
	server.stats.sample(time.Now())

	recorder := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet, "/api/stats", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	var response statsResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	// The acceptance bot appears in the per bot list but never in the
	// fleet aggregates.
	require.Len(t, response.Bots, 2)
	require.Equal(t, 1, response.Fleet.Registered)
	require.EqualValues(t, 1, response.Fleet.Kills)
}
