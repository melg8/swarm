// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
	"log"
	"runtime"
	"sync"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
)

// The statistics collector of the web layer: a human paced sampler
// that reads the lifetime counters of every registry tracker (and the
// process metrics of the Go runtime) into bounded rings, so the
// statistics tab answers "how did the fleet behave over the last day"
// with real history instead of only the current snapshot values.
//
// Layering: the counters live in state.Bot (the single source of
// truth); this collector only aggregates copies at its own cadence and
// never sits on a hotpath (one walk per 15 seconds per bot).
//
// Retention: every ring holds statsRingCapacity samples. When a ring
// fills, every second sample is dropped (the half-on-full compaction):
// the fresh data stays at the full 15 second resolution, the older
// data ages into coarser resolutions, and the memory stays strictly
// bounded whatever the process uptime.

// Sampling and retention constants.
const (
	// statsSamplePeriod is the sampler cadence: 15 seconds resolve a
	// kill timeline well while a 24/7 run stays cheap.
	statsSamplePeriod = 15 * time.Second
	// statsRingCapacity is the fixed sample count of every ring: the
	// compaction keeps the memory bounded, the capacity sets the fine
	// resolution window (2048 x 15 s = 8.5 hours).
	statsRingCapacity = 2048
	// statsEventsCapacity bounds the event ring of one bot: the rare
	// transitions (deaths, relogins, level changes) and the kill
	// deltas of the last sample windows.
	statsEventsCapacity = 64
	// statsHistoryMaxPoints bounds the downsampled history the
	// endpoints serve: the charts draw a few hundred points.
	statsHistoryMaxPoints = 256
)

// Phase codes of the per bot samples: the compact one byte storage of
// the hunt phase. The mapping mirrors the phase strings the hunt loop
// publishes; an unmapped phase lands in the unknown bucket.
const (
	phaseUnknown uint8 = iota
	phaseEngage
	phaseLoot
	phaseTownWalk
	phaseTownSell
	phaseTownReturn
	phaseDelevel
	phaseUser
	phaseIdle
	phaseCount
)

// phaseCodes maps the published phase strings to the sample codes.
var phaseCodes = map[string]uint8{
	"":           phaseUnknown,
	"engage":     phaseEngage,
	"loot":       phaseLoot,
	"townWalk":   phaseTownWalk,
	"townSell":   phaseTownSell,
	"townReturn": phaseTownReturn,
	"delevel":    phaseDelevel,
	"user":       phaseUser,
	"idle":       phaseIdle,
}

// phaseNames maps the sample codes back to the published strings.
var phaseNames = [phaseCount]string{
	"", "engage", "loot", "townWalk", "townSell", "townReturn",
	"delevel", "user", "idle",
}

// phaseCode resolves the sample code of a phase string.
func phaseCode(phase string) uint8 {
	if code, ok := phaseCodes[phase]; ok {
		return code
	}

	return phaseUnknown
}

// botSample is one aggregated observation of a bot at a sample time:
// the cumulative counters (monotonic unless the tracker resets) and
// the instantaneous vitals, packed for the ring.
type botSample struct {
	at       int64 // unix seconds
	exp      int64
	adena    int64
	kills    int32
	deaths   int32
	sessions int32
	level    int32
	// hpPct is the health percentage 0..100, -1 while the vitals are
	// unknown (a connecting or offline bot).
	hpPct    int16
	phase    uint8
	flags    uint8 // bit 0 online, bit 1 in combat
	tickMs   uint16
	packetPs uint16
	dmgTaken uint32
	swings   uint32
}

// Sample flags.
const (
	sampleOnline uint8 = 1 << 0
	sampleCombat uint8 = 1 << 1
)

// encodeMs packs a duration in milliseconds (scaled by ten, saturated).
func encodeMs(d time.Duration) uint16 {
	ms := d.Milliseconds()
	if ms < 0 {
		return 0
	}
	scaled := ms * 10
	if ms > 0 && scaled/10 != ms {
		return 65535
	}
	if scaled > 65535 {
		return 65535
	}

	return uint16(scaled)
}

// decodeMs unpacks a duration in milliseconds from a sample field.
func decodeMs(v uint16) float64 {
	return float64(v) / 10.0
}

