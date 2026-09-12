// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package session

import (
	"sort"
	"time"
)

// Aggregation caps. The lists keep their newest entries and fold the
// overflow into a counter, so a pathological run still renders a
// bounded report (the raw journal keeps everything).
const (
	maxDeaths     = 32
	maxBuys       = 48
	maxStalls     = 32
	maxFreezes    = 16
	maxGaps       = 16
	maxLevelMarks = 96
	maxSlowFight  = 10
	maxMobs       = 64
	storyRingLen  = 128
)

// Phase groups of the downtime attribution. The hunt phases farm, the
// town phases walk and trade, the rest is idle, manual or deleveling.
func phaseGroup(ph string) string {
	switch ph {
	case "engage", "loot":
		return "hunt"
	case "townWalk", "townSell", "townReturn":
		return "town"
	case "delevel":
		return "delevel"
	default:
		return "other"
	}
}

// botAgg is the in-memory aggregate of one bot the report renders: the
// session timeline, the per-hour buckets of the sampled state, the
// kill and death statistics, the money trail of the purchases and the
// tail of the story. One struct, no maps beyond the mobs and the
// buckets, so a 24 hour run (roughly 3 000 samples, 5 000 kills) costs
// a few kilobytes.
type botAgg struct {
	id    string
	first int64
	last  int64

	// The story cap state (a rolling per-minute budget).
	storyMinute   int64
	storyMinuteAt int64
	storyMuted    int
	ring          []storyLine

	// The sampled state trail.
	prev      *sampleState
	hours     map[int32]*hourBucket
	levels    []levelMark
	freezes   []freezeMark
	gaps      []gapMark
	freezeRun int
	freezeAt  int64
	freezeXY  [2]int32

	// The hunt statistics.
	kills     int
	durHist   [62]int32
	slowFight []fightMark
	mobs      map[string]*mobStat
	mobOrder  []string
	deaths    []deathMark

	// The town trip and money trail.
	trips      int
	tripSec    float64
	tripMax    tripMark
	tripOpen   *tripMark
	tripReason map[string]int
	buys       []buyMark
	sellBatch  int
	sellItems  int

	// The stall and walk evidence.
	xpStalls   []stallMark
	posStalls  []stallMark
	repaths    int
	connects   int
	lost       int
	lastSample int64
}

// storyLine is one entry of the recent story ring.
type storyLine struct {
	t   int64
	msg string
}

// sampleState is the previous sample the deltas compute against.
type sampleState struct {
	t    int64
	lv   int32
	xp   int64
	ad   int64
	x    int32
	y    int32
	ph   string
	seen bool
}

// hourBucket aggregates one clock hour of samples and events.
type hourBucket struct {
	kills   int
	deaths  int
	huntSec float64
	townSec float64
	otherS  float64
	xpFirst int64
	xpLast  int64
	adFirst int64
	adLast  int64
	lvFirst int32
	lvLast  int32
	at      int64
}

// levelMark is one level up.
type levelMark struct {
	t  int64
	lv int32
}

// deathMark is one character death.
type deathMark struct {
	t  int64
	lv int32
	x  int32
	y  int32
}

// fightMark is one of the slowest fights.
type fightMark struct {
	t   int64
	mob string
	lvl int32
	dur float64
}

// mobStat accumulates the fights against one mob species.
type mobStat struct {
	name    string
	count   int
	durSum  float64
	durMax  float64
	hpBelow float64
	hpHits  int
}

// tripMark is one town trip (open until the end reason arrives).
type tripMark struct {
	t      int64
	reason string
	dur    float64
}

// buyMark is one purchase batch.
type buyMark struct {
	t     int64
	items string
	count int
	cost  int64
}

// stallMark is one stagnation event.
type stallMark struct {
	t   int64
	sec float64
	x   int32
	y   int32
}

// freezeMark is a position hold detected from the samples.
type freezeMark struct {
	t   int64
	sec float64
	x   int32
	y   int32
}

