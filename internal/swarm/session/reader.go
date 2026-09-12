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
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open journal: %w", err)
	}
	defer file.Close()

	var reader io.Reader = bufio.NewReaderSize(file, 1<<16)
	if hasGzipExt(path) {
		gz, err := gzip.NewReader(reader)
		if err != nil {
			return nil, fmt.Errorf("open journal gzip: %w", err)
		}
		defer gz.Close()
		reader = gz
	}

	return parseJournal(path, reader)
}

// hasGzipExt reports whether the file name carries the gzip suffix.
func hasGzipExt(path string) bool {
	return len(path) > 3 && path[len(path)-3:] == ".gz"
}

// parseJournal folds every record of the stream into fresh
// aggregates. The parser shares the apply path with the live journal,
// so the offline report and the live report agree by construction.
func parseJournal(path string, reader io.Reader) (*JournalFile, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	parsed := &JournalFile{
		Path:   path,
		Build:  "",
		ShutAt: "",
		Aggs:   make(map[string]*botAgg),
		Order:  nil,
	}
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
		parsed.fold(r)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read journal: %w", err)
	}

	return parsed, nil
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
