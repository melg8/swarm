// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"bufio"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/session"
	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// journalRecords reads the records of a closed journal file.
func journalRecords(t *testing.T, path string) []sessionRecord {
	t.Helper()
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()
	var records []sessionRecord
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var r sessionRecord
		require.NoError(t, json.Unmarshal(line, &r))
		records = append(records, r)
	}
	require.NoError(t, scanner.Err())

	return records
}

// sessionRecord mirrors the wire record of the session package (the
// hunt tests assert on the wire fields, not on the package types, so
// the emission stays pinned across refactors).
type sessionRecord struct {
	T   int64   `json:"t"`
	B   string  `json:"b"`
	E   string  `json:"e"`
	M   string  `json:"m"`
	Mob string  `json:"mob"`
	Lv  int32   `json:"lv"`
	Lvl int32   `json:"lvl"`
	Dur float64 `json:"dur"`
	Hp  float64 `json:"hp"`
	X   int32   `json:"x"`
	Y   int32   `json:"y"`
	R   string  `json:"r"`
	N   int32   `json:"n"`
}

// TestLoopJournalKill verifies the structured kill emission: the mob
// name, the mob level, the fight duration from the engage stamp and
// the health percent land in the journal file.
func TestLoopJournalKill(t *testing.T) {
	bot := state.NewBot("test1")
	bot.SetCharacter("test1", 7, 18, 45000, 50000, -3500, 80, 40)
	bot.SetOnline("test1")
	bot.ApplyUserInfo(state.UserInfo{
		Name: "test1", Level: 5, Race: 1, ClassID: 18,
		X: 45000, Y: 50000, Z: -3500,
		Exp: 2500, MaxHP: 100, CurHP: 87, MaxMP: 50, CurMP: 40,
		CurrentLoad: 10, MaxLoad: 100,
	})
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 9, X: 45100, Y: 50100, Name: "Keltir",
		Attackable: true,
	})
	loop := NewLoop(&fakeGame{}, bot)
	journal, err := session.NewJournal(t.TempDir(), nil)
	require.NoError(t, err)
	loop.SetJournal(journal)

	loop.fightStartFor = 9
	loop.fightStartAt = time.Now().Add(-8 * time.Second)
	now := time.Now()
	loop.journalKill(9, now)
	journal.Close()

	records := journalRecords(t, journal.Path())
	require.Len(t, records, 1)
	require.Equal(t, "kill", records[0].E)
	require.Equal(t, "test1", records[0].B)
	require.Equal(t, "Keltir", records[0].Mob)
	require.InDelta(t, 8.0, records[0].Dur, 1.5)
	require.InDelta(t, 87.0, records[0].Hp, 0.001)
}

// TestLoopJournalDeath verifies the death emission.
func TestLoopJournalDeath(t *testing.T) {
	bot := state.NewBot("test1")
	bot.SetCharacter("test1", 7, 18, 45000, 50000, -3500, 80, 40)
	loop := NewLoop(&fakeGame{}, bot)
	journal, err := session.NewJournal(t.TempDir(), nil)
	require.NoError(t, err)
	loop.SetJournal(journal)

	level := bot.SelfLevel()
	x, y, _, _ := bot.SelfPosition()
	journal.Death(bot.ID(), level, x, y)
	journal.Close()

	records := journalRecords(t, journal.Path())
	require.Len(t, records, 1)
	require.Equal(t, "death", records[0].E)
	require.Equal(t, int32(45000), records[0].X)
	require.Equal(t, int32(50000), records[0].Y)
}

// TestLoopJournalNil verifies the loop runs without a journal: no
// emission path panics (the default of every existing test).
func TestLoopJournalNil(t *testing.T) {
	bot := state.NewBot("test1")
	bot.SetCharacter("test1", 7, 18, 45000, 50000, -3500, 80, 40)
	loop := NewLoop(&fakeGame{}, bot)
	require.Nil(t, loop.journal)
	require.NotPanics(t, func() {
		loop.journalKill(9, time.Now())
	})
}
