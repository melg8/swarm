// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package session

import (
	"context"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// enterWorldCharacter fills the tracker with a live character state.
func enterWorldCharacter(tracker *state.Bot) {
	tracker.SetCharacter("test1", 7, 18, 100, 200, -3000, 80, 40)
	tracker.SetOnline("test1")
	tracker.SetPhase("engage")
	tracker.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: 5, Race: 1, ClassID: 18,
		X: 100, Y: 200, Z: -3000,
		Exp: 2500, MaxHP: 100, CurHP: 90, MaxMP: 50, CurMP: 40,
		CurrentLoad: 10, MaxLoad: 100,
	})
}

// levelUpCharacter bumps the character to the next level.
func levelUpCharacter(tracker *state.Bot) {
	tracker.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: 6, Race: 1, ClassID: 18,
		X: 100, Y: 200, Z: -3000,
		Exp: 3000, MaxHP: 100, CurHP: 90, MaxMP: 50, CurMP: 40,
		CurrentLoad: 10, MaxLoad: 100,
	})
}

// TestSamplerPublishes verifies the sampler reads the tracker and the
// samples land in the journal file (a character in the world samples,
// a connecting one does not, a level up emits the level mark). The
// assertions read the file after Close: the flusher drained the queue
// by then, so the test stays deterministic.
func TestSamplerPublishes(t *testing.T) {
	// A fresh tracker has no character: nothing lands in the journal.
	empty := state.NewBot("test1")
	journalEmpty, err := NewJournal(t.TempDir(), nil)
	require.NoError(t, err)
	NewSampler("test1", empty, journalEmpty).publish()
	journalEmpty.Close()
	require.Empty(t, readLines(t, journalEmpty.Path()))

	// The character enters the world: the baseline sample lands with
	// the level, the cumulative exp and the phase; a level up emits
	// the level mark.
	tracker := state.NewBot("test1")
	journal, err := NewJournal(t.TempDir(), nil)
	require.NoError(t, err)
	enterWorldCharacter(tracker)
	sampler := NewSampler("test1", tracker, journal)
	sampler.publish()
	levelUpCharacter(tracker)
	sampler.publish()
	journal.Close()

	lines := readLines(t, journal.Path())
	require.Len(t, lines, 3)
	require.Equal(t, kindSample, lines[0].E)
	require.Equal(t, int32(5), lines[0].Lv)
	require.Equal(t, int64(2500), lines[0].Xp)
	require.Equal(t, "engage", lines[0].Ph)
	require.Equal(t, kindSample, lines[1].E)
	require.Equal(t, int32(6), lines[1].Lv)
	require.Equal(t, kindLevel, lines[2].E)
	require.Equal(t, int32(6), lines[2].Lv)
}

// TestSamplerRunStops verifies the run loop exits with the context.
func TestSamplerRunStops(t *testing.T) {
	tracker := state.NewBot("test1")
	tracker.SetCharacter("test1", 7, 18, 100, 200, -3000, 80, 40)
	journal, err := NewJournal(t.TempDir(), nil)
	require.NoError(t, err)
	defer journal.Close()

	ctx, cancel := context.WithCancel(context.Background())
	sampler := NewSampler("test1", tracker, journal)
	done := make(chan struct{})
	go func() {
		sampler.Run(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the sampler did not stop with the context")
	}
}

// TestSamplerNilJournal verifies the disabled journal is a no-op.
func TestSamplerNilJournal(t *testing.T) {
	tracker := state.NewBot("test1")
	enterWorldCharacter(tracker)
	sampler := NewSampler("test1", tracker, nil)
	require.NotPanics(t, func() {
		sampler.publish()
		sampler.Run(context.Background())
	})
}

// TestNilJournalRun verifies Run exits at once without a journal.
func TestNilJournalRun(t *testing.T) {
	tracker := state.NewBot("test1")
	sampler := NewSampler("test1", tracker, nil)
	done := make(chan struct{})
	go func() {
		sampler.Run(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run without a journal must return at once")
	}
}
