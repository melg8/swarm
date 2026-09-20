// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Package botlog writes the hyper-detailed per-run log files of the
// acceptance scenarios into logs/acceptance: one file per run, the
// full story of the temp bot from the server wiring to the verdict.
//
// The problem it solves: the acceptance scenarios run on the live
// Mobius stack for minutes, and when a run hangs, drifts or passes
// without the bot really doing the thing, the web UI test log (a 40
// line ring) and the point-in-time state dump cannot answer what
// happened at the moment things went wrong. The run log keeps the
// complete trail on disk: the test contract (the description and the
// pass/fail checks exactly like the web UI card), the environment
// (the login, the proxy, the database, the build), every game packet
// the session received and sent (decoded into one readable line
// each), the scenario narration, the monitor check transitions (the
// moments the tester decided an event happened), the periodic state
// samples of the character and the final verdict block.
//
// The intended use: the owner attaches the one file of a suspicious
// run to the next agent prompt ("it does not work" or "the test
// passed but the bot never did X") and the file alone carries every
// fact needed to diagnose the run without reproducing it.
package botlog

import (
    "bufio"
    "fmt"
    "os"
    "path/filepath"
    "runtime"
    "strconv"
    "strings"
    "sync"
    "sync/atomic"
    "time"
)

// Writer tuning. The flush period bounds how much of a crashed run
// is lost; the queue depth rides out a slow disk burst; the drop
// path keeps a flooded packet stream from blocking the session
// reader (the dropped count rides the verdict so the gap is visible
// instead of silent).
const (
    flushPeriod     = time.Second
    queueDepth      = 8192
    writerBufferLen = 1 << 16
)

// Header carries the identity block written at the top of the file:
// the test card of the web UI plus the environment the run is wired
// into. The acceptance manager builds it when the run starts.
type Header struct {
    // TestID, Title, Account and Generation identify the run: the
    // generation is the manager launch counter of the test (a
    // restart of a running test bumps it).
    TestID     string
    Title      string
    Account    string
    Generation uint64
    // Timeout bounds the whole run (the scenario bound of the web
    // UI card).
    Timeout time.Duration
    // Description is the hover text of the web UI card: the Start,
    // Flow and Pass story of the scenario.
    Description string
    // Checks is the pass/fail contract: the list the monitor
    // rewrites from the live tracker state.
    Checks []CheckDesc
    // Environment carries the wiring lines of the run (the login
    // endpoint, the proxy, the database, the navigator, the build).
    Environment []EnvLine
    // Args is the command line of the process.
    Args string
}

// CheckDesc is one condition of the pass/fail contract: the short id
// the monitor rewrites and the label the web UI list shows.
type CheckDesc struct {
    ID     string
    Label  string
    Done   bool
    Detail string
}

// EnvLine is one "key: value" line of the environment block.
type EnvLine struct {
    Key   string
    Value string
}

// entry is one queued record of the writer goroutine: the capture
// time, the offset from the run start and the fully rendered lines
// (the taps render on their own goroutines so the writer only adds
// the time prefix).
type entry struct {
    at    time.Time
    after float64
    tag   string
    lines []string
}

// RunLog is the append-only log file of one acceptance run. A single
// writer goroutine owns the file; the callers hand entries over a
// buffered channel and never block (a full queue drops the line and
// counts it - the session reader goroutine must not wait on disk).
//
// A nil *RunLog is valid: every method is a no-op on it, so the
// runner and the manager call it unconditionally (the disabled log
// directory turns the whole subsystem off at once).
type RunLog struct {
    path    string
    started time.Time
    queue   chan entry
    done    chan struct{}
    wg      sync.WaitGroup
    closed  atomic.Bool
    dropped atomic.Uint64
    recvN   atomic.Uint64
    sentN   atomic.Uint64
}

// fileSeq disambiguates two runs of the same test that start within
// the same second on the same process.
var fileSeq atomic.Uint64

// Open creates the log directory when missing, opens the run file
// (named after the test, the start time, the pid and a sequence
// number) and starts the writer goroutine with the header block.
// The file name stays flat so a directory listing sorts the runs
// chronologically.
func Open(dir string, head Header) (*RunLog, error) {
    if err := os.MkdirAll(dir, 0o755); err != nil {
        return nil, fmt.Errorf("botlog dir: %w", err)
    }
    seq := fileSeq.Add(1)
    started := time.Now()
    stamp := started.Format("20060102-150405")
    name := fmt.Sprintf("%s-%s-%d-%d.log",
        sanitizeName(head.TestID), stamp, os.Getpid(), seq)
    path := filepath.Join(dir, name)
    file, err := os.Create(path)
    if err != nil {
        return nil, fmt.Errorf("botlog file: %w", err)
    }
    log := &RunLog{
        path:    path,
        started: started,
        queue:   make(chan entry, queueDepth),
        done:    make(chan struct{}),
        wg:      sync.WaitGroup{},
        closed:  atomic.Bool{},
        dropped: atomic.Uint64{},
        recvN:   atomic.Uint64{},
        sentN:   atomic.Uint64{},
    }
    writer := bufio.NewWriterSize(file, writerBufferLen)
    log.wg.Add(1)
    go log.writeLoop(file, writer, head)

    return log, nil
}

// Path returns the log file path (empty on a nil log).
func (l *RunLog) Path() string {
    if l == nil {
        return ""
    }

    return l.path
}

