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
    "log"
    "os"
    "path/filepath"
    "sync"
    "sync/atomic"
    "time"
)

// Journal tuning: the flush period bounds the loss of a crashed run, the
// channel depth rides out a slow disk burst, the rotation size keeps a runaway
// loop from filling the disk (a segment gzips to a tenth) and the story cap
// mutes a flood (a healthy bot writes well under thirty lines a minute).
const (
    flushPeriod     = 2 * time.Second
    eventQueueDepth = 4096
    rotateBytes     = 64 << 20
    storyCapPerMin  = 120
    writerBufferLen = 1 << 16
)

// The report errors of a disabled or unknown journal.
var (
    errNoJournal  = errors.New("session journal is disabled")
    errUnknownBot = errors.New("bot has no session journal aggregate")
)

// Journal is the append-only session journal of one process: every bot
// writes into one JSONL file, each record carrying its bot id. A single
// writer goroutine owns the file (the callers never block, so neither the hunt
// tick nor the tracker locks wait on disk) and rotates it at rotateBytes with
// the copy gzipped in the background; the aggregator rides it so the report
// sees the flushed truth. The first record flushes at once, so a run killed
// inside the first flush period keeps the build identity line. A nil *Journal
// is a no-op everywhere (-session-dir "" turns it off).
type Journal struct {
    dir         string
    path        string
    logger      *log.Logger
    queue       chan record
    closed      atomic.Bool
    done        chan struct{}
    wg          sync.WaitGroup
    mu          sync.RWMutex
    aggs        map[string]*botAgg
    file        *os.File
    writer      *bufio.Writer
    counter     *countingWriter
    enc         *json.Encoder
    seq         int
    written     int64
    dropped     atomic.Uint64
    firstRecord bool
}

// NewJournal opens the journal file in dir (created when missing) and
// starts the writer goroutine. The caller owns the returned journal
// and must Close it on shutdown.
func NewJournal(dir string, logger *log.Logger) (*Journal, error) {
    if err := os.MkdirAll(dir, 0o755); err != nil {
        return nil, err //nolint:wrapcheck // os error already carries the path
    }
    file, path, err := createJournalFile(dir)
    if err != nil {
        return nil, err //nolint:wrapcheck // os error already carries the path
    }
    j := &Journal{ //nolint:exhaustruct_v5 // closed is a working zero
        dir:         dir,
        path:        path,
        logger:      logger,
        queue:       make(chan record, eventQueueDepth),
        done:        make(chan struct{}),
        wg:          sync.WaitGroup{},
        mu:          sync.RWMutex{},
        aggs:        make(map[string]*botAgg),
        file:        nil,
        writer:      nil,
        counter:     &countingWriter{inner: nil, written: 0},
        enc:         nil,
        seq:         0,
        written:     0,
        dropped:     atomic.Uint64{},
        firstRecord: true,
    }
    j.install(file)
    j.wg.Add(1)
    go j.flusher()

    return j, nil
}

// createJournalFile opens a fresh journal file named after the start timestamp
// and the process id; the attempt counter disambiguates two journals of the
// same second and the exclusive create refuses an existing file.
func createJournalFile(dir string) (*os.File, string, error) {
    stamp := time.Now().Format("20060102-150405")
    for attempt := range 64 {
        name := "session-" + stamp + "-" + itoa(int64(os.Getpid()))
        if attempt > 0 {
            name += "-" + itoa(int64(attempt))
        }
        path := filepath.Join(dir, name+".jsonl")
        file, err := os.OpenFile(
            path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644)
        if err == nil {
            return file, path, nil
        }
        if !errors.Is(err, os.ErrExist) {
            return nil, "", err
        }
    }

    return nil, "", os.ErrExist
}

// install sets up the buffered writing stack over an opened file.
func (j *Journal) install(file *os.File) {
    j.file = file
    j.counter.inner = file
    j.writer = bufio.NewWriterSize(j.counter, writerBufferLen)
    j.enc = json.NewEncoder(j.writer)
    j.enc.SetEscapeHTML(false)
}

// Path returns the journal file path ("" on a nil journal).
func (j *Journal) Path() string {
    if j == nil {
        return ""
    }

    return j.path
}

