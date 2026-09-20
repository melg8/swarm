// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package session

import (
    "bufio"
    "compress/gzip"
    "encoding/json"
    "errors"
    "io"
    "os"
    "path/filepath"
    "strings"
    "sync"
    "sync/atomic"
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

// readLines reads the journal file after Close (everything is flushed).
func readLines(t *testing.T, path string) []record {
    t.Helper()
    file, err := os.Open(path)
    require.NoError(t, err)
    defer file.Close()
    var records []record
    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        line := scanner.Bytes()
        if len(line) == 0 {
            continue
        }
        var r record
        require.NoError(t, json.Unmarshal(line, &r))
        records = append(records, r)
    }
    require.NoError(t, scanner.Err())

    return records
}

// TestJournalLifecycle pins the write path: the records land in the
// file in order, the flat JSON keys stay short and the build line
// rides first.
func TestJournalLifecycle(t *testing.T) {
    dir := t.TempDir()
    journal, err := NewJournal(dir, nil)
    require.NoError(t, err)
    journal.Build("main:abc123 dirty=true")
    at := time.Unix(1700000000, 0)
    journal.Story("unittest1", "Hunt: zone switch to Elven Ruins", at)
    journal.Kill("unittest1", "Kaboo Orc", 4, 8.2, 87.5, 45000, 50000)
    journal.Death("unittest1", 5, -1234, 4567)
    journal.Close()

    lines := readLines(t, journal.Path())
    require.Len(t, lines, 4)
    require.Equal(t, kindBuild, lines[0].E)
    require.Equal(t, "main:abc123 dirty=true", lines[0].V)
    require.Equal(t, kindStory, lines[1].E)
    require.Equal(t, "Hunt: zone switch to Elven Ruins", lines[1].M)
    require.Equal(t, kindKill, lines[2].E)
    require.Equal(t, "Kaboo Orc", lines[2].Mob)
    require.Equal(t, int32(4), lines[2].Lvl)
    require.InDelta(t, 8.2, lines[2].Dur, 0.001)
    require.InDelta(t, 87.5, lines[2].Hp, 0.001)
    require.Equal(t, kindDeath, lines[3].E)
    require.Equal(t, int32(5), lines[3].Lv)
}

// TestJournalWireFormat pins the compactness contract: the kill line
// uses the short flat keys and stays under 120 bytes.
func TestJournalWireFormat(t *testing.T) {
    r := newRecord("unittest1", kindKill, time.Unix(1700000000, 0))
    r.Mob = "Kaboo Orc Fighter"
    r.Lvl = 6
    r.Dur = 12.5
    r.Hp = 74
    data, err := json.Marshal(&r)
    require.NoError(t, err)
    require.Less(t, len(data), 120,
        "the kill wire line must stay compact, got: %s", data)
    require.Contains(t, string(data), `"e":"kill"`)
    require.Contains(t, string(data), `"mob":"Kaboo Orc Fighter"`)
    require.NotContains(t, string(data), `"mob":""`)
}

// TestJournalNilSafe pins the nil receiver contract: every method is a
// no-op, the disabled journal never panics.
func TestJournalNilSafe(t *testing.T) {
    var journal *Journal
    require.Empty(t, journal.Path())
    require.Equal(t, uint64(0), journal.Dropped())
    journal.Build("identity")
    journal.Story("unittest1", "message", time.Now())
    journal.Sample("unittest1", Sample{})
    journal.Kill("unittest1", "mob", 1, 1, 1, 0, 0)
    journal.Death("unittest1", 1, 0, 0)
    journal.TripStart("unittest1", "reason")
    journal.TripEnd("unittest1", "reason", time.Second)
    journal.Buy("unittest1", "item", 1, 10)
    journal.Sell("unittest1", 1)
    journal.Zone("unittest1", "zone", "reason")
    journal.Stall("unittest1", "xp", time.Minute, 0, 0, "engage")
    journal.Repath("unittest1", 1)
    journal.Connect("unittest1", "entered", "")
    journal.Lost("unittest1", "reason")
    journal.Shutdown("reason")
    _, err := journal.Report("unittest1", nil)
    require.ErrorIs(t, err, errNoJournal)
    journal.Close()
}

// TestStoryCap verifies the flood insurance: a story burst past the
// per-minute budget mutes instead of writing, and the muted count
// names the flood.
func TestStoryCap(t *testing.T) {
    dir := t.TempDir()
    journal, err := NewJournal(dir, nil)
    require.NoError(t, err)
    at := time.Unix(1700000000, 0)
    for i := range storyCapPerMin * 2 {
        journal.Story("unittest1", "flood line "+itoa(int64(i)), at)
    }
    journal.Close()

    lines := readLines(t, journal.Path())
    require.Len(t, lines, storyCapPerMin)
    for _, line := range lines {
        require.Equal(t, kindStory, line.E)
    }
    report, err := journal.Report("unittest1", nil)
    require.NoError(t, err)
    require.Contains(t, report, "story muted by the flood cap: "+
        itoa(int64(storyCapPerMin)))
}

