// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package webserver

import (
    "net/http"
    "net/http/httptest"
    "strconv"
    "strings"
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// TestBotDumpEndpoint pins the state dump report: the plain text
// carries the character sheet, the position, the inventory, the
// objects around, the walk plan and the deep event window - the
// debugging material the HUD button copies to the clipboard.
func TestBotDumpEndpoint(t *testing.T) {
    server, bot := newTestServer(t)
    bot.ApplyStatusUpdate(100, []state.Attribute{
        {ID: state.AttrCurHP, Value: 40},
    })
    bot.ApplyWaitType(state.WaitType{ObjectID: 100, Sitting: true})
    bot.RecordEvent("sat down to rest")
    bot.RecordEvent("Hunt: outside the hunting zone, pathfinding back")
    bot.SetHuntingZone(46112, 41500, 450)
    bot.SetWalkPlan(state.WalkPlan{
        Origin: &state.WalkPoint{X: 45800, Y: 41700, Z: -3500},
        Points: []state.WalkPoint{
            {X: 46000, Y: 41600, Z: -3500},
            {X: 46112, Y: 41500, Z: -3510},
        },
        Index: 1,
        Dest:  &state.WalkPoint{X: 46150, Y: 41480, Z: -3512},
    })

    recorder := httptest.NewRecorder()
    server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
        http.MethodGet, "/api/bots/unittest1/dump", nil))

    require.Equal(t, http.StatusOK, recorder.Code)
    require.Equal(t, "text/plain; charset=utf-8",
        recorder.Header().Get("Content-Type"))
    report := recorder.Body.String()

    // The header identifies the code state and the session.
    require.Contains(t, report, "swarm state dump")
    require.Contains(t, report, "build: ")
    require.Contains(t, report, "bot: unittest1 (status online")

    // The character sheet: the position, the vitals, the sit state.
    require.Contains(t, report, "name: unittest1 (object 100)")
    require.Contains(t, report, "position: x 45000, y 50000, z -3500")
    require.Contains(t, report, "sitting true")

    // The objects around, the walk plan and the zone. The passed
    // waypoint carries its timing suffix (the plan published with the
    // cursor already past it pre-fills the arrival with the publish
    // moment - the zero segments of a mid walk publish).
    require.Contains(t, report, "Keltir")
    require.Contains(t, report,
        "walk plan (2 waypoints, aiming at wp 1):")
    require.Contains(t, report, "  from 45800 41700 -3500")
    require.Contains(t, report, "  started ")
    require.Contains(t, report,
        "wp 0: 46000 41600 -3500 (passed, t+0.0s, segment 0.0s)")
    require.Contains(t, report, "wp 1: 46112 41500 -3510  <-- TARGET")
    require.Contains(t, report, "  dest 46150 41480 -3512")
    require.Contains(t, report, "hunting zone: center 46112 41500, half 450")

    // The event log mirrors the game events and the hunt decisions.
    require.Contains(t, report, "sat down to rest")
    require.Contains(t, report, "Hunt: outside the hunting zone")
}

// TestWalkPlanSectionSearchWord pins the search word of the walk
// plan header (the repro contract of the 3D pathfind link): a mesh
// plan names the word (the priced search needs no filter name) so
// the report names the search the link replays, the direct segments (no
// mesh search) keep the bare header.
func TestWalkPlanSectionSearchWord(t *testing.T) {
    paths := []state.WalkPoint{{X: 1, Y: 2, Z: 3}}

    var b strings.Builder
    writeWalkPlanSection(&b, "walk plan (", paths, nil, 0, nil,
        &state.WalkSearch{}, time.Time{}, time.Time{}, nil)
    require.Contains(t, b.String(),
        "walk plan (1 waypoints, mesh, aiming at wp 0):")

    b.Reset()
    writeWalkPlanSection(&b, "walk plan (", paths, nil, 0, nil,
        &state.WalkSearch{Approach: 150},
        time.Time{}, time.Time{}, nil)
    require.Contains(t, b.String(),
        "walk plan (1 waypoints, mesh, aiming at wp 0):")

    b.Reset()
    writeWalkPlanSection(&b, "walk plan (", paths, nil, 0, nil,
        nil, time.Time{}, time.Time{}, nil)
    require.Contains(t, b.String(),
        "walk plan (1 waypoints, aiming at wp 0):")
}

