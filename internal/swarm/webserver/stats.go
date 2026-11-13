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

// The statistics collector: a human paced sampler that reads the lifetime
// counters of every registry tracker and the Go runtime metrics into bounded
// rings, so the statistics tab answers how the fleet behaved over the last day
// with real history. The counters stay in state.Bot; the collector aggregates
// copies at its own cadence and never sits on a hotpath (one walk per 15
// seconds per bot). Every ring holds statsRingCapacity samples and drops every
// second one when full, so the fresh data keeps the 15 second resolution, the
// older data ages coarser, and the memory stays bounded whatever the uptime.

// Sampling and retention constants.
const (
    // statsSamplePeriod is the sampler cadence: 15 seconds resolve a kill
    // timeline.
    statsSamplePeriod = 15 * time.Second
    // statsRingCapacity sets the fine resolution window (2048 x 15 s = 8.5
    // hours).
    statsRingCapacity = 2048
    // statsEventsCapacity bounds the rare transitions and the kill deltas of a
    // bot.
    statsEventsCapacity = 64
    // statsHistoryMaxPoints bounds the downsampled history the endpoints serve.
    statsHistoryMaxPoints = 256
)

// Phase codes of the per bot samples: the mapping mirrors the published phase
// strings, an unmapped phase lands in the unknown bucket.
const (
    phaseUnknown uint8 = iota
    phaseEngage
    phaseLoot
    phaseTownWalk
    phaseTownSell
    phaseTownReturn
    phaseCityShop
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
    "cityShop":   phaseCityShop,
    "delevel":    phaseDelevel,
    "user":       phaseUser,
    "idle":       phaseIdle,
}

// phaseNames maps the sample codes back to the published strings.
var phaseNames = [phaseCount]string{
    "", "engage", "loot", "townWalk", "townSell", "townReturn",
    "cityShop", "delevel", "user", "idle",
}

func phaseCode(phase string) uint8 {
    if code, ok := phaseCodes[phase]; ok {
        return code
    }

    return phaseUnknown
}