// gapMark is a sample gap interpreted as an offline stretch.
type gapMark struct {
	t   int64
	sec float64
}

// newBotAgg builds the empty aggregate of one bot.
func newBotAgg(id string) *botAgg {
	return &botAgg{
		id:            id,
		first:         0,
		last:          0,
		storyMinute:   0,
		storyMinuteAt: 0,
		storyMuted:    0,
		ring:          make([]storyLine, 0, storyRingLen),
		prev:          nil,
		hours:         make(map[int32]*hourBucket),
		levels:        nil,
		freezes:       nil,
		gaps:          nil,
		freezeRun:     0,
		freezeAt:      0,
		freezeXY:      [2]int32{0, 0},
		kills:         0,
		durHist:       [62]int32{},
		slowFight:     nil,
		mobs:          make(map[string]*mobStat),
		mobOrder:      nil,
		deaths:        nil,
		trips:         0,
		tripSec:       0,
		tripMax:       tripMark{t: 0, reason: "", dur: 0},
		tripOpen:      nil,
		tripReason:    make(map[string]int),
		buys:          nil,
		sellBatch:     0,
		sellItems:     0,
		xpStalls:      nil,
		posStalls:     nil,
		repaths:       0,
		connects:      0,
		lost:          0,
		lastSample:    0,
	}
}

// accept reports whether the record rides the journal. The story
// records pass a per-minute budget: a flooded event log (a misbehaving
// loop) mutes instead of filling the disk, and the muted count names
// the flood in the report.
func (a *botAgg) accept(r record) bool {
	if r.E != kindStory {
		return true
	}
	minute := r.T / 60
	if minute != a.storyMinuteAt {
		a.storyMinuteAt = minute
		a.storyMinute = 0
	}
	if a.storyMinute >= storyCapPerMin {
		a.storyMuted++

		return false
	}
	a.storyMinute++

	return true
}

// apply folds one record into the aggregate. The caller must hold the
// journal write lock (the report readers walk the same state under
// the read lock).
func (a *botAgg) apply(r record) {
	if a.first == 0 {
		a.first = r.T
	}
	if r.T > a.last {
		a.last = r.T
	}
	switch r.E {
	case kindStory:
		a.pushStory(r)
	case kindSample:
		a.applySample(r)
	case kindKill:
		a.applyKill(r)
	case kindDeath:
		a.pushDeath(r)
	case kindLevel:
		a.pushLevel(r)
	case kindTripStart:
		a.openTrip(r)
	case kindTripEnd:
		a.closeTrip(r)
	case kindBuy:
		a.pushBuy(r)
	default:
		a.applyEvent(r)
	}
}

// applyEvent folds the scalar events: the sells, the zone switches,
// the stalls, the re-paths and the lifecycle marks.
func (a *botAgg) applyEvent(r record) {
	switch r.E {
	case kindSell:
		a.sellBatch++
		a.sellItems += int(r.N)
	case kindZone:
		a.pushStory(zoneStory(r))
	case kindStall:
		a.pushStall(r)
	case kindRepath:
		a.repaths++
	case kindConnect:
		// Only the world entries count: the reconnect waits of the
		// supervisor are lifecycle marks, not connections.
		if r.R == "entered" {
			a.connects++
		}
	case kindLost:
		a.lost++
	}
}

// pushStory appends to the ring (a fixed capacity circular buffer, the
// newest line last).
func (a *botAgg) pushStory(r record) {
	if len(a.ring) < storyRingLen {
		a.ring = append(a.ring, storyLine{t: r.T, msg: r.M})

		return
	}
	copy(a.ring, a.ring[1:])
	a.ring[storyRingLen-1] = storyLine{t: r.T, msg: r.M}
}

// zoneStory renders the zone switch as a story line (the zone events
// of the journal also ride the report tail).
func zoneStory(r record) record {
	story := newRecord(r.B, kindStory, time.Unix(r.T, 0))
	story.M = "Hunt: zone switch to " + r.Mob + " (" + r.R + ")"

	return story
}

