// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package session

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

// The anchor moment of the scripted journals: 22:51:30 UTC, the start
// clock of the observed six hour fleet run.
var queryBase = time.Date(2026, 9, 12, 22, 51, 30, 0, time.UTC)

// writeQueryJournal writes a journal file with a scripted multi-bot
// event set the query and anomaly tests share.
func writeQueryJournal(t *testing.T, records []record) string {
    t.Helper()
    dir := t.TempDir()
    path := filepath.Join(dir, "session-query.jsonl")
    var body strings.Builder
    for _, r := range records {
        raw, err := json.Marshal(r)
        require.NoError(t, err)
        body.Write(raw)
        body.WriteString("\n")
    }
    require.NoError(t, os.WriteFile(path, []byte(body.String()), 0o600))

    return path
}

// at returns the record time of an offset in seconds from the anchor.
func at(offset int) time.Time {
    return queryBase.Add(time.Duration(offset) * time.Second)
}

// queryScript builds the shared event set: one bot with a kill streak
// of growing durations, a repeated decision line, a death streak, an
// emergency logout streak, a trip abort loop, and a second bot that
// stays healthy.
func queryScript() []record {
    records := []record{}
    story := func(bot, msg string, offset int) record {
        r := newRecord(bot, kindStory, at(offset))
        r.M = msg

        return r
    }
    // The repeated decision line (a steering loop shape): the varying
    // digits collapse into one pattern key, and the interleaved game
    // events give the context mode non-matching neighbors to print.
    for i := range 15 {
        records = append(records, story("unittest1",
            fmt.Sprintf("Hunt: steering the walk around Kaboo Orc at 29%d76 52%d06", i, i),
            60+i*7))
        records = append(records, story("unittest1", "npc spawned: Dryad", 63+i*7))
    }
    // The kill clock accumulation: the same mob, growing durations.
    for i := range 6 {
        r := newRecord("unittest1", kindKill, at(300+i*40))
        r.Mob = "Kaboo Orc Fighter Lieutenant"
        r.Lvl = 11
        r.Dur = float64(60 + i*40)
        records = append(records, r)
    }
    // One real outlier fight.
    r := newRecord("unittest1", kindKill, at(1500))
    r.Mob = "Crimson Spider"
    r.Lvl = 15
    r.Dur = 4369
    records = append(records, r)
    // The death streak at two positions.
    for i := range 6 {
        d := newRecord("unittest1", kindDeath, at(2000+i*30))
        d.Lv = 18
        d.X = []int32{42971, 47595}[i%2]
        d.Y = []int32{51372, 51569}[i%2]
        records = append(records, d)
    }
    // The emergency logout streak with its closed-socket lost marks.
    for i := range 4 {
        records = append(records, story("unittest1",
            "Hunt: 3 mobs piled on us, emergency logout for 2s", 3000+i*20))
        l := newRecord("unittest1", kindLost, at(3002+i*20))
        l.R = "game connection lost: failed to read packet header: " +
            "read tcp 1.2.3.4:1->1.2.3.4:7777: use of closed network connection"
        records = append(records, l)
        c := newRecord("unittest1", kindConnect, at(3010+i*20))
        c.R = "entered"
        c.M = "level 17"
        records = append(records, c)
    }
    // The town trip abort loop.
    for i := range 4 {
        s := newRecord("unittest1", kindTripStart, at(4000+i*300))
        s.R = "the shop strategy plans purchases worth 62214 adena"
        records = append(records, s)
        e := newRecord("unittest1", kindTripEnd, at(4003+i*300))
        e.R = "aborted, no walkable path to the shop"
        e.Dur = 4
        records = append(records, e)
    }
    // A healthy second bot.
    r = newRecord("unittest2", kindKill, at(500))
    r.Mob = "Dryad"
    r.Lvl = 13
    r.Dur = 18
    records = append(records, r)
    records = append(records, story("unittest2", "Hunt: target died, looting", 510))

    return records
}