// TestWalkPlanSectionTiming pins the timing suffixes of the walk
// plan section: a passed waypoint prints its moment on the walk
// timeline (t+) and the segment duration that ended there, the aimed one
// prints the time the walk already spends on it (the stuck number),
// the future ones stay plain, and the sub minute precision folds
// into the minute shape for the long segments.
func TestWalkPlanSectionTiming(t *testing.T) {
    now := time.Now()
    start := now.Add(-90 * time.Second)
    wpAt := []time.Time{
        start.Add(10400 * time.Millisecond),
        start.Add(30200 * time.Millisecond),
        {},
        {},
    }
    var b strings.Builder
    writeWalkPlanSection(&b, "walk plan (",
        []state.WalkPoint{
            {X: 1, Y: 2, Z: 3},
            {X: 4, Y: 5, Z: 6},
            {X: 7, Y: 8, Z: 9},
            {X: 10, Y: 11, Z: 12},
        },
        &state.WalkPoint{X: 0, Y: 0, Z: 0}, 2,
        &state.WalkPoint{X: 10, Y: 11, Z: 12},
        nil,
        start, now.Add(-5*time.Second), wpAt)
    report := b.String()

    require.Contains(t, report, "walk plan (4 waypoints, aiming at wp 2):")
    require.Contains(t, report, "  started ")
    require.Contains(t, report, ", last seen ")
    require.Contains(t, report, " on the walk\n")
    require.Contains(t, report,
        "wp 0: 1 2 3 (passed, t+10.4s, segment 10.4s)")
    require.Contains(t, report,
        "wp 1: 4 5 6 (passed, t+30.2s, segment 19.8s)")
    // The aimed waypoint measures to the moment the plan was last
    // seen alive: (now-5s) - (start+30.2s) = 54.8s.
    require.Contains(t, report, "wp 2: 7 8 9  <-- TARGET (walking 54.8s)")
    require.Contains(t, report, "wp 3: 10 11 12\n",
        "the future waypoints stay plain")

    // The long segments fold into the minute shape.
    longStart := now.Add(-3 * time.Minute)
    b.Reset()
    writeWalkPlanSection(&b, "walk plan (",
        []state.WalkPoint{{X: 1, Y: 2, Z: 3}, {X: 4, Y: 5, Z: 6}},
        nil, 1, nil, nil,
        longStart, now.Add(-time.Second),
        []time.Time{longStart.Add(65300 * time.Millisecond), {}})
    report = b.String()
    require.Contains(t, report,
        "wp 0: 1 2 3 (passed, t+1m5s, segment 1m5s)")
    require.Contains(t, report, "<-- TARGET (walking 1m54s)")
}

// TestWalkPlanSectionUntimed pins the plain shape: a walk without a
// timing view (the zero start, the empty arrivals) prints the legacy
// lines - the parser and the owner eye read the same either way.
func TestWalkPlanSectionUntimed(t *testing.T) {
    var b strings.Builder
    writeWalkPlanSection(&b, "last walk plan (",
        []state.WalkPoint{{X: 1, Y: 2, Z: 3}}, nil, 0, nil, nil,
        time.Time{}, time.Time{}, nil)
    report := b.String()

    require.Contains(t, report, "wp 0: 1 2 3  <-- TARGET\n")
    require.NotContains(t, report, "walking")
    require.NotContains(t, report, "t+")
    require.NotContains(t, report, "started")
}