// encodeRate packs a rate scaled by ten (saturated).
func encodeRate(v float64) uint16 {
	if v < 0 {
		return 0
	}
	scaled := int(v*10 + 0.5)
	if scaled > 65535 {
		return 65535
	}

	return uint16(scaled)
}

// decodeRate unpacks a rate from a sample field.
func decodeRate(v uint16) float64 {
	return float64(v) / 10.0
}

// statsEventKind enumerates the event kinds of the per bot ring.
const (
	eventKill    = "kill"
	eventDeath   = "death"
	eventRelogin = "relogin"
	eventLevel   = "level"
	eventOnline  = "online"
	eventOffline = "offline"
)

// statsEvent is one observed transition of a bot between two samples.
type statsEvent struct {
	AtMs  int64  `json:"atMs"`
	Kind  string `json:"kind"`
	Value int64  `json:"value"`
}

// sampleRing is the fixed capacity ring of one series with the
// half-on-full compaction. The layout mirrors combatFeed: a
// preallocated array walked by head and count, so the steady state
// allocates nothing.
type sampleRing struct {
	samples [statsRingCapacity]botSample
	head    int
	count   int
}

// append stores one sample and compacts the ring when it is full.
func (r *sampleRing) append(s botSample) {
	if r.count == statsRingCapacity {
		r.compact()
	}
	r.samples[r.head] = s
	r.head = (r.head + 1) % statsRingCapacity
	if r.count < statsRingCapacity {
		r.count++
	}
}

// compact drops every second sample keeping the chronological order:
// the ring count halves, the resolution of the kept half doubles.
func (r *sampleRing) compact() {
	kept := 0
	for i := range r.count {
		if i%2 != 0 {
			continue
		}
		index := (r.head - r.count + i + 2*statsRingCapacity) %
			statsRingCapacity
		r.samples[kept] = r.samples[index]
		kept++
	}
	r.head = kept % statsRingCapacity
	r.count = kept
}

// walk calls visit for every sample of the window in chronological
// order. The window cut of zero walks the whole ring.
func (r *sampleRing) walk(
	now time.Time, window time.Duration, visit func(botSample),
) {
	if r.count == 0 {
		return
	}
	cut := int64(0)
	if window > 0 {
		cut = now.Unix() - int64(window/time.Second)
	}
	start := (r.head - r.count + 2*statsRingCapacity) % statsRingCapacity
	for i := range r.count {
		index := (start + i) % statsRingCapacity
		sample := r.samples[index]
		if sample.at < cut {
			continue
		}
		visit(sample)
	}
}

// last returns the newest sample, false while the ring is empty.
func (r *sampleRing) last() (botSample, bool) {
	if r.count == 0 {
		return botSample{}, false //nolint:exhaustruct_v5 // the zero sample
	}
	index := (r.head - 1 + 2*statsRingCapacity) % statsRingCapacity

	return r.samples[index], true
}

// botSeries is the collected history of one bot id: the ring, the
// event log and the baselines the derived views compute against.
type botSeries struct {
	bot *state.Bot
	// expBase and levelBase anchor the "gained" views: the values of
	// the first sample of the series (a deleveling drop reads as a
	// real loss of the net view).
	expBase   int64
	levelBase int32
	ring      sampleRing
	events    []statsEvent
}

// newBotSeries opens the history of a tracker.
func newBotSeries(bot *state.Bot) *botSeries {
	return &botSeries{
		bot:       bot,
		expBase:   0,
		levelBase: 0,
		ring:      sampleRing{}, //nolint:exhaustruct_v5 // the zero ring
		events:    nil,
	}
}

// noteEvent appends one transition event to the bounded ring.
func (s *botSeries) noteEvent(now time.Time, kind string, value int64) {
	s.events = append(s.events, statsEvent{
		AtMs:  now.UnixMilli(),
		Kind:  kind,
		Value: value,
	})
	if len(s.events) > statsEventsCapacity {
		s.events = s.events[len(s.events)-statsEventsCapacity:]
	}
}