// TestRunQueryFiltersByEventKind verifies the events filter selects the
// records of one kind without reading anything else.
func TestRunQueryFiltersByEventKind(t *testing.T) {
    path := writeQueryJournal(t, queryScript())
    var out strings.Builder
    require.NoError(t, RunQuery(path, QueryFilter{
        Bot: "unittest1", Events: []string{"death"},
    }, &out))
    text := out.String()
    require.Contains(t, text, "died at level 18")
    require.NotContains(t, text, "steering")
    require.Contains(t, text, "matched")
}

// TestRunQueryMatchAndContext verifies the regex filter with a time
// context window around every match.
func TestRunQueryMatchAndContext(t *testing.T) {
    path := writeQueryJournal(t, queryScript())
    var out strings.Builder
    require.NoError(t, RunQuery(path, QueryFilter{
        Bot: "unittest1", Match: "steering", Context: 30 * time.Second,
    }, &out))
    text := out.String()
    require.Contains(t, text, "steering")
    // The context prefix marks the non-matching neighbors.
    require.Contains(t, text, "~ ")
}

// TestRunQueryTimeWindow verifies the bare clock window resolves
// against the first record date and bounds the output.
func TestRunQueryTimeWindow(t *testing.T) {
    path := writeQueryJournal(t, queryScript())
    var out strings.Builder
    require.NoError(t, RunQuery(path, QueryFilter{
        Bot: "unittest1", From: "23:59", To: "23:59:30", Events: []string{"kill"},
    }, &out))
    // Every scripted kill sits before 23:16: the minute window sees
    // none of them.
    require.Contains(t, out.String(), "0 matched")
}

// TestRunQueryLimit verifies the record cap.
func TestRunQueryLimit(t *testing.T) {
    path := writeQueryJournal(t, queryScript())
    var out strings.Builder
    require.NoError(t, RunQuery(path, QueryFilter{
        Bot: "unittest1", Events: []string{"story"}, Limit: 3,
    }, &out))
    printed := 0
    for _, line := range strings.Split(out.String(), "\n") {
        if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "~") {
            printed++
        }
    }
    require.LessOrEqual(t, printed, 3)
}

// TestRunQueryBadMatch verifies the regex validation fails fast.
func TestRunQueryBadMatch(t *testing.T) {
    path := writeQueryJournal(t, queryScript())
    var out strings.Builder
    err := RunQuery(path, QueryFilter{Match: "(unclosed"}, &out)
    require.ErrorIs(t, err, errQueryBadRegex)
}

// TestParseQueryTimeNearestDay verifies the overnight resolution: a
// bare 01:30 clock binds to the morning after a 22:51 start.
func TestParseQueryTimeNearestDay(t *testing.T) {
    base := time.Date(2026, 9, 12, 22, 51, 30, 0, time.UTC)
    resolved, err := ParseQueryTime("01:30", base)
    require.NoError(t, err)
    require.Equal(t, 13, resolved.Day())
    require.Equal(t, 1, resolved.Hour())

    resolved, err = ParseQueryTime("23:00", base)
    require.NoError(t, err)
    require.Equal(t, 12, resolved.Day())

    resolved, err = ParseQueryTime("2026-09-13T00:00:00Z", base)
    require.NoError(t, err)
    require.Equal(t, 13, resolved.Day())

    _, err = ParseQueryTime("25:99", base)
    require.ErrorIs(t, err, errQueryBadTime)
}

// TestParseJournalFileWindow verifies the windowed report fold drops
// the records outside the bounds.
func TestParseJournalFileWindow(t *testing.T) {
    path := writeQueryJournal(t, queryScript())
    full, err := ParseJournalFileWindow(path, "", "")
    require.NoError(t, err)
    require.Len(t, full.Order, 2)
    require.Equal(t, 7, full.Aggs["unittest1"].kills)
    // The tight window starts at +1000s: only the +1500s outlier kill
    // and the later events fold.
    from := at(1000).Format("15:04:05")
    tight, err := ParseJournalFileWindow(path, from, "")
    require.NoError(t, err)
    agg := tight.Aggs["unittest1"]
    require.Equal(t, 1, agg.kills)
    require.Len(t, agg.deaths, 6)
}