// TestBotDumpUnknownBot: the dump of an unknown bot is a 404 like the
// other per bot endpoints.
func TestBotDumpUnknownBot(t *testing.T) {
    server, _ := newTestServer(t)
    recorder := httptest.NewRecorder()
    server.httpServer.Handler.ServeHTTP(recorder, httptest.NewRequest(
        http.MethodGet, "/api/bots/nobody/dump", nil))

    require.Equal(t, http.StatusNotFound, recorder.Code)
}

// TestBuildStateDumpEventWindow pins the deep event window: the dump
// reads up to dumpEventLimit events where the snapshot streams one
// hundred - bounded by the ring capacity (512) when more landed, the
// full story of a stuck session stays readable.
func TestBuildStateDumpEventWindow(t *testing.T) {
    bot := state.NewBot("acc1")
    bot.SetCharacter("acc1", 100, 18, 45000, 50000, -3500, 50, 30)
    for i := range 700 {
        bot.RecordEvent("event " + strconv.Itoa(i) + " " +
            strings.Repeat("x", 300))
    }
    report := BuildStateDump(bot)
    require.Contains(t, report, "events (512):")
    require.Contains(t, report, "event 699 ",
        "the newest events survive")
    require.Contains(t, report, "event 188 ",
        "the ring head is the oldest event kept")
    require.NotContains(t, report, "event 187 ",
        "the ring evicted the older half")
    require.NotContains(t, report, "event 0 ",
        "the first events rolled out of the ring")
}

// TestDumpSlotNames pins the body part mask labels of the equipment
// lines against the Mobius BodyPart enum: the jewel and armor masks
// (0x40 head, 0x08 neck, 0x800 legs, 0x1000 feet) once printed as
// shifted slot names (a Cloth Cap read as lfinger, a Necklace of Magic
// as lear ear) and made a healthy paperdoll look corrupted in the
// field reports.
func TestDumpSlotNames(t *testing.T) {
    require.Equal(t, "underwear", dumpSlotName(0x01))
    require.Equal(t, "rear ear", dumpSlotName(0x02))
    require.Equal(t, "lear ear", dumpSlotName(0x04))
    require.Equal(t, "earring", dumpSlotName(0x06))
    require.Equal(t, "necklace", dumpSlotName(0x08))
    require.Equal(t, "rfinger", dumpSlotName(0x10))
    require.Equal(t, "lfinger", dumpSlotName(0x20))
    require.Equal(t, "ring", dumpSlotName(0x30))
    require.Equal(t, "head", dumpSlotName(0x40))
    require.Equal(t, "rhand", dumpSlotName(0x80))
    require.Equal(t, "lhand", dumpSlotName(0x100))
    require.Equal(t, "gloves", dumpSlotName(0x200))
    require.Equal(t, "chest", dumpSlotName(0x400))
    require.Equal(t, "legs", dumpSlotName(0x800))
    require.Equal(t, "feet", dumpSlotName(0x1000))
    require.Equal(t, "back", dumpSlotName(0x2000))
    require.Equal(t, "lrhand", dumpSlotName(0x4000))
    require.Equal(t, "full armor", dumpSlotName(0x8000))
    require.Equal(t, "hair", dumpSlotName(0x10000))
    require.Equal(t, "part 0x20000", dumpSlotName(0x20000))
}