// TestStoryCapWindowReset verifies the budget refreshes on the next
// minute boundary.
func TestStoryCapWindowReset(t *testing.T) {
    agg := newBotAgg("unittest1")
    minute := int64(1700000000 / 60)
    for range storyCapPerMin {
        r := newRecord("unittest1", kindStory, time.Unix(minute*60, 0))
        r.M = "line"
        require.True(t, agg.accept(r))
    }
    r := newRecord("unittest1", kindStory, time.Unix(minute*60, 0))
    require.False(t, agg.accept(r))
    next := newRecord("unittest1", kindStory, time.Unix((minute+1)*60, 0))
    require.True(t, agg.accept(next))
    require.Equal(t, 1, agg.storyMuted)
}

// TestJournalReportLive verifies the report renders from the live
// aggregate with the live header block.
func TestJournalReportLive(t *testing.T) {
    dir := t.TempDir()
    journal, err := NewJournal(dir, nil)
    require.NoError(t, err)
    at := time.Unix(1700000000, 0)
    journal.Story("unittest1", "Hunt: the story line", at)
    journal.Kill("unittest1", "Kaboo Orc", 4, 8.2, 87.5, 45000, 50000)
    journal.Close()
    live := &LiveView{
        ID: "unittest1", Status: "online", Phase: "engage", Level: 5,
        ExpPercent: 42.5, Health: 93, Adena: 45123, X: -12345, Y: 98765,
        StartedUnix: at.Unix(),
    }
    report, err := journal.Report("unittest1", live)
    require.NoError(t, err)
    require.Contains(t, report, "swarm session report")
    require.Contains(t, report, "journal: "+journal.Path())
    require.Contains(t, report, "bot: unittest1")
    require.Contains(t, report, "live now: level 5")
    require.Contains(t, report, "Hunt: the story line")

    _, err = journal.Report("unknown", nil)
    require.ErrorIs(t, err, errUnknownBot)
}

// TestJournalFirstRecordFlush pins the immediate first flush: the very
// first record (the build identity line) reaches the disk at once, so
// a run killed inside the first flush period still leaves a readable
// self-describing file instead of a zero byte mystery. The deadline
// sits just below the flush period on purpose: the ticker never fires
// early, so a write observed before it can only come from the first
// record flush.
func TestJournalFirstRecordFlush(t *testing.T) {
    dir := t.TempDir()
    journal, err := NewJournal(dir, nil)
    require.NoError(t, err)
    defer journal.Close()
    journal.Build("main:abc123")

    deadline := time.Now().Add(1900 * time.Millisecond)
    for time.Now().Before(deadline) {
        info, err := os.Stat(journal.Path())
        if err == nil && info.Size() > 0 {
            return
        }
        time.Sleep(5 * time.Millisecond)
    }
    t.Fatal("the first record stayed buffered: the file is still empty " +
        "before the flush period could have fired")
}

// TestJournalUniqueNames verifies two processes (or a restart) never
// clobber each other: the file name carries the process id.
func TestJournalUniqueNames(t *testing.T) {
    dir := t.TempDir()
    first, err := NewJournal(dir, nil)
    require.NoError(t, err)
    first.Close()
    second, err := NewJournal(dir, nil)
    require.NoError(t, err)
    second.Close()
    require.NotEqual(t, first.Path(), second.Path())
    require.Equal(t, filepath.Dir(first.Path()), filepath.Dir(second.Path()))
}

// TestJournalMultipleBots verifies the fleet share: one file, the bot
// id separates the streams.
func TestJournalMultipleBots(t *testing.T) {
    dir := t.TempDir()
    journal, err := NewJournal(dir, nil)
    require.NoError(t, err)
    journal.Kill("unittest1", "Kaboo Orc", 4, 8.2, 87.5, 45000, 50000)
    journal.Kill("unittest2", "Wolf", 2, 3.1, 99, 1, 2)
    journal.Close()

    lines := readLines(t, journal.Path())
    require.Len(t, lines, 2)
    require.Equal(t, "unittest1", lines[0].B)
    require.Equal(t, "unittest2", lines[1].B)
}

// TestGroupDigits pins the report number rendering.
func TestGroupDigits(t *testing.T) {
    require.Equal(t, "0", groupDigits(0))
    require.Equal(t, "9999", groupDigits(9999))
    require.Equal(t, "12 345", groupDigits(12345))
    require.Equal(t, "1 234 567", groupDigits(1234567))
    require.Equal(t, "12 345", groupDigits(-12345))
}

