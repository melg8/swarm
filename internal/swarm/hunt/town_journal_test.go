// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/session"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// journalTestLoop prepares a level 5 character with a journal and a standing
// position (the trip exit paths read the paperdoll and the inventory).
func journalTestLoop(t *testing.T) (*Loop, *session.Journal) {
    t.Helper()
    bot := state.NewBot("unittest1")
    bot.SetCharacter("unittest1", 7, 18, 45000, 50000, -3500, 80, 40)
    bot.SetOnline("unittest1")
    loop := NewLoop(&fakeGame{}, bot)
    journal, err := session.NewJournal(t.TempDir(), nil)
    require.NoError(t, err)
    loop.SetJournal(journal)

    return loop, journal
}

// TestZoneReturnWalkWritesNoTripEnd pins the accounting fix: the walk home
// after a respawn borrows the trip waypoint machinery but is no town trip, so
// it writes no trip record that the report would count against no start.
func TestZoneReturnWalkWritesNoTripEnd(t *testing.T) {
    loop, journal := journalTestLoop(t)
    loop.zoneReturnWalk = true
    loop.tripStart = time.Now().Add(-30 * time.Second)
    loop.endTownTrip("a target stands inside the zone")
    journal.Close()

    require.False(t, loop.zoneReturnWalk)
    require.True(t, loop.tripEndedAt.IsZero(),
        "the return walk must not arm the town trip cooldown")
    require.Empty(t, journalRecords(t, journal.Path()))
}

// TestTownTripEndWritesOneTripEnd pins the real trip exit next to it: the
// record carries the reason, the duration and the cooldown.
func TestTownTripEndWritesOneTripEnd(t *testing.T) {
    loop, journal := journalTestLoop(t)
    loop.tripStart = time.Now().Add(-time.Minute)
    loop.endTownTrip("back at the farm spot")
    journal.Close()

    require.False(t, loop.tripEndedAt.IsZero())
    records := journalRecords(t, journal.Path())
    require.Len(t, records, 1)
    require.Equal(t, "trip-end", records[0].E)
    require.Equal(t, "unittest1", records[0].B)
    require.Equal(t, "back at the farm spot", records[0].R)
    require.InDelta(t, 60.0, records[0].Dur, 1.5)
}