// TestDumpEmptySlots pins the paperdoll hole line of the dump: the
// equipment section only printed the occupied slots, so a character
// farming without its legs armor (the 2026-09-12 04:58 pantsless
// report) read as a fine outfit - the empty families now name
// themselves below the worn pieces. The two hand weapon blocks the
// left hand and the one-piece armor the segments, and a half-filled
// pair names its empty half.
func TestDumpEmptySlots(t *testing.T) {
    bot := state.NewBot("acc1")
    bot.SetCharacter("unittest2", 100, 18, 38344, 46248, -3592, 339, 137)
    // The dump character of the report: every slot filled except the
    // legs, one earring worn (the other half empty).
    bot.ApplyItemList([]state.InventoryItem{
        {ObjectID: 1, ItemID: 20, Count: 1, Equipped: true, BodyPart: 0x100},
        {ObjectID: 2, ItemID: 22, Count: 1, Equipped: true, BodyPart: 0x400},
        {ObjectID: 3, ItemID: 43, Count: 1, Equipped: true, BodyPart: 0x40},
        {ObjectID: 4, ItemID: 49, Count: 1, Equipped: true, BodyPart: 0x200},
        {ObjectID: 5, ItemID: 37, Count: 1, Equipped: true, BodyPart: 0x1000},
        {ObjectID: 6, ItemID: 112, Count: 1, Equipped: true, BodyPart: 0x6},
        {ObjectID: 7, ItemID: 153, Count: 1, Equipped: true, BodyPart: 0x80},
    })

    report := BuildStateDump(bot)

    require.Contains(t, report, "empty slots: legs, back, necklace, earring half",
        "the dump names the missing legs armor of the report")
    require.NotContains(t, report, "empty slots: rhand",
        "the worn pieces never name their own slots empty")
}

// TestDumpEmptySlotsBlockers pins the family blockers: a two hand
// weapon fills the right hand and blocks the left hand, a one-piece
// armor fills the chest and blocks the segments - neither names the
// blocked slot a hole.
func TestDumpEmptySlotsBlockers(t *testing.T) {
    bot := state.NewBot("acc1")
    bot.SetCharacter("unittest2", 100, 18, 38344, 46248, -3592, 339, 137)
    bot.ApplyItemList([]state.InventoryItem{
        {ObjectID: 1, ItemID: 1333, Count: 1, Equipped: true, BodyPart: 0x4000},
    })

    report := BuildStateDump(bot)

    require.Contains(t, report,
        "empty slots: head, gloves, chest, legs, feet, back, necklace",
        "the two hand weapon blocks the hands out of the hole list")
    require.NotContains(t, report, "empty slots: lhand",
        "the blocked left hand never names a hole")
    require.NotContains(t, report, "empty slots: rhand",
        "the filled right hand never names a hole")
}

// TestDumpBuffsSection pins the active effect section of the issue
// #37 round: the dump lists every running buff with its level, the
// remaining seconds over the full duration and the derived applied
// age, so a problem report answers "what was up and for how long"
// without the web UI (an empty strip prints its zero count too -
// the guide buff diagnosis of issue #35 needed exactly that).
func TestDumpBuffsSection(t *testing.T) {
    bot := state.NewBot("buffed")
    bot.SetCharacter("buffed", 100, 18, 45000, 50000, -3500, 50, 30)
    report := BuildStateDump(bot)
    require.Contains(t, report, "buffs (0):",
        "the empty strip prints its zero count")

    bot.SetBuffs([]state.BuffEntry{
        {SkillID: 1204, Level: 1, Time: 1200},
        {SkillID: 1068, Level: 3, Time: 900},
    })
    report = BuildStateDump(bot)
    require.Contains(t, report, "buffs (2):")
    require.Contains(t, report, "(1204) lvl 1: 1200/1200s left",
        "the fresh guide Wind Walk reads full")
    require.Contains(t, report, "applied ~0s ago",
        "the fresh cast derives a zero age")
    // The Might entry reports 900 seconds left; the skill stats
    // know the full abnormal time of the level 3 cast (1200s), so
    // the strip reads 900 of 1200 - a mid-buff observation with the
    // applied age derived from the gap (the first-observation rule
    // of SetBuffs, the honest reading of a login inside a running
    // buff).
    require.Contains(t, report, "(1068) lvl 3: 900/1200s left",
        "the Might entry carries its level and the known full time")
    require.Contains(t, report, "applied ~5m0s ago",
        "the mid-buff observation derives its age from the gap")
}
