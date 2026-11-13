// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Package botlog writes the per-run log file of one acceptance scenario into
// logs/acceptance: the test contract, the environment, every received and sent
// packet, the scenario narration, the check transitions, the periodic state
// samples and the verdict, so one attached file carries the whole run. The
// writer goroutine keeps the session reader off the disk (a full queue drops
// lines and counts them). The file layout: docs/acceptance_botlog.md.
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

// Writer tuning: the flush period bounds what a crashed run loses, the queue
// depth rides out a disk burst and the drop path keeps a flooded packet stream
// from blocking the session reader (the dropped count rides the verdict).
const (
    flushPeriod     = time.Second
    queueDepth      = 8192
    writerBufferLen = 1 << 16
)

// Header carries the identity block at the top of the file: the web UI test
// card plus the environment the run is wired into.
type Header struct {
    // TestID, Title, Account and Generation identify the run; the generation is
    // the manager launch counter (a restart bumps it).
    TestID     string
    Title      string
    Account    string
    Generation uint64
    // Timeout bounds the whole run (the scenario bound of the web UI card).
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

// entry is one queued record: the capture time, the offset from the run start
// and the rendered lines (the taps render on their own goroutines).
type entry struct {
    at    time.Time
    after float64
    tag   string
    lines []string
}

// RunLog is the append-only log file of one acceptance run: a single writer
// goroutine owns the file and the callers hand entries over a buffered channel
// without blocking (a full queue drops the line and counts it).
//
// A nil *RunLog is valid: every method is a no-op, so the runner calls it
// unconditionally.
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

// Open creates the log directory, opens the run file (named after the test, the
// start time, the pid and a sequence number, so a listing sorts
// chronologically) and starts the writer goroutine with the header block.
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

// writeLoop owns the file: it drains the queue, flushes on the period and
// writes the header first, so a run killed at once still leaves the contract.
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

// writeEntry renders one record: the wall clock, the offset, the tag and the
// lines (the first rides the tag row, the rest indent under it).
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

// emit renders and queues one record without blocking: a full queue drops and
// counts it, a closed log drops silently.
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
