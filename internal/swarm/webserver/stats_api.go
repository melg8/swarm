// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
)

// The statistics endpoints of the web layer: the fleet wide view
// (GET /api/stats) and the per bot view (GET /api/stats/{id}). Both
// answer the current counters plus the downsampled history of the
// requested window (?window=<seconds>, the default is the last day, 0
// walks everything the rings hold).
//
// Locking: the tracker reads (Info, Metrics, SelfSnapshot) happen
// before the collector lock is taken and never inside it, mirroring
// the sampler order - no goroutine ever holds a bot lock while
// waiting for the collector, so the two lock families never cycle.

// statsWindowDefault is the default history window of the endpoints.
const statsWindowDefault = 24 * time.Hour

// rateFloorSeconds bounds the rate denominators: a bot or a process
// younger than a minute reports no hourly rate instead of a spike.
const rateFloorSeconds = 60.0

// parseStatsWindow resolves the window query parameter.
func parseStatsWindow(r *http.Request) time.Duration {
	raw := r.URL.Query().Get("window")
	if raw == "" {
		return statsWindowDefault
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds < 0 {
		return statsWindowDefault
	}

	return time.Duration(seconds) * time.Second
}

// botNow is one tracker read outside the collector lock: the live
// values the "now" views render next to the collected history.
type botNow struct {
	info      state.BotInfo
	metrics   state.MetricsView
	character state.CharacterSnapshot
	startedAt time.Time
}

// readBotNow snapshots one tracker outside every collector lock.
func readBotNow(bot *state.Bot) botNow {
	return botNow{
		info:      bot.Info(),
		metrics:   bot.Metrics(),
		character: bot.SelfSnapshot(),
		startedAt: bot.StartedAt(),
	}
}

// statsResponse is the fleet view payload of GET /api/stats.
type statsResponse struct {
	NowMs           int64          `json:"nowMs"`
	SamplePeriodSec int            `json:"samplePeriodSec"`
	CollectionSec   int64          `json:"collectionSec"`
	Process         processStats   `json:"process"`
	Fleet           fleetStats     `json:"fleet"`
	Bots            []botStatsView `json:"bots"`
	History         fleetHistory   `json:"history"`
}

// processStats is the Go runtime view of the process.
type processStats struct {
	UptimeSec     int64   `json:"uptimeSec"`
	Goroutines    int     `json:"goroutines"`
	HeapMB        float64 `json:"heapMB"`
	SysMB         float64 `json:"sysMB"`
	NumGC         int     `json:"numGC"`
	LastGCPauseMs float64 `json:"lastGCPauseMs"`
}

// fleetStats is the aggregated "now" view of the long running fleet.
type fleetStats struct {
	Registered   int     `json:"registered"`
	Online       int     `json:"online"`
	Kills        int64   `json:"kills"`
	Deaths       int64   `json:"deaths"`
	Rejoins      int64   `json:"rejoins"`
	ExpGained    int64   `json:"expGained"`
	KillsPerHour float64 `json:"killsPerHour"`
	AvgTickMs    float64 `json:"avgTickMs"`
	PacketRate   float64 `json:"packetRate"`
	HitRate      float64 `json:"hitRate"`
}

// botStatsView is one bot row of the fleet view and the head of the
// per bot view.
type botStatsView struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Kind            string  `json:"kind"`
	Status          string  `json:"status"`
	Phase           string  `json:"phase"`
	Online          bool    `json:"online"`
	InCombat        bool    `json:"inCombat"`
	Level           int32   `json:"level"`
	LevelGained     int32   `json:"levelGained"`
	ExpGained       int64   `json:"expGained"`
	ExpPercent      float64 `json:"expPercent"`
	Kills           int32   `json:"kills"`
	Deaths          int32   `json:"deaths"`
	KD              float64 `json:"kd"`
	KillsPerHour    float64 `json:"killsPerHour"`
	DeathsPerHour   float64 `json:"deathsPerHour"`
	Sessions        int32   `json:"sessions"`
	Rejoins         int32   `json:"rejoins"`
	SwingsMade      int32   `json:"swingsMade"`
	SwingsLanded    int32   `json:"swingsLanded"`
	SwingsTaken     int32   `json:"swingsTaken"`
	HitRate         float64 `json:"hitRate"`
	DamageTaken     float64 `json:"damageTaken"`
	AvgTickMs       float64 `json:"avgTickMs"`
	MaxTickMs       float64 `json:"maxTickMs"`
	PacketRate      float64 `json:"packetRate"`
	HpPercent       float64 `json:"hpPercent"`
	Adena           int64   `json:"adena"`
	UptimeSec       int64   `json:"uptimeSec"`
	LastKillAgoSec  int64   `json:"lastKillAgoSec"`
	LastDeathAgoSec int64   `json:"lastDeathAgoSec"`
}