// fleetSample is one aggregated observation of the fleet and the
// process at a sample time.
type fleetSample struct {
	at         int64
	online     int32
	registered int32
	kills      int64
	deaths     int64
	expGained  int64
	tickAvgMs  uint16
	packetPs   uint16
	heapMB     uint16
	sysMB      uint16
	goroutines int32
	numGC      int32
}

// statsCollector owns the sampler goroutine and the collected history.
type statsCollector struct {
	registry    *state.Registry
	logger      *log.Logger
	mu          sync.Mutex
	bots        map[string]*botSeries
	fleet       []fleetSample
	started     time.Time
	lastPackets map[string]int64
	stopCh      chan struct{}
	stopOnce    sync.Once
}

// newStatsCollector creates the collector of a registry.
func newStatsCollector(
	registry *state.Registry, logger *log.Logger,
) *statsCollector {
	return &statsCollector{
		registry:    registry,
		logger:      logger,
		mu:          sync.Mutex{},
		bots:        make(map[string]*botSeries),
		fleet:       nil,
		started:     time.Time{},
		lastPackets: make(map[string]int64),
		stopCh:      make(chan struct{}),
		stopOnce:    sync.Once{},
	}
}

// stop ends the sampler goroutine (idempotent).
func (c *statsCollector) stop() {
	c.stopOnce.Do(func() { close(c.stopCh) })
}

// run samples the fleet until stop is called.
func (c *statsCollector) run() {
	c.mu.Lock()
	c.started = time.Now()
	c.mu.Unlock()
	ticker := time.NewTicker(statsSamplePeriod)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.sample(time.Now())
		}
	}
}

// sample walks every registered tracker once, appends the per bot
// samples with their transition events, then aggregates the fleet and
// the process into the fleet ring. The caller must NOT hold the
// collector lock (the tracker reads take their own locks).
func (c *statsCollector) sample(now time.Time) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	aggregates := fleetAggregates{} //nolint:exhaustruct_v5 // zero totals
	for _, bot := range c.registry.Bots() {
		c.sampleBot(bot, now, &aggregates)
	}

	tickAvg := time.Duration(0)
	if aggregates.tickCount > 0 {
		tickAvg = aggregates.tickSum / time.Duration(aggregates.tickCount)
	}
	fleet := fleetSample{
		at:         now.Unix(),
		online:     aggregates.online,
		registered: aggregates.registered,
		kills:      aggregates.kills,
		deaths:     aggregates.deaths,
		expGained:  aggregates.expGained,
		tickAvgMs:  encodeMs(tickAvg),
		packetPs:   encodeRate(aggregates.packetRate),
		heapMB:     uint16(mem.HeapAlloc >> 20),
		sysMB:      uint16(mem.Sys >> 20),
		goroutines: int32(runtime.NumGoroutine()),
		numGC:      int32(mem.NumGC),
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.fleet = append(c.fleet, fleet)
	if len(c.fleet) > statsRingCapacity {
		c.compactFleetLocked()
	}
}

// fleetAggregates accumulates the fleet wide totals of one sampling
// round (the long running bots only, the acceptance tests run their
// own worlds).
type fleetAggregates struct {
	registered int32
	online     int32
	kills      int64
	deaths     int64
	expGained  int64
	tickSum    time.Duration
	tickCount  int
	packetRate float64
}

// botSampleOf packs the tracker reads into one ring sample.
func botSampleOf(
	now time.Time, read botNow, hpPct int16, flags uint8, packetPs float64,
) botSample {
	return botSample{
		at:       now.Unix(),
		exp:      int64(read.character.Exp),
		adena:    int64(read.character.Adena),
		kills:    read.metrics.Kills,
		deaths:   read.metrics.Deaths,
		sessions: read.metrics.Sessions,
		level:    read.info.Level,
		hpPct:    hpPct,
		phase:    phaseCode(read.info.Phase),
		flags:    flags,
		tickMs:   encodeMs(read.metrics.TickEMA),
		packetPs: encodeRate(packetPs),
		dmgTaken: uint32(read.metrics.DamageTaken),
		swings:   uint32(read.metrics.SwingsLanded),
	}
}