// sanitizeName keeps the file name free of path separators and
// spaces (the test ids are slugs already, the guard is cheap).
func sanitizeName(value string) string {
    var out strings.Builder
    out.Grow(len(value))
    for i := range len(value) {
        ch := value[i]
        switch {
        case ch >= 'a' && ch <= 'z', ch >= 'A' && ch <= 'Z',
            ch >= '0' && ch <= '9', ch == '-', ch == '_', ch == '.':
            out.WriteByte(ch)
        default:
            out.WriteByte('-')
        }
    }

    return out.String()
}

// writeLoop owns the file: it drains the queue, flushes on the
// period and writes the header before the first entry so a run
// killed at once still leaves the contract behind.
func (l *RunLog) writeLoop(
    file *os.File, writer *bufio.Writer, head Header,
) {
    defer l.wg.Done()
    defer func() {
        _ = file.Close()
    }()
    writeHeader(writer, l.started, head)
    ticker := time.NewTicker(flushPeriod)
    defer ticker.Stop()
    for {
        select {
        case e := <-l.queue:
            writeEntry(writer, e)
        case <-ticker.C:
            _ = writer.Flush()
        case <-l.done:
            l.drain(writer)

            return
        }
    }
}

// drain empties the queue after the close signal and flushes the
// last lines to disk.
func (l *RunLog) drain(writer *bufio.Writer) {
    for {
        select {
        case e := <-l.queue:
            writeEntry(writer, e)
        default:
            _ = writer.Flush()

            return
        }
    }
}

// writeHeader renders the identity, contract and environment blocks
// of the file top.
func writeHeader(
    writer *bufio.Writer, started time.Time, head Header,
) {
    writeln(writer, separator)
    writeln(writer, " swarm acceptance run log")
    writeln(writer, separator)
    writeln(writer, " test      : "+head.TestID+" - "+head.Title)
    writeln(writer, " account   : "+head.Account)
    writeln(writer, " run       : generation "+
        strconv.FormatUint(head.Generation, 10))
    writeln(writer, " started   : "+
        started.Format("2006-01-02 15:04:05.000")+
        " (unix "+strconv.FormatInt(started.Unix(), 10)+")")
    writeln(writer, " timeout   : "+head.Timeout.String())
    writeln(writer, " go        : "+runtime.Version())
    if head.Args != "" {
        writeln(writer, " args      : "+head.Args)
    }
    writeln(writer, "")
    writeln(writer, "--- test description (the web ui card) ---")
    writeln(writer, head.Description)
    writeln(writer, "")
    writeln(writer, "--- pass / fail contract ---")
    writeln(writer, "the scenario publishes its check list at the run"+
        " start (the")
    writeln(writer, "first [check] block below): the monitor rewrites"+
        " those checks")
    writeln(writer, "from the live tracker state, the run passes when"+
        " the scenario")
    writeln(writer, "returns nil (every check done) and fails on the"+
        " scenario")
    writeln(writer, "error, the timeout ("+head.Timeout.String()+
        ") or a cancel.")
    writeln(writer, "")
    writeln(writer, "--- environment ---")
    for i := range head.Environment {
        writeln(writer, "  "+head.Environment[i].Key+": "+
            head.Environment[i].Value)
    }
    writeln(writer, "")
    writeln(writer, "--- event log ---")
    writeln(writer, "format: [wall clock +offset] [tag] message")
    writeln(writer, "tags: scenario = the test narration, "+
        "hunt = the loop decisions,")
    writeln(writer, "      db = the character injection sql, "+
        "session = the connection story,")
    writeln(writer, "      recv/send = the decoded game packets, "+
        "check = the monitor verdicts,")
    writeln(writer, "      sample = the periodic character state, "+
        "verdict = the outcome")
    _ = writer.Flush()
}

// separator is the horizontal rule of the header block.
const separator = "=================================================="

// checkMark renders the checkbox of one contract line.
func checkMark(done bool) string {
    if done {
        return "[x]"
    }

    return "[ ]"
}

// checkIDWidth aligns the check ids of the contract block.
const checkIDWidth = 12

// padRight pads a string with spaces to the width.
func padRight(value string, width int) string {
    for len(value) < width {
        value += " "
    }

    return value
}

// writeln writes one line and its newline.
func writeln(writer *bufio.Writer, line string) {
    _, _ = writer.WriteString(line)
    _ = writer.WriteByte('\n')
}

// writeEntry renders one queued record: the wall clock time, the
// offset from the run start, the tag and the lines (the first line
// rides the tag row, the continuation lines indent under it).
func writeEntry(writer *bufio.Writer, e entry) {
    prefix := fmt.Sprintf("[%s +%.3fs] [%s] ",
        e.at.Format("15:04:05.000"), e.after, e.tag)
    for i, line := range e.lines {
        if i == 0 {
            writeln(writer, prefix+line)

            continue
        }
        writeln(writer, "        "+line)
    }
}

// emit renders and queues one record. The queue handoff never
// blocks: a full queue drops the record and counts it, a closed log
// drops silently (the verdict is already written).
func (l *RunLog) emit(tag string, lines []string) {
    if l == nil || l.closed.Load() {
        return
    }
    at := time.Now()
    e := entry{
        at:    at,
        after: at.Sub(l.started).Seconds(),
        tag:   tag,
        lines: lines,
    }
    select {
    case l.queue <- e:
    default:
        l.dropped.Add(1)
    }
}