// Dropped returns the count of records the queue refused: a growing non-zero
// count means the disk stalls the writer.
func (j *Journal) Dropped() uint64 {
    if j == nil {
        return 0
    }

    return j.dropped.Load()
}

// send hands one record to the writer without blocking: a full queue drops the
// record and counts it (the journal is diagnostics only).
func (j *Journal) send(r record) {
    if j == nil || j.closed.Load() {
        return
    }
    select {
    case j.queue <- r:
    default:
        j.dropped.Add(1)
    }
}

// flusher is the single writer goroutine: it drains the queue into the
// buffered file, flushes on the period ticker and exits when the journal closes
// and the queue drains. The first record lands on disk immediately, so an empty
// journal means a run that never got past NewJournal.
func (j *Journal) flusher() {
    defer j.wg.Done()
    ticker := time.NewTicker(flushPeriod)
    defer ticker.Stop()
    for {
        select {
        case r := <-j.queue:
            j.write(r)
            if j.firstRecord {
                j.firstRecord = false
                j.flushFile()
            }
        case <-ticker.C:
            j.flushFile()
        case <-j.done:
            j.drain()
            j.flushFile()
            j.closeFile()

            return
        }
    }
}

// drain empties the queue without blocking (Close flipped the flag so
// no new records can enter).
func (j *Journal) drain() {
    for {
        select {
        case r := <-j.queue:
            j.write(r)
        default:
            return
        }
    }
}

// write encodes one record into the file and folds it into the aggregate of
// its bot. The muted count rides the aggregate, so the report names the story
// cap drops. The aggregate is touched under the write lock only.
func (j *Journal) write(r record) {
    j.mu.Lock()
    agg := j.lookupLocked(r.B)
    allowed := agg.accept(r)
    if allowed {
        agg.apply(r)
    }
    j.mu.Unlock()
    if !allowed {
        return
    }
    if err := j.enc.Encode(&r); err != nil && j.logger != nil {
        j.logger.Printf("Error session journal encode: %v", err)
    }
}

// lookupLocked returns the aggregate of one bot, creating it on first
// sight. The caller must hold the write lock.
func (j *Journal) lookupLocked(bot string) *botAgg {
    agg, ok := j.aggs[bot]
    if !ok {
        agg = newBotAgg(bot)
        j.aggs[bot] = agg
    }

    return agg
}

// flushFile pushes the buffered records to the operating system and
// rotates the file past the size cap.
func (j *Journal) flushFile() {
    if err := j.writer.Flush(); err != nil && j.logger != nil {
        j.logger.Printf("Error session journal flush: %v", err)

        return
    }
    j.written = j.counter.written
    if j.written >= rotateBytes {
        j.rotate()
    }
}