// sampleBot reads one tracker into its series (with the transition
// events of the sample gap) and folds the long running bots into the
// fleet aggregates. The tracker reads run without the collector lock;
// the series and packet counter updates take it.
func (c *statsCollector) sampleBot(
	bot *state.Bot, now time.Time, agg *fleetAggregates,
) {
	id := bot.ID()
	read := readBotNow(bot)
	metrics := read.metrics
	info := read.info
	packets := bot.Packets()

	hpPct := int16(-1)
	if info.MaxHP > 0 {
		hpPct = int16(info.CurHP / info.MaxHP * 100.0)
		if hpPct < 0 {
			hpPct = 0
		}
		if hpPct > 100 {
			hpPct = 100
		}
	}
	flags := uint8(0)
	if info.Status == state.StatusOnline {
		flags |= sampleOnline
	}
	if info.InCombat {
		flags |= sampleCombat
	}

	c.mu.Lock()
	packetPs := c.botPacketRate(id, packets)
	sample := botSampleOf(now, read, hpPct, flags, packetPs)
	series := c.seriesFor(id, bot)
	previous, hasPrevious := series.ring.last()
	series.ring.append(sample)
	if !hasPrevious {
		series.expBase = sample.exp
		series.levelBase = sample.level
	} else {
		c.noteBotEventsLocked(series, now, sample, previous)
	}
	expGained := sample.exp - series.expBase
	c.mu.Unlock()

	if info.Kind == "" || info.Kind == state.KindLongRunning {
		agg.registered++
		if info.Status == state.StatusOnline {
			agg.online++
		}
		agg.kills += int64(metrics.Kills)
		agg.deaths += int64(metrics.Deaths)
		agg.expGained += expGained
		agg.packetRate += packetPs
		if metrics.TickCount > 0 {
			agg.tickSum += metrics.TickEMA
			agg.tickCount++
		}
	}
}

// seriesFor returns the series of a bot id, opening a fresh one when
// the tracker object changed (the acceptance manager replaces the
// trackers in place) so the counters of a new tracker never read as a
// drop of the old series. The caller must hold the collector lock.
func (c *statsCollector) seriesFor(id string, bot *state.Bot) *botSeries {
	series, ok := c.bots[id]
	if !ok || series.bot != bot {
		series = newBotSeries(bot)
		c.bots[id] = series
	}

	return series
}

// noteBotEventsLocked derives the transition events of the sample gap
// from the counter deltas: the kills fold into one kill event per
// sample, the rare transitions (deaths, relogins, level changes,
// online flips) record one event each. The caller must hold the
// collector lock.
func (c *statsCollector) noteBotEventsLocked(
	series *botSeries, now time.Time, sample, previous botSample,
) {
	if sample.kills > previous.kills {
		series.noteEvent(now, eventKill, int64(sample.kills-previous.kills))
	}
	if sample.deaths > previous.deaths {
		series.noteEvent(now, eventDeath, int64(sample.deaths-previous.deaths))
	}
	if sample.sessions > previous.sessions {
		series.noteEvent(now, eventRelogin,
			int64(sample.sessions-previous.sessions))
	}
	if sample.level != previous.level {
		series.noteEvent(now, eventLevel, int64(sample.level))
	}
	wasOnline := previous.flags&sampleOnline != 0
	isOnline := sample.flags&sampleOnline != 0
	switch {
	case isOnline && !wasOnline:
		series.noteEvent(now, eventOnline, 0)
	case !isOnline && wasOnline:
		series.noteEvent(now, eventOffline, 0)
	}
}

// botPacketRate returns the observed packet rate of a bot: the packet
// counter delta over the nominal sample period. The caller must hold
// the collector lock.
func (c *statsCollector) botPacketRate(id string, packets int64) float64 {
	last, ok := c.lastPackets[id]
	c.lastPackets[id] = packets
	if !ok {
		return 0
	}

	return float64(packets-last) / statsSamplePeriod.Seconds()
}

// compactFleetLocked halves the fleet slice the same way the per bot
// rings compact. The caller must hold the collector lock.
func (c *statsCollector) compactFleetLocked() {
	kept := c.fleet[:0]
	for i := 0; i < len(c.fleet); i += 2 {
		kept = append(kept, c.fleet[i])
	}
	c.fleet = kept
}