// botSample is one observation of a bot: the cumulative counters (monotonic
// unless the tracker resets) and the instantaneous vitals, packed for the ring.
type botSample struct {
    at       int64 // unix seconds
    exp      int64
    adena    int64
    kills    int32
    deaths   int32
    sessions int32
    level    int32
    // hpPct is the health percentage 0..100, -1 while the vitals are unknown.
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

// sampleRing is the fixed capacity ring of one series with the half-on-full
// compaction: a preallocated array walked by head and count, so the steady
// state allocates nothing.
type sampleRing struct {
    samples [statsRingCapacity]botSample
    head    int
    count   int
}

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

// compact drops every second sample in chronological order, doubling the
// resolution.
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

// walk calls visit for every sample of the window in chronological order (zero
// walks the whole ring).
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

// botSeries is the history of one bot id: the ring, the event log and the
// baselines.
type botSeries struct {
    bot *state.Bot
    // based marks that the baselines hold real character values: the anchor
    // waits for the first sample with level > 0, never the zeroed character of
    // a reconnect gap.
    based bool
    // expBase, adenaBase and levelBase anchor the "gained" views at the first
    // valid sample, so a deleveling drop reads as a real loss of the net view.
    expBase   int64
    adenaBase int64
    levelBase int32
    ring      sampleRing
    events    []statsEvent
}

func newBotSeries(bot *state.Bot) *botSeries {
    return &botSeries{
        bot:       bot,
        based:     false,
        expBase:   0,
        adenaBase: 0,
        levelBase: 0,
        ring:      sampleRing{}, //nolint:exhaustruct_v5 // the zero ring
        events:    nil,
    }
}

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
    adena      int64
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

// sample walks every registered tracker once, appends the per bot samples with
// their transition events, then aggregates the fleet and the process into the
// fleet ring. The caller must not hold the collector lock.
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
        adena:      aggregates.adena,
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

// fleetAggregates accumulates the fleet wide totals of one round (the long
// running bots only, the acceptance tests run their own worlds).
type fleetAggregates struct {
    registered int32
    online     int32
    kills      int64
    deaths     int64
    expGained  int64
    adena      int64
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

// carryGapSample fills a sample inside the reconnect gap: ResetSession zeroed
// the tracker character and the new UserInfo has not arrived, so the character
// facts read as zeros. The character does not change while the bot is away (the
// relogin answer returns exp, adena and level exactly as they were, a death
// penalty lands before the logout), so the sample carries the last known values
// instead of a zero flash that crashes the charts; the connection facts (online
// flag, packet rate, phase) stay untouched.
func carryGapSample(sample *botSample, previous botSample) {
    sample.exp = previous.exp
    sample.adena = previous.adena
    sample.level = previous.level
    sample.hpPct = previous.hpPct
}

// sampleBot reads one tracker into its series with the transition events of the
// gap and folds the long running bots into the fleet aggregates. The tracker
// reads run without the collector lock, the series and packet counter updates
// take it.
func (c *statsCollector) sampleBot(
    bot *state.Bot, now time.Time, agg *fleetAggregates,
) {
    id := bot.ID()
    read := readBotNow(bot)
    metrics := read.metrics
    info := read.info
    packets := bot.Packets()

    hpPct := int16(hpPercentOf(info))
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
    c.appendSampleLocked(series, now, sample)
    expGained := int64(0)
    if series.based {
        expGained = sample.exp - series.expBase
    }
    c.mu.Unlock()

    if info.Kind == "" || info.Kind == state.KindLongRunning {
        agg.registered++
        if info.Status == state.StatusOnline {
            agg.online++
        }
        agg.kills += int64(metrics.Kills)
        agg.deaths += int64(metrics.Deaths)
        agg.expGained += expGained
        agg.adena += sample.adena
        agg.packetRate += packetPs
        if metrics.TickCount > 0 {
            agg.tickSum += metrics.TickEMA
            agg.tickCount++
        }
    }
}

// appendSampleLocked folds one sample into its series: a gap sample carries the
// last known character, the first valid sample anchors the baselines, and the
// transition events note against the previous sample. The caller must hold the
// collector lock.
func (c *statsCollector) appendSampleLocked(
    series *botSeries, now time.Time, sample botSample,
) {
    previous, hasPrevious := series.ring.last()
    if hasPrevious && sample.level <= 0 {
        carryGapSample(&sample, previous)
    }
    if !series.based && sample.level > 0 {
        series.expBase = sample.exp
        series.adenaBase = sample.adena
        series.levelBase = sample.level
        series.based = true
    }
    series.ring.append(sample)
    if hasPrevious {
        c.noteBotEventsLocked(series, now, sample, previous)
    }
}

// seriesFor returns the series of a bot id, opening a fresh one when the
// tracker object changed (the acceptance manager replaces trackers in place),
// so the counters of a new tracker never read as a drop of the old series. The
// caller must hold the lock.
func (c *statsCollector) seriesFor(id string, bot *state.Bot) *botSeries {
    series, ok := c.bots[id]
    if !ok || series.bot != bot {
        series = newBotSeries(bot)
        c.bots[id] = series
    }

    return series
}

// noteBotEventsLocked derives the transition events from the counter deltas:
// the kills fold into one event per sample, the rare transitions (deaths,
// relogins, level changes, online flips) record one each. The caller must hold
// the collector lock.
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

func (c *statsCollector) botPacketRate(id string, packets int64) float64 {
    last, ok := c.lastPackets[id]
    c.lastPackets[id] = packets
    if !ok {
        return 0
    }

    return float64(packets-last) / statsSamplePeriod.Seconds()
}

func (c *statsCollector) compactFleetLocked() {
    kept := c.fleet[:0]
    for i := 0; i < len(c.fleet); i += 2 {
        kept = append(kept, c.fleet[i])
    }
    c.fleet = kept
}
