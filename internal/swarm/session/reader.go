// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package session

import (
    "bufio"
    "compress/gzip"
    "encoding/json"
    "fmt"
    "io"
    "os"
    "time"
)

// JournalFile is the parsed view of one journal file: the per-bot
// aggregates and the provenance of the file. The -session-report CLI
// and any post-mortem investigation of a finished or crashed run build
// it from disk (the live journal serves the web UI endpoint from
// memory).
type JournalFile struct {
    Path   string
    Build  string
    ShutAt string
    Aggs   map[string]*botAgg
    Order  []string
}

// ParseJournalFile reads a journal file (plain or gzipped - the rotated
// segments land as .gz) and returns the aggregates of every bot it
// saw, in first-seen order.
func ParseJournalFile(path string) (*JournalFile, error) {
    parsed, err := parseJournalWindow(path, "", "")
    if err != nil {
        return nil, err
    }

    return parsed, nil
}

// ParseJournalFileWindow reads a journal file and folds only the
// records inside the [from, to] window: the time-boxed report of one
// interesting stretch of a long session. The bounds stay raw strings on
// purpose - a bare 15:04 or 15:04:05 clock resolves against the date of
// the first record (a session crossing midnight binds 01:30 to the
// morning after the 22:51 start), a full RFC3339 timestamp passes
// through. Empty bounds keep the window open on that side.
func ParseJournalFileWindow(
    path string, from string, to string,
) (*JournalFile, error) {
    parsed, err := parseJournalWindow(path, from, to)
    if err != nil {
        return nil, err
    }

    return parsed, nil
}

// scanJournal streams every parsable record of a journal file (plain or
// gzipped) into the visit callback and reports the read errors. A torn
// final line of a crashed run is skipped: the records before it still
// parse. The tooling of the package (the report parser, the query and
// the anomaly scan) shares this one reader.
func scanJournal(path string, visit func(record) error) error {
    file, err := os.Open(path)
    if err != nil {
        return fmt.Errorf("open journal: %w", err)
    }
    defer file.Close()

    var reader io.Reader = bufio.NewReaderSize(file, 1<<16)
    if hasGzipExt(path) {
        gz, err := gzip.NewReader(reader)
        if err != nil {
            return fmt.Errorf("open journal gzip: %w", err)
        }
        defer gz.Close()
        reader = gz
    }

    scanner := bufio.NewScanner(reader)
    scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
    for scanner.Scan() {
        line := scanner.Bytes()
        if len(line) == 0 {
            continue
        }
        var r record
        if err := json.Unmarshal(line, &r); err != nil {
            // A torn final line of a crashed run is expected: the
            // records before it still parse.
            continue
        }
        if err := visit(r); err != nil {
            return err
        }
    }
    if err := scanner.Err(); err != nil {
        return fmt.Errorf("read journal: %w", err)
    }

    return nil
}

// hasGzipExt reports whether the file name carries the gzip suffix.
func hasGzipExt(path string) bool {
    return len(path) > 3 && path[len(path)-3:] == ".gz"
}

// parseJournalWindow folds every record of the stream inside the window
// into fresh aggregates. The parser shares the apply path with the live
// journal, so the offline report and the live report agree by
// construction. The bare clock bounds resolve against the first record
// date exactly like the query filters.
func parseJournalWindow(
    path string, fromArg string, toArg string,
) (*JournalFile, error) {
    parsed := &JournalFile{
        Path:   path,
        Build:  "",
        ShutAt: "",
        Aggs:   make(map[string]*botAgg),
        Order:  nil,
    }
    from, to := time.Time{}, time.Time{}
    anchored := false
    err := scanJournal(path, func(r record) error {
        if !anchored {
            anchored = true
            base := time.Unix(r.T, 0).UTC()
            resolved, err := resolveWindow(fromArg, toArg, base)
            if err != nil {
                return err
            }
            from, to = resolved[0], resolved[1]
        }
        if r.E == kindBuild || r.E == kindShutdown ||
            insideWindow(r.T, from, to) {
            parsed.fold(r)
        }

        return nil
    })
    if err != nil {
        return nil, err
    }

    return parsed, nil
}

// resolveWindow parses both bounds of a window against the anchor date.
func resolveWindow(
    fromArg string, toArg string, base time.Time,
) ([2]time.Time, error) {
    var window [2]time.Time
    if fromArg != "" {
        from, err := ParseQueryTime(fromArg, base)
        if err != nil {
            return window, fmt.Errorf("-from: %w", err)
        }
        window[0] = from
    }
    if toArg != "" {
        to, err := ParseQueryTime(toArg, base)
        if err != nil {
            return window, fmt.Errorf("-to: %w", err)
        }
        window[1] = to
    }

    return window, nil
}

// insideWindow reports whether a unix second falls into the bounds
// (zero bounds stay open).
func insideWindow(t int64, from time.Time, to time.Time) bool {
    if !from.IsZero() && t < from.Unix() {
        return false
    }
    if !to.IsZero() && t > to.Unix() {
        return false
    }

    return true
}

// fold applies one record to the parsed aggregates.
func (j *JournalFile) fold(r record) {
    switch r.E {
    case kindBuild:
        j.Build = r.V

        return
    case kindShutdown:
        j.ShutAt = r.R

        return
    }
    agg, ok := j.Aggs[r.B]
    if !ok {
        agg = newBotAgg(r.B)
        j.Aggs[r.B] = agg
        j.Order = append(j.Order, r.B)
    }
    agg.accept(r)
    agg.apply(r)
}

// Report renders the report of one bot of the parsed file (the live
// view is absent: the offline report reads the journal only).
func (j *JournalFile) Report(bot string) (string, error) {
    agg, ok := j.Aggs[bot]
    if !ok {
        return "", errUnknownBot
    }
    // The offline report carries no live block: the run is over, the
    // journal is the only truth.
    return renderReport(j.Path, agg, nil), nil
}