// fleetHistory is the downsampled fleet series of a window.
type fleetHistory struct {
	At           []int64   `json:"at"`
	Online       []int32   `json:"online"`
	Registered   []int32   `json:"registered"`
	Kills        []int64   `json:"kills"`
	Deaths       []int64   `json:"deaths"`
	KillsPerMin  []float64 `json:"killsPerMin"`
	DeathsPerMin []float64 `json:"deathsPerMin"`
	ExpGained    []int64   `json:"expGained"`
	AvgTickMs    []float64 `json:"avgTickMs"`
	PacketRate   []float64 `json:"packetRate"`
	HeapMB       []float64 `json:"heapMB"`
	SysMB        []float64 `json:"sysMB"`
	Goroutines   []int32   `json:"goroutines"`
	NumGC        []int32   `json:"numGC"`
}

// botStatsResponse is the per bot payload of GET /api/stats/{id}.
type botStatsResponse struct {
	botStatsView
	Phases  []phaseShare `json:"phases"`
	Events  []statsEvent `json:"events"`
	History botHistory   `json:"history"`
}

// phaseShare is one phase of the per bot phase distribution: the
// observed seconds of the window the bot spent in it.
type phaseShare struct {
	Phase   string  `json:"phase"`
	Seconds float64 `json:"seconds"`
}

// botHistory is the downsampled per bot series of a window.
type botHistory struct {
	At          []int64   `json:"at"`
	Exp         []int64   `json:"exp"`
	ExpGained   []int64   `json:"expGained"`
	Level       []int32   `json:"level"`
	HpPercent   []float64 `json:"hpPercent"`
	Adena       []int64   `json:"adena"`
	Kills       []int32   `json:"kills"`
	Deaths      []int32   `json:"deaths"`
	KillsPerMin []float64 `json:"killsPerMin"`
	AvgTickMs   []float64 `json:"avgTickMs"`
	PacketRate  []float64 `json:"packetRate"`
	Phase       []string  `json:"phase"`
}

// handleStats serves the fleet statistics view.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if s.stats == nil {
		http.Error(w, "no statistics collector", http.StatusNotFound)

		return
	}
	now := time.Now()
	response := s.stats.fleetView(now, parseStatsWindow(r))
	response.Process = s.stats.processStats(now)
	writeJSON(w, s.logger, response)
}

// handleBotStats serves the per bot statistics view.
func (s *Server) handleBotStats(w http.ResponseWriter, r *http.Request) {
	if s.stats == nil {
		http.Error(w, "no statistics collector", http.StatusNotFound)

		return
	}
	bot, ok := s.registry.Get(r.PathValue("id"))
	if !ok {
		http.Error(w, "bot not found", http.StatusNotFound)

		return
	}
	now := time.Now()
	read := readBotNow(bot)
	response, ok := s.stats.botView(read, now, parseStatsWindow(r))
	if !ok {
		// The sampler has not observed the bot yet (the first sample
		// lands within one period): answer the current counters with
		// an empty history instead of a 404.
		response = botStatsResponse{
			botStatsView: botViewFrom(read, nil, now),
			Phases:       []phaseShare{},
			Events:       []statsEvent{},
			History:      botHistory{}, //nolint:exhaustruct_v5 // empty view
		}
	}
	writeJSON(w, s.logger, response)
}