// applySample folds one periodic sample: the hourly bucket, the level
// marks, the offline gaps and the position freeze detection.
func (a *botAgg) applySample(r record) {
	now := r.T
	if a.lastSample != 0 {
		a.gapCheck(now)
	}
	a.lastSample = now
	state := &sampleState{
		t: now, lv: r.Lv, xp: r.Xp, ad: r.Ad,
		x: r.X, y: r.Y, ph: r.Ph, seen: true,
	}
	if a.prev != nil && a.prev.seen {
		a.bucketDelta(a.prev, state)
	}
	if a.prev == nil {
		a.bucketFirst(state)
	}
	if a.prev != nil && r.Lv > a.prev.lv {
		a.pushLevel(r)
	}
	a.freezeCheck(r)
	a.prev = state
}

// bucketOf returns the hour bucket of a unix second, creating it.
func (a *botAgg) bucketOf(t int64) *hourBucket {
	key := int32(t / 3600)
	bucket, ok := a.hours[key]
	if !ok {
		bucket = &hourBucket{ //nolint:exhaustruct_v5 // zero counters
			at: t, lvFirst: 0, lvLast: 0,
		}
		a.hours[key] = bucket
	}

	return bucket
}

// bucketFirst seeds the first bucket of the trail.
func (a *botAgg) bucketFirst(s *sampleState) {
	bucket := a.bucketOf(s.t)
	bucket.xpFirst = s.xp
	bucket.adFirst = s.ad
	bucket.lvFirst = s.lv
	bucket.lvLast = s.lv
	bucket.xpLast = s.xp
	bucket.adLast = s.ad
}

// bucketDelta folds the transition between two samples into the hour
// buckets of both (the phase seconds attribute to the bucket the
// interval starts in).
func (a *botAgg) bucketDelta(from *sampleState, to *sampleState) {
	dt := float64(to.t - from.t)
	if dt <= 0 {
		return
	}
	bucket := a.bucketOf(from.t)
	switch phaseGroup(from.ph) {
	case "hunt":
		bucket.huntSec += dt
	case "town":
		bucket.townSec += dt
	default:
		bucket.otherS += dt
	}
	bucket.xpLast = to.xp
	bucket.adLast = to.ad
	bucket.lvLast = to.lv
	if next := a.bucketOf(to.t); next != bucket {
		next.xpFirst = to.xp
		next.adFirst = to.ad
		next.lvFirst = to.lv
		next.lvLast = to.lv
		next.xpLast = to.xp
		next.adLast = to.ad
	}
}

// gapCheck records an offline gap when the samples went silent past
// the gap threshold (the sampler pauses while the supervisor waits out
// a reconnect backoff).
func (a *botAgg) gapCheck(now int64) {
	gap := now - a.lastSample
	if gap < gapThresholdSec {
		return
	}
	a.gaps = appendCapped(a.gaps, gapMark{t: a.lastSample, sec: float64(gap)},
		maxGaps)
}

// freezeCheck tracks consecutive samples on the same cell: six in a
// row (three minutes at the sampler period) while the phase farms is
// a frozen walk the stagnation watch may have missed.
func (a *botAgg) freezeCheck(r record) {
	if r.X == a.freezeXY[0] && r.Y == a.freezeXY[1] {
		a.freezeRun++
		if a.freezeRun == freezeRunThreshold {
			a.freezes = appendCapped(a.freezes,
				freezeMark{
					t:   r.T,
					sec: float64(a.freezeRun * samplePeriodSec),
					x:   r.X, y: r.Y,
				}, maxFreezes)
		}

		return
	}
	a.freezeXY = [2]int32{r.X, r.Y}
	a.freezeRun = 1
	a.freezeAt = r.T
}