// TestDurText pins the duration rendering.
func TestDurText(t *testing.T) {
    require.Equal(t, "45s", durText(45))
    require.Equal(t, "2m30s", durText(150))
    require.Equal(t, "1h5m", durText(3900))
}

// TestSignPrefix pins the delta sign rendering.
func TestSignPrefix(t *testing.T) {
    require.Equal(t, "+", signPrefix(5))
    require.Equal(t, "-", signPrefix(-5))
}

// TestHasGzipExt pins the gz suffix detection.
func TestHasGzipExt(t *testing.T) {
    require.True(t, hasGzipExt("session.jsonl.gz"))
    require.False(t, hasGzipExt("session.jsonl"))
    require.False(t, hasGzipExt("session.gz.jsonl"))
}

// TestReportEmptyBot renders the report of a bot with no events: the
// sections still land, no zero-division panics.
func TestReportEmptyBot(t *testing.T) {
    agg := newBotAgg("unittest1")
    report := renderReport("some/path.jsonl", agg, nil)
    require.Contains(t, report, "swarm session report")
    require.Contains(t, report, "no events recorded")
    require.Contains(t, report, "kills: 0")
    require.Contains(t, report, "xp stalls: 0")
    require.True(t, strings.HasSuffix(report, "\n"))
}

// TestGzipFileKeepsTheOriginalWhenTheCopyFails pins the data
// guarantee of the rotation: a gzip copy failure (a disk full under
// the rotated segment) keeps the original on disk and drops the
// partial archive - the old order removed the source unconditionally
// and destroyed the record the comment promised to protect.
func TestGzipFileKeepsTheOriginalWhenTheCopyFails(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "session.jsonl")
    const payload = "the rotated segment payload"
    require.NoError(t, os.WriteFile(path, []byte(payload), 0o600))

    orig := gzipCopy
    gzipCopy = func(io.Writer, io.Reader) (int64, error) {
        return 0, errors.New("disk full")
    }
    defer func() { gzipCopy = orig }()

    var wg sync.WaitGroup
    wg.Add(1)
    gzipFile(path, &wg, nil)
    wg.Wait()

    got, err := os.ReadFile(path)
    require.NoError(t, err, "the original segment must survive")
    require.Equal(t, payload, string(got))
    require.NoFileExists(t, path+".gz", "the partial archive must go")
}

// TestGzipFileCompressesOnSuccess pins the happy path: the original
// goes away, the archive round-trips byte for byte.
func TestGzipFileCompressesOnSuccess(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "session.jsonl")
    payload := strings.Repeat("the journal line\n", 64)
    require.NoError(t, os.WriteFile(path, []byte(payload), 0o600))

    var wg sync.WaitGroup
    wg.Add(1)
    gzipFile(path, &wg, nil)
    wg.Wait()

    require.NoFileExists(t, path)
    file, err := os.Open(path + ".gz")
    require.NoError(t, err)
    defer file.Close()
    gz, err := gzip.NewReader(file)
    require.NoError(t, err)
    got, err := io.ReadAll(gz)
    require.NoError(t, err)
    require.NoError(t, gz.Close())
    require.Equal(t, payload, string(got))
}

// TestRotateKeepsTheWriterWhenTheNextSegmentFails pins the rotation
// failure path: the next segment opens before the old file closes, so
// a failed open leaves the writer on the still-open file and the
// later records keep landing. The old order closed the file first and
// stranded every later record on the closed handle - the report kept
// counting while the file went silent.
func TestRotateKeepsTheWriterWhenTheNextSegmentFails(t *testing.T) {
    dir := t.TempDir()
    file, path, err := createJournalFile(dir)
    require.NoError(t, err)
    j := &Journal{
        dir:     dir,
        path:    path,
        counter: &countingWriter{inner: file, written: 0},
        wg:      sync.WaitGroup{},
        mu:      sync.RWMutex{},
        aggs:    map[string]*botAgg{},
        seq:     0,
        written: 0,
        dropped: atomic.Uint64{},
    }
    j.install(file)

    before := newRecord("unittest1", kindStory, time.Now())
    before.M = "before"
    j.write(before)
    j.flushFile()

    // The segment paths move into a missing directory: the rotate
    // open fails while the active file stays healthy.
    j.dir = filepath.Join(dir, "missing")
    j.written = rotateBytes
    j.counter.written = rotateBytes
    j.flushFile()

    after := newRecord("unittest1", kindStory, time.Now())
    after.M = "after"
    j.write(after)
    j.flushFile()
    j.closeFile()

    lines := readLines(t, path)
    require.Len(t, lines, 2, "both records must land despite the failed rotate")
    require.Equal(t, "before", lines[0].M)
    require.Equal(t, "after", lines[1].M)
}
