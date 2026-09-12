// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package session

import (
	"context"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
)

// samplePeriod is the sampler tick: thirty seconds of resolution is
// one thousand lines a day (roughly 100 KB), enough for the hourly
// tables, the level marks and the freeze detection without any
// per-packet cost.
const samplePeriod = 30 * time.Second

// Sampler is the periodic character state publisher of one bot: it
// reads the public tracker accessors and feeds the journal samples.
// The hunt loop and the supervisor own the events; the sampler owns
// the quantitative trail (the xp and adena curves, the phase share).
type Sampler struct {
	bot       string
	tracker   *state.Bot
	journal   *Journal
	period    time.Duration
	lastLevel int32
}

// NewSampler builds the sampler of one bot.
func NewSampler(
	bot string, tracker *state.Bot, journal *Journal,
) *Sampler {
	return &Sampler{
		bot:       bot,
		tracker:   tracker,
		journal:   journal,
		period:    samplePeriod,
		lastLevel: 0,
	}
}

// Run samples until the context ends. The first sample lands at once
// (the baseline), then the ticker owns the rhythm.
func (s *Sampler) Run(ctx context.Context) {
	if s.journal == nil || s.tracker == nil {
		return
	}
	s.publish()
	ticker := time.NewTicker(s.period)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.publish()
		}
	}
}

// publish reads the tracker and hands the sample to the journal; a
// level change also emits the level mark. A character out of the
// world (level zero of a connecting session) skips the sample.
// The exp field of the tracker is already the cumulative total (the
// Mobius PlayerStat keeps one running number; the level derives from
// the table - see the soak metrics note), so the sample carries it
// as is.
func (s *Sampler) publish() {
	level := s.tracker.SelfLevel()
	if level <= 0 {
		return
	}
	x, y, _, ok := s.tracker.SelfPosition()
	if !ok {
		x, y = 0, 0
	}
	exp := int64(s.tracker.SelfExp())
	adena := int64(s.tracker.InventoryStats().Adena)
	s.journal.Sample(s.bot, Sample{
		Level:  level,
		Exp:    exp,
		Adena:  adena,
		Health: s.tracker.SelfHealthPercent(),
		X:      x,
		Y:      y,
		Phase:  s.tracker.Phase(),
	})
	if s.lastLevel > 0 && level > s.lastLevel {
		s.journal.Level(s.bot, level, exp)
	}
	s.lastLevel = level
}