// processStats snapshots the Go runtime metrics of the process.
func (c *statsCollector) processStats(now time.Time) processStats {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	lastPause := time.Duration(0)
	if mem.NumGC > 0 {
		lastPause = time.Duration(mem.PauseNs[(mem.NumGC+
			uint32(len(mem.PauseNs))-1)%uint32(len(mem.PauseNs))])
	}
	uptime := int64(0)
	c.mu.Lock()
	if !c.started.IsZero() {
		uptime = int64(now.Sub(c.started) / time.Second)
	}
	c.mu.Unlock()

	return processStats{
		UptimeSec:     uptime,
		Goroutines:    runtime.NumGoroutine(),
		HeapMB:        float64(mem.HeapAlloc) / (1 << 20),
		SysMB:         float64(mem.Sys) / (1 << 20),
		NumGC:         int(mem.NumGC),
		LastGCPauseMs: float64(lastPause) / float64(time.Millisecond),
	}
}

// fleetView builds the fleet response of a window: the live counters
// of every registry bot (with the series baselines of the collected
// history) and the downsampled fleet ring.
func (c *statsCollector) fleetView(
	now time.Time, window time.Duration,
) statsResponse {
	bots := c.registry.Bots()
	reads := make([]botNow, 0, len(bots))
	for _, bot := range bots {
		reads = append(reads, readBotNow(bot))
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	collection := int64(0)
	if !c.started.IsZero() {
		collection = int64(now.Sub(c.started) / time.Second)
	}

	views := make([]botStatsView, 0, len(reads))
	fleet := fleetStats{} //nolint:exhaustruct_v5 // the sums start at zero
	swingsMade, swingsLanded := int64(0), int64(0)
	tickSum, tickBots := 0.0, 0
	for _, read := range reads {
		series := c.bots[read.info.ID]
		views = append(views, botViewFrom(read, series, now))
		if read.info.Kind != "" &&
			read.info.Kind != state.KindLongRunning {
			continue
		}
		fleet.Registered++
		if read.info.Status == state.StatusOnline {
			fleet.Online++
		}
		fleet.Kills += int64(read.metrics.Kills)
		fleet.Deaths += int64(read.metrics.Deaths)
		fleet.Rejoins += int64(rejoinsOf(read.metrics.Sessions))
		fleet.ExpGained += expGainedOf(read, series)
		fleet.PacketRate += lastPacketRate(series)
		if read.metrics.TickCount > 0 {
			tickSum += read.metrics.TickEMA.Seconds() * 1000
			tickBots++
		}
		swingsMade += int64(read.metrics.SwingsMade)
		swingsLanded += int64(read.metrics.SwingsLanded)
	}
	if tickBots > 0 {
		fleet.AvgTickMs = tickSum / float64(tickBots)
	}
	fleet.KillsPerHour = ratePerHour(fleet.Kills, collection)
	if swingsMade > 0 {
		fleet.HitRate = float64(swingsLanded) / float64(swingsMade)
	}

	return statsResponse{ //nolint:exhaustruct_v5 // process filled by the caller
		NowMs:           now.UnixMilli(),
		SamplePeriodSec: int(statsSamplePeriod / time.Second),
		CollectionSec:   collection,
		Fleet:           fleet,
		Bots:            views,
		History:         c.fleetHistoryLocked(now, window),
	}
}

// botView builds the per bot response of a window. The second return
// is false while the sampler has not observed the bot yet.
func (c *statsCollector) botView(
	read botNow, now time.Time, window time.Duration,
) (botStatsResponse, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	series, ok := c.bots[read.info.ID]
	if !ok || series.ring.count == 0 {
		return botStatsResponse{}, false //nolint:exhaustruct_v5 // the empty miss
	}

	view := botViewFrom(read, series, now)
	response := botStatsResponse{
		botStatsView: view,
		Phases:       c.phaseSharesLocked(series, now, window),
		Events:       append([]statsEvent{}, series.events...),
		History:      c.botHistoryLocked(series, now, window),
	}

	return response, true
}

// botViewFrom builds the live counter view of one bot: the tracker
// reads of the caller plus the baselines and the last sample of the
// series (nil while the collector has none yet).
func botViewFrom(read botNow, series *botSeries, now time.Time) botStatsView {
	uptimeSec := int64(0)
	if !read.startedAt.IsZero() {
		uptimeSec = int64(now.Sub(read.startedAt) / time.Second)
	}
	view := botStatsView{
		ID:              read.info.ID,
		Name:            read.info.Name,
		Kind:            read.info.Kind,
		Status:          string(read.info.Status),
		Phase:           read.info.Phase,
		Online:          read.info.Status == state.StatusOnline,
		InCombat:        read.info.InCombat,
		Level:           read.info.Level,
		LevelGained:     0,
		ExpGained:       0,
		ExpPercent:      read.info.ExpPercent,
		Kills:           read.metrics.Kills,
		Deaths:          read.metrics.Deaths,
		KD:              kdOf(read.metrics.Kills, read.metrics.Deaths),
		KillsPerHour:    ratePerHour(int64(read.metrics.Kills), uptimeSec),
		DeathsPerHour:   ratePerHour(int64(read.metrics.Deaths), uptimeSec),
		Sessions:        read.metrics.Sessions,
		Rejoins:         rejoinsOf(read.metrics.Sessions),
		SwingsMade:      read.metrics.SwingsMade,
		SwingsLanded:    read.metrics.SwingsLanded,
		SwingsTaken:     read.metrics.SwingsTaken,
		HitRate:         0,
		DamageTaken:     read.metrics.DamageTaken,
		AvgTickMs:       read.metrics.TickEMA.Seconds() * 1000,
		MaxTickMs:       read.metrics.TickMax.Seconds() * 1000,
		PacketRate:      lastPacketRate(series),
		HpPercent:       hpPercentOf(read.info),
		Adena:           int64(read.character.Adena),
		UptimeSec:       uptimeSec,
		LastKillAgoSec:  agoSec(read.metrics.LastKillAt, now),
		LastDeathAgoSec: agoSec(read.metrics.LastDeathAt, now),
	}
	if read.metrics.SwingsMade > 0 {
		view.HitRate = float64(read.metrics.SwingsLanded) /
			float64(read.metrics.SwingsMade)
	}
	if series != nil && series.ring.count > 0 {
		view.ExpGained = expGainedOf(read, series)
		view.LevelGained = read.info.Level - series.levelBase
	}

	return view
}

// fleetHistoryLocked downsamples the fleet ring of a window. The
// caller must hold the collector lock.
func (c *statsCollector) fleetHistoryLocked(
	now time.Time, window time.Duration,
) fleetHistory {
	windowed := c.windowedFleetLocked(now, window)
	history := fleetHistory{} //nolint:exhaustruct_v5 // the series start empty
	stride := historyStride(len(windowed))
	prevAt := int64(0)
	prevKills, prevDeaths := int64(0), int64(0)
	for i := range windowed {
		if i%stride != 0 && i != len(windowed)-1 {
			continue
		}
		sample := windowed[i]
		history.At = append(history.At, sample.at)
		history.Online = append(history.Online, sample.online)
		history.Registered = append(history.Registered, sample.registered)
		history.Kills = append(history.Kills, sample.kills)
		history.Deaths = append(history.Deaths, sample.deaths)
		history.ExpGained = append(history.ExpGained, sample.expGained)
		history.AvgTickMs = append(history.AvgTickMs,
			decodeMs(sample.tickAvgMs))
		history.PacketRate = append(history.PacketRate,
			decodeRate(sample.packetPs))
		history.HeapMB = append(history.HeapMB, float64(sample.heapMB))
		history.SysMB = append(history.SysMB, float64(sample.sysMB))
		history.Goroutines = append(history.Goroutines, sample.goroutines)
		history.NumGC = append(history.NumGC, sample.numGC)
		if len(history.At) == 1 {
			history.KillsPerMin = append(history.KillsPerMin, 0)
			history.DeathsPerMin = append(history.DeathsPerMin, 0)
		} else {
			dt := rateFloor(prevAt, sample.at)
			history.KillsPerMin = append(history.KillsPerMin,
				perMinute(sample.kills-prevKills, dt))
			history.DeathsPerMin = append(history.DeathsPerMin,
				perMinute(sample.deaths-prevDeaths, dt))
		}
		prevAt, prevKills, prevDeaths = sample.at, sample.kills, sample.deaths
	}

	return history
}

// windowedFleetLocked collects the fleet samples of the window. The
// caller must hold the collector lock.
func (c *statsCollector) windowedFleetLocked(
	now time.Time, window time.Duration,
) []fleetSample {
	cut := int64(0)
	if window > 0 {
		cut = now.Unix() - int64(window/time.Second)
	}
	windowed := make([]fleetSample, 0, len(c.fleet))
	for _, sample := range c.fleet {
		if sample.at >= cut {
			windowed = append(windowed, sample)
		}
	}

	return windowed
}

// botHistoryLocked downsamples the per bot ring of a window. The
// caller must hold the collector lock.
func (c *statsCollector) botHistoryLocked(
	series *botSeries, now time.Time, window time.Duration,
) botHistory {
	windowed := make([]botSample, 0, series.ring.count)
	series.ring.walk(now, window, func(sample botSample) {
		windowed = append(windowed, sample)
	})
	history := botHistory{} //nolint:exhaustruct_v5 // the series start empty
	stride := historyStride(len(windowed))
	prevAt := int64(0)
	prevKills := int64(0)
	for i := range windowed {
		if i%stride != 0 && i != len(windowed)-1 {
			continue
		}
		sample := windowed[i]
		history.At = append(history.At, sample.at)
		history.Exp = append(history.Exp, sample.exp)
		history.Level = append(history.Level, sample.level)
		history.HpPercent = append(history.HpPercent, hpOfSample(sample))
		history.Adena = append(history.Adena, sample.adena)
		history.Kills = append(history.Kills, sample.kills)
		history.Deaths = append(history.Deaths, sample.deaths)
		history.AvgTickMs = append(history.AvgTickMs, decodeMs(sample.tickMs))
		history.PacketRate = append(history.PacketRate,
			decodeRate(sample.packetPs))
		if sample.phase < phaseCount {
			history.Phase = append(history.Phase, phaseNames[sample.phase])
		} else {
			history.Phase = append(history.Phase, "")
		}
		history.ExpGained = append(history.ExpGained,
			sample.exp-series.expBase)
		if len(history.At) == 1 {
			history.KillsPerMin = append(history.KillsPerMin, 0)
		} else {
			dt := rateFloor(prevAt, sample.at)
			history.KillsPerMin = append(history.KillsPerMin, perMinute(
				int64(sample.kills)-prevKills, dt))
		}
		prevAt = sample.at
		prevKills = int64(sample.kills)
	}

	return history
}

// phaseSharesLocked measures how long the bot spent in every hunt
// phase over the window (the share of each sample gap attributes to
// the phase of the earlier sample). The caller must hold the
// collector lock.
func (c *statsCollector) phaseSharesLocked(
	series *botSeries, now time.Time, window time.Duration,
) []phaseShare {
	shares := make([]phaseShare, phaseCount)
	for i := range shares {
		shares[i] = phaseShare{Phase: phaseNames[i], Seconds: 0}
	}
	previousAt := int64(0)
	previousPhase := uint8(255)
	series.ring.walk(now, window, func(sample botSample) {
		if previousAt > 0 && previousPhase < phaseCount {
			gap := float64(sample.at - previousAt)
			shares[previousPhase].Seconds += gap
		}
		previousAt = sample.at
		previousPhase = sample.phase
	})
	if previousAt > 0 && previousPhase < phaseCount {
		last := now.Unix()
		if window > 0 {
			bounded := previousAt + int64(window/time.Second)
			if last > bounded {
				last = bounded
			}
		}
		shares[previousPhase].Seconds += float64(last - previousAt)
	}

	visible := make([]phaseShare, 0, phaseCount)
	for _, share := range shares {
		if share.Seconds > 0 {
			visible = append(visible, share)
		}
	}

	return visible
}

// historyStride returns the stride that keeps the point count of a
// windowed series under the served bound.
func historyStride(count int) int {
	if count <= 1 {
		return 1
	}
	stride := (count + statsHistoryMaxPoints - 1) / statsHistoryMaxPoints
	if stride < 1 {
		return 1
	}

	return stride
}

// rateFloor returns the rate denominator between two picked samples
// (the floor keeps a tight pair from spiking the per minute view).
func rateFloor(prevAt, at int64) float64 {
	dt := rateFloorSeconds
	if elapsed := float64(at - prevAt); elapsed > dt {
		dt = elapsed
	}

	return dt
}

// rejoinsOf derives the rejoin count of the session count: the first
// session of a tracker opens the deployment, every later one is a
// rejoin.
func rejoinsOf(sessions int32) int32 {
	if sessions <= 1 {
		return 0
	}

	return sessions - 1
}

// kdOf computes the kill to death ratio: a deathless bot reports its
// kill count.
func kdOf(kills, deaths int32) float64 {
	if deaths <= 0 {
		return float64(kills)
	}

	return float64(kills) / float64(deaths)
}

// ratePerHour converts a counter into an hourly rate over the given
// observation seconds (floored).
func ratePerHour(value int64, seconds int64) float64 {
	if seconds < 1 {
		return 0
	}
	denominator := float64(seconds)
	if denominator < rateFloorSeconds {
		denominator = rateFloorSeconds
	}

	return float64(value) * 3600.0 / denominator
}

// perMinute converts a delta into a per minute rate over the floored
// seconds.
func perMinute(delta int64, seconds float64) float64 {
	if seconds <= 0 {
		return 0
	}

	return float64(delta) * 60.0 / seconds
}

// expGainedOf computes the net experience gain of a bot against its
// series baseline (a deleveling drop reads as a real loss).
func expGainedOf(read botNow, series *botSeries) int64 {
	if series == nil || series.ring.count == 0 {
		return 0
	}

	return int64(read.character.Exp) - series.expBase
}

// lastPacketRate reads the observed packet rate of the last sample.
func lastPacketRate(series *botSeries) float64 {
	if series == nil {
		return 0
	}
	if sample, ok := series.ring.last(); ok {
		return decodeRate(sample.packetPs)
	}

	return 0
}

// hpPercentOf computes the health percentage of the live view, -1
// while the vitals are unknown.
func hpPercentOf(info state.BotInfo) float64 {
	if info.MaxHP <= 0 {
		return -1
	}
	percent := info.CurHP / info.MaxHP * 100.0
	if percent < 0 {
		return 0
	}
	if percent > 100 {
		return 100
	}

	return percent
}

// hpOfSample reads the health percentage of a sample (-1 unknown).
func hpOfSample(sample botSample) float64 {
	return float64(sample.hpPct)
}

// agoSec returns the age of a timestamp in whole seconds (0 when it
// never happened).
func agoSec(at time.Time, now time.Time) int64 {
	if at.IsZero() {
		return 0
	}

	return int64(now.Sub(at) / time.Second)
}