// applyKill folds one kill: the counters, the duration histogram (one
// second buckets up to sixty) and the per-mob statistics.
func (a *botAgg) applyKill(r record) {
	a.kills++
	bucket := a.bucketOf(r.T)
	bucket.kills++
	bin := int(r.Dur)
	if bin < 0 {
		bin = 0
	}
	if bin >= len(a.durHist) {
		bin = len(a.durHist) - 1
	}
	a.durHist[bin]++
	full := len(a.slowFight) >= maxSlowFight
	if !full || r.Dur > a.slowFight[len(a.slowFight)-1].dur {
		a.slowFight = append(a.slowFight, fightMark{
			t: r.T, mob: r.Mob, lvl: r.Lvl, dur: r.Dur,
		})
		sort.Slice(a.slowFight, func(i, j int) bool {
			return a.slowFight[i].dur > a.slowFight[j].dur
		})
		if len(a.slowFight) > maxSlowFight {
			a.slowFight = a.slowFight[:maxSlowFight]
		}
	}
	mob := a.mobs[r.Mob]
	if mob == nil {
		mob = &mobStat{ //nolint:exhaustruct_v5 // zero accumulators
			name: r.Mob,
		}
		a.mobs[r.Mob] = mob
		if len(a.mobs) <= maxMobs {
			a.mobOrder = append(a.mobOrder, r.Mob)
		}
	}
	mob.count++
	mob.durSum += r.Dur
	if r.Dur > mob.durMax {
		mob.durMax = r.Dur
	}
	if r.Hp > 0 {
		mob.hpBelow += r.Hp
		mob.hpHits++
	}
}

// pushDeath records one death into the capped list and the bucket.
func (a *botAgg) pushDeath(r record) {
	a.deaths = appendCapped(a.deaths,
		deathMark{t: r.T, lv: r.Lv, x: r.X, y: r.Y}, maxDeaths)
	a.bucketOf(r.T).deaths++
}

// pushLevel records one level up (the samples and the sampler agree;
// the duplicate marks deduplicate on the level value).
func (a *botAgg) pushLevel(r record) {
	if n := len(a.levels); n > 0 && a.levels[n-1].lv >= r.Lv {
		return
	}
	a.levels = appendCapped(a.levels, levelMark{t: r.T, lv: r.Lv}, maxLevelMarks)
}

// openTrip remembers the running trip bracket.
func (a *botAgg) openTrip(r record) {
	a.tripOpen = &tripMark{t: r.T, reason: r.R, dur: 0}
}

// closeTrip folds the finished trip into the counters.
func (a *botAgg) closeTrip(r record) {
	if a.tripOpen == nil {
		return
	}
	dur := r.Dur
	if dur <= 0 {
		dur = float64(r.T - a.tripOpen.t)
	}
	a.trips++
	a.tripSec += dur
	a.tripReason[r.R]++
	if dur > a.tripMax.dur {
		a.tripMax = tripMark{t: a.tripOpen.t, reason: a.tripOpen.reason, dur: dur}
	}
	a.tripOpen = nil
}

// pushBuy records one purchase batch.
func (a *botAgg) pushBuy(r record) {
	a.buys = appendCapped(a.buys, buyMark{
		t: r.T, items: r.Items, count: int(r.N), cost: r.Cost,
	}, maxBuys)
}

// pushStall records one stagnation event into the matching list.
func (a *botAgg) pushStall(r record) {
	mark := stallMark{t: r.T, sec: r.Dur, x: r.X, y: r.Y}
	if r.R == "xp" {
		a.xpStalls = appendCapped(a.xpStalls, mark, maxStalls)

		return
	}
	a.posStalls = appendCapped(a.posStalls, mark, maxStalls)
}

// appendCapped appends to a slice keeping the newest limit entries.
func appendCapped[T any](dst []T, entry T, limit int) []T {
	if len(dst) < limit {
		return append(dst, entry)
	}
	copy(dst, dst[1:])
	dst[limit-1] = entry

	return dst
}

// The sampler constants the aggregate reasons with (the freeze
// seconds and the gap threshold mirror the sampler period).
const (
	samplePeriodSec    = 30
	freezeRunThreshold = 6
	gapThresholdSec    = 300
)
