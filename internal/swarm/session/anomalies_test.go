// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package session

import (
    "strings"
    "testing"

    "github.com/stretchr/testify/require"
)

// TestRunAnomaliesFindings verifies the ranked report names every
// scripted anomaly of the shared journal and carries the drill-down
// command lines.
func TestRunAnomaliesFindings(t *testing.T) {
    path := writeQueryJournal(t, queryScript())
    var out strings.Builder
    require.NoError(t, RunAnomalies(path, &out))
    text := out.String()

    require.Contains(t, text, "swarm session anomalies")
    require.Contains(t, text, "records")

    // The emergency logout loop of the closed-socket streak.
    require.Contains(t, text, "emergency logout loop")
    require.Contains(t, text, "4 emergency logouts")
    require.Contains(t, text, "closed-socket reconnects")
    require.Contains(t, text, "median session")

    // The repeated decision line of the steering loop.
    require.Contains(t, text, "repeated decision line")
    require.Contains(t, text, "15x")
    require.Contains(t, text, "steering the walk around")

    // The town trip abort loop.
    require.Contains(t, text, "town trip loop")
    require.Contains(t, text, `4 of 4 trips ended "aborted, no walkable path to the shop"`)

    // The fight duration outlier.
    require.Contains(t, text, "fight duration outlier")
    require.Contains(t, text, "Crimson Spider")
    require.Contains(t, text, "1h13m")

    // The death streak.
    require.Contains(t, text, "death streak")
    require.Contains(t, text, "6 deaths in")
    require.Contains(t, text, "2 distinct positions")

    // Every finding carries the ready-made drill-down.
    require.Contains(t, text, "drill: swarm -session-query")
}

// TestRunAnomaliesQuiet verifies a healthy journal renders no findings.
func TestRunAnomaliesQuiet(t *testing.T) {
    records := []record{}
    r := newRecord("unittest2", kindKill, at(10))
    r.Mob = "Dryad"
    r.Lvl = 13
    r.Dur = 18
    records = append(records, r)
    story := newRecord("unittest2", kindStory, at(20))
    story.M = "Hunt: target died, looting"
    records = append(records, story)
    path := writeQueryJournal(t, records)

    var out strings.Builder
    require.NoError(t, RunAnomalies(path, &out))
    require.Contains(t, out.String(), "no anomalies above the thresholds")
}

// TestAnomalyBotPatternNormalize verifies the digit collapsing of the
// pattern normalizer: coordinates and ids fold into one shape.
func TestAnomalyBotPatternNormalize(t *testing.T) {
    bot := newAnomalyBot("unittest1")
    for i := range 14 {
        r := newRecord("unittest1", kindStory, at(i*10))
        r.M = strings.Replace(
            "Hunt: no pickable target: Kaboo Orc (268439512) at 29176 52406",
            "268439512", "26843951"+string(rune('0'+i%10)), 1)
        bot.applyStory(r)
    }
    patterns := bot.topPatterns()
    require.Len(t, patterns, 1)
    require.Equal(t, 14, patterns[0].count)
}

// TestAnomalyBotDeathBursts verifies the sliding death window.
func TestAnomalyBotDeathBursts(t *testing.T) {
    bot := newAnomalyBot("unittest1")
    // Six deaths inside ten minutes flag one burst.
    for i := range 6 {
        r := newRecord("unittest1", kindDeath, at(2000+i*60))
        r.Lv, r.X, r.Y = 18, 42971, 51372
        bot.apply(r)
    }
    // A scattered pair outside the window stays quiet.
    for i := range 2 {
        r := newRecord("unittest1", kindDeath, at(6000+i*1200))
        r.Lv, r.X, r.Y = 18, 1, 1
        bot.apply(r)
    }
    bursts := bot.deathBursts()
    require.Len(t, bursts, 1)
    require.Equal(t, 6, bursts[0].count)
}

// TestAnomalyBotMutedCount verifies the story cap replication.
func TestAnomalyBotMutedCount(t *testing.T) {
    bot := newAnomalyBot("unittest1")
    for i := range storyCapPerMin + 40 {
        r := newRecord("unittest1", kindStory, at(0))
        r.M = "npc spawned: Dryad"
        // Every line lands in the same minute of the anchor day.
        r.T = at(i % 30).Unix()
        bot.applyStory(r)
    }
    require.Equal(t, 40, bot.mutedCount())
}