// rotate gzips the filled segment in the background and opens the next one;
// the sequence numbers order the segments lexically after the base file. The
// next segment opens before the old one closes, so a failed open keeps the
// writer on the still-open file and the size cap re-arms the attempt on each
// later flush (closing first would strand the later records).
func (j *Journal) rotate() {
    rotated := j.path
    if j.seq > 0 {
        rotated = j.segmentPath(j.seq)
    }
    next, err := os.OpenFile(
        j.segmentPath(j.seq+1), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
    if err != nil {
        if j.logger != nil {
            j.logger.Printf("Error session journal rotate: %v", err)
        }

        return
    }
    if err := j.file.Close(); err != nil && j.logger != nil {
        j.logger.Printf("Error session journal close: %v", err)
    }
    j.wg.Add(1)
    go gzipFile(rotated, &j.wg, j.logger)
    j.seq++
    j.written = 0
    j.counter.written = 0
    j.install(next)
}

// segmentPath builds the numbered file name of a rotated segment.
func (j *Journal) segmentPath(seq int) string {
    base := filepath.Base(j.path)
    ext := filepath.Ext(base)
    stem := base[:len(base)-len(ext)]

    return filepath.Join(j.dir, stem+"."+itoa(int64(seq))+ext)
}

// closeFile flushes and closes the active segment.
func (j *Journal) closeFile() {
    if err := j.writer.Flush(); err != nil && j.logger != nil {
        j.logger.Printf("Error session journal flush: %v", err)
    }
    if err := j.file.Close(); err != nil && j.logger != nil {
        j.logger.Printf("Error session journal close: %v", err)
    }
}

// Close stops new records, drains and flushes the queue, closes the file and
// waits for the background gzip to land. It is idempotent.
func (j *Journal) Close() {
    if j == nil || !j.closed.CompareAndSwap(false, true) {
        return
    }
    close(j.done)
    j.wg.Wait()
}

// Build writes the process identity line (the first record of the file).
func (j *Journal) Build(identity string) {
    r := newRecord("", kindBuild, time.Now())
    r.V = identity
    j.send(r)
}

// Story mirrors one tracker event line into the journal at the given
// time (the sink of state.Bot, which owns the exact event timestamp).
func (j *Journal) Story(bot string, message string, at time.Time) {
    r := newRecord(bot, kindStory, at)
    r.M = message
    j.send(r)
}

// Sample records one periodic character state read.
func (j *Journal) Sample(bot string, s Sample) {
    r := newRecord(bot, kindSample, time.Now())
    r.Lv = s.Level
    r.Xp = s.Exp
    r.Ad = s.Adena
    r.Hp = s.Health
    r.X = s.X
    r.Y = s.Y
    r.Ph = s.Phase
    j.send(r)
}

// Level records a level up of the character.
func (j *Journal) Level(bot string, level int32, exp int64) {
    r := newRecord(bot, kindLevel, time.Now())
    r.Lv = level
    r.Xp = exp
    j.send(r)
}

// Kill records one killed mob with its fight duration, the end health percent
// and the position, so the drill-down locates the bad fights without the
// samples around every kill.
func (j *Journal) Kill(
    bot string, mob string, level int32, fightSec float64, health float64,
    x int32, y int32,
) {
    r := newRecord(bot, kindKill, time.Now())
    r.Mob = mob
    r.Lvl = level
    r.Dur = fightSec
    r.Hp = health
    r.X = x
    r.Y = y
    j.send(r)
}

// Death records one character death at the given level and position.
func (j *Journal) Death(bot string, level int32, x int32, y int32) {
    r := newRecord(bot, kindDeath, time.Now())
    r.Lv = level
    r.X = x
    r.Y = y
    j.send(r)
}

// TripStart brackets the beginning of a town trip with its reason.
func (j *Journal) TripStart(bot string, reason string) {
    r := newRecord(bot, kindTripStart, time.Now())
    r.R = reason
    j.send(r)
}

// TripEnd closes a town trip bracket with its reason and duration.
func (j *Journal) TripEnd(bot string, reason string, dur time.Duration) {
    r := newRecord(bot, kindTripEnd, time.Now())
    r.R = reason
    r.Dur = dur.Seconds()
    j.send(r)
}

// Buy records one buy request batch with its item list and adena cost.
func (j *Journal) Buy(bot string, items string, count int, cost int64) {
    r := newRecord(bot, kindBuy, time.Now())
    r.Items = items
    r.N = int32(count)
    r.Cost = cost
    j.send(r)
}

// Sell records one sell batch with its item count.
func (j *Journal) Sell(bot string, count int) {
    r := newRecord(bot, kindSell, time.Now())
    r.N = int32(count)
    j.send(r)
}

// Zone records a hunting zone switch with the reason.
func (j *Journal) Zone(bot string, name string, reason string) {
    r := newRecord(bot, kindZone, time.Now())
    r.Mob = name
    r.R = reason
    j.send(r)
}

// Stall records a stagnation watch event with the hunt phase it fired in; the
// phase separates the stalled delevel walks from the stalled farms.
func (j *Journal) Stall(
    bot string, kind string, held time.Duration, x int32, y int32,
    phase string,
) {
    r := newRecord(bot, kindStall, time.Now())
    r.R = kind
    r.Dur = held.Seconds()
    r.X = x
    r.Y = y
    r.Ph = phase
    j.send(r)
}

// Repath records one stuck-and-replanned walk segment.
func (j *Journal) Repath(bot string, attempt int) {
    r := newRecord(bot, kindRepath, time.Now())
    r.N = int32(attempt)
    j.send(r)
}

// Connect records a session lifecycle mark of the supervisor.
func (j *Journal) Connect(bot string, stage string, detail string) {
    r := newRecord(bot, kindConnect, time.Now())
    r.R = stage
    r.M = detail
    j.send(r)
}

// Lost records a lost session of the supervisor with the reason.
func (j *Journal) Lost(bot string, reason string) {
    r := newRecord(bot, kindLost, time.Now())
    r.R = reason
    j.send(r)
}

// Logout records an intentional emergency logout with its reason and login
// cooldown: the socket close the supervisor books as a lost connection gets its
// honest cause, so the analysis separates the pile-up escapes from the real
// network drops.
func (j *Journal) Logout(bot string, reason string, cooldown time.Duration) {
    r := newRecord(bot, kindLogout, time.Now())
    r.R = reason
    r.Ti = cooldown.Seconds()
    j.send(r)
}

// Shutdown records the graceful process shutdown.
func (j *Journal) Shutdown(reason string) {
    r := newRecord("", kindShutdown, time.Now())
    r.R = reason
    j.send(r)
}

// Report renders the compact agent-facing report of one bot from the
// live aggregate plus the optional live view. It returns an error when
// the journal never saw the bot.
func (j *Journal) Report(bot string, live *LiveView) (string, error) {
    if j == nil {
        return "", errNoJournal
    }
    j.mu.RLock()
    defer j.mu.RUnlock()
    agg, ok := j.aggs[bot]
    if !ok {
        return "", errUnknownBot
    }

    return renderReport(j.path, agg, live), nil
}

// itoa renders a non-negative int64 in decimal without the fmt
// machinery (the perfsprint rule of the repository).
func itoa(v int64) string {
    if v == 0 {
        return "0"
    }
    var buf [20]byte
    pos := len(buf)
    for v > 0 {
        pos--
        buf[pos] = byte('0' + v%10)
        v /= 10
    }

    return string(buf[pos:])
}

// countingWriter tracks the written bytes behind the rotation decision. The
// counter is read after the bufio flush on the same goroutine, so the
// unsynchronized int64 is safe.
type countingWriter struct {
    inner   io.Writer
    written int64
}

// Write implements io.Writer.
func (c *countingWriter) Write(p []byte) (int, error) {
    n, err := c.inner.Write(p)
    c.written += int64(n)

    return n, err //nolint:wrapcheck // passthrough writer
}

// gzipFile compresses a rotated segment to path.gz and removes the original
// on success. A failure keeps the original (the data survives, the compression
// is lost) and drops the partial archive, so a truncated .gz never shadows the
// intact one. Both handles close before the remove: Windows refuses to unlink
// a file another handle holds.
func gzipFile(path string, wg *sync.WaitGroup, logger *log.Logger) {
    defer wg.Done()
    source, err := os.Open(path)
    if err != nil {
        gzipLog(logger, "open", err)

        return
    }
    target, err := os.Create(path + ".gz")
    if err != nil {
        gzipLog(logger, "create", err)
        closeGzipHandle(logger, "close source", source)

        return
    }
    if err := compressSegment(target, source); err != nil {
        gzipLog(logger, "compress", err)
        closeGzipHandle(logger, "close source", source)
        if rmErr := os.Remove(path + ".gz"); rmErr != nil {
            gzipLog(logger, "cleanup", rmErr)
        }

        return
    }
    closeGzipHandle(logger, "close source", source)
    if err := os.Remove(path); err != nil {
        gzipLog(logger, "cleanup", err)
    }
}

// compressSegment copies the source through the gzip sink into the target: a
// failure anywhere means the archive did not land, and every path closes both
// handles, so a failed archive never pins the partial file open.
func compressSegment(target *os.File, source *os.File) error {
    sink := gzip.NewWriter(target)
    _, err := gzipCopy(sink, source)
    closeErr := sink.Close()
    if err == nil {
        err = closeErr
    }
    if targetErr := target.Close(); err == nil {
        err = targetErr
    }
    if err != nil {
        return err
    }

    return nil
}

// closeGzipHandle closes one handle and reports the failure; a handle that only
// served a failed stage never masks the stage error.
func closeGzipHandle(logger *log.Logger, stage string, handle *os.File) {
    gzipLog(logger, stage, handle.Close())
}

// gzipLog reports one gzip stage failure when the journal carries a logger.
func gzipLog(logger *log.Logger, stage string, err error) {
    if logger == nil || err == nil {
        return
    }
    logger.Printf("Error session journal gzip %s: %v", stage, err)
}

// gzipCopy is the copy step of compressSegment, a seam the tests
// swap to force a copy failure.
var gzipCopy = io.Copy
