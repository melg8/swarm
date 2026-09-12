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

// Journal tuning. The flush period bounds how much of a crashed run is
// lost (2 s of events at the observed rates is a couple of lines); the
// channel depth rides out a slow disk burst; the rotation size keeps a
// runaway loop from filling the user disk (the rotated file gzips to
// roughly a tenth - JSONL with repeated keys compresses well); the
// story cap mutes an event flood of a misbehaving loop (a healthy bot
// writes well under thirty lines a minute, the cap sits at four times
// that so the flood itself stays visible as a drop counter).
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

// Journal is the append-only session journal of one swarm process:
// every bot of the process writes into one JSONL file, each record
// carrying its bot id. A single writer goroutine owns the file (the
// callers hand records over a channel and never block - the hunt tick
// and the tracker locks must not wait on disk), the aggregator rides
// the same goroutine so the report always sees the flushed truth, and
// the file rotates at rotateBytes with the rotated copy gzipped in the
// background.
//
// A nil *Journal is valid: every method is a no-op on it, so the hunt
// loop, the supervisor and the web handlers call it unconditionally
// (the -session-dir "" mode turns the journal off everywhere at once).
type Journal struct {
	dir     string
	path    string
	logger  *log.Logger
	queue   chan record
	closed  atomic.Bool
	done    chan struct{}
	wg      sync.WaitGroup
	mu      sync.RWMutex
	aggs    map[string]*botAgg
	file    *os.File
	writer  *bufio.Writer
	counter *countingWriter
	enc     *json.Encoder
	seq     int
	written int64
	dropped atomic.Uint64
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
		dir:     dir,
		path:    path,
		logger:  logger,
		queue:   make(chan record, eventQueueDepth),
		done:    make(chan struct{}),
		wg:      sync.WaitGroup{},
		mu:      sync.RWMutex{},
		aggs:    make(map[string]*botAgg),
		file:    nil,
		writer:  nil,
		counter: &countingWriter{inner: nil, written: 0},
		enc:     nil,
		seq:     0,
		written: 0,
		dropped: atomic.Uint64{},
	}
	j.install(file)
	j.wg.Add(1)
	go j.flusher()

	return j, nil
}

// createJournalFile opens a fresh journal file. The name carries the
// start timestamp and the process id; two journals of the same second
// (a test, a quick restart) disambiguate through an attempt counter,
// the exclusive create refuses to append to an existing file.
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

// Dropped returns the count of records the queue refused (a storm the
// writer could not keep up with); a persistent non-zero growth means
// the disk stalls the writer.
func (j *Journal) Dropped() uint64 {
	if j == nil {
		return 0
	}

	return j.dropped.Load()
}

// send hands one record to the writer. It never blocks: a full queue
// drops the record and counts it (the journal is diagnostics, never a
// load-bearing path of the bot).
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

// flusher is the single writer goroutine: it drains the queue into
// the buffered file, flushes on the period ticker and exits when the
// journal closes and the queue drains.
func (j *Journal) flusher() {
	defer j.wg.Done()
	ticker := time.NewTicker(flushPeriod)
	defer ticker.Stop()
	for {
		select {
		case r := <-j.queue:
			j.write(r)
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

// write encodes one record into the file and folds it into the
// aggregate of its bot. The story cap keeps a flooded event log from
// eating the disk; the muted count rides the aggregate so the report
// names it. The aggregate state is only touched under the write lock
// (the report readers hold the read lock).
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

// rotate closes the current file, gzips it in the background and opens
// the next segment. The segments keep sequence numbers, so they order
// lexically after the base file.
func (j *Journal) rotate() {
	if err := j.file.Close(); err != nil && j.logger != nil {
		j.logger.Printf("Error session journal close: %v", err)
	}
	rotated := j.path
	if j.seq > 0 {
		rotated = j.segmentPath(j.seq)
	}
	j.wg.Add(1)
	go gzipFile(rotated, &j.wg, j.logger)
	j.seq++
	next, err := os.OpenFile(
		j.segmentPath(j.seq), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		if j.logger != nil {
			j.logger.Printf("Error session journal rotate: %v", err)
		}

		return
	}
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

// Close stops accepting new records, drains and flushes the queue,
// closes the file and waits for the background gzip to land. It is
// idempotent.
func (j *Journal) Close() {
	if j == nil || !j.closed.CompareAndSwap(false, true) {
		return
	}
	close(j.done)
	j.wg.Wait()
}

// Build writes the process identity line (the first record of the
// file).
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

// Kill records one killed mob: the fight duration in seconds and the
// health percent the character ended the fight with.
func (j *Journal) Kill(
	bot string, mob string, level int32, fightSec float64, health float64,
) {
	r := newRecord(bot, kindKill, time.Now())
	r.Mob = mob
	r.Lvl = level
	r.Dur = fightSec
	r.Hp = health
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

// Stall records a stagnation watch event (an xp or position hold).
func (j *Journal) Stall(
	bot string, kind string, held time.Duration, x int32, y int32,
) {
	r := newRecord(bot, kindStall, time.Now())
	r.R = kind
	r.Dur = held.Seconds()
	r.X = x
	r.Y = y
	j.send(r)
}

// Repath records one stuck-and-replanned walk leg.
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

// countingWriter wraps the journal file to track the written bytes of
// the rotation decision (the counter is read after the bufio flush,
// so the unsynchronized int64 is always observed after a flush that
// happens-before it on the same goroutine).
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

// gzipFile compresses a rotated journal segment to path.gz and removes
// the original on success. A gzip failure keeps the original on disk
// (the data survives, only the compression is lost).
func gzipFile(path string, wg *sync.WaitGroup, logger *log.Logger) {
	defer wg.Done()
	source, err := os.Open(path)
	if err != nil {
		if logger != nil {
			logger.Printf("Error session journal gzip open: %v", err)
		}

		return
	}
	defer source.Close()
	target, err := os.Create(path + ".gz")
	if err != nil {
		if logger != nil {
			logger.Printf("Error session journal gzip create: %v", err)
		}

		return
	}
	sink := gzip.NewWriter(target)
	if _, err := io.Copy(sink, source); err != nil && logger != nil {
		logger.Printf("Error session journal gzip copy: %v", err)
	}
	if err := sink.Close(); err != nil && logger != nil {
		logger.Printf("Error session journal gzip close: %v", err)
	}
	if err := target.Close(); err != nil && logger != nil {
		logger.Printf("Error session journal gzip target: %v", err)
	}
	if err := os.Remove(path); err != nil && logger != nil {
		logger.Printf("Error session journal gzip cleanup: %v", err)
	}
}
