// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package session

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// writeTestJournal writes a journal file with a known event set and
// returns its path.
func writeTestJournal(t *testing.T, path string) {
	t.Helper()
	journal, err := NewJournal(filepath.Dir(path), nil)
	require.NoError(t, err)
	// NewJournal names its own file; write the records, then move the
	// file to the requested path.
	journal.Build("main:abc123 dirty=false")
	journal.Story("test1", "Hunt: the recorded line",
		time.Unix(1700000000, 0))
	r := newRecord("test1", kindSample, time.Unix(1700000000, 0))
	r.Lv, r.Xp, r.Ad, r.Hp = 5, 1000, 500, 90
	r.X, r.Y, r.Ph = 10, 20, "engage"
	journal.Sample("test1", Sample{
		Level: 5, Exp: 1000, Adena: 500, Health: 90, X: 10, Y: 20,
		Phase: "engage",
	})
	_ = r
	journal.Kill("test1", "Kaboo Orc", 4, 8.2, 87.5)
	journal.Close()
	require.NoError(t, os.Rename(journal.Path(), path))
}

// TestParseJournalFile verifies the offline reader rebuilds the same
// aggregates the live journal holds.
func TestParseJournalFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session-test.jsonl")
	writeTestJournal(t, path)
	parsed, err := ParseJournalFile(path)
	require.NoError(t, err)
	require.Equal(t, "main:abc123 dirty=false", parsed.Build)
	require.Equal(t, []string{"test1"}, parsed.Order)
	agg := parsed.Aggs["test1"]
	require.Equal(t, 1, agg.kills)
	require.Equal(t, "Kaboo Orc", agg.slowFight[0].mob)
	require.NotNil(t, agg.prev)
	require.Equal(t, int64(500), agg.prev.ad)
	report, err := parsed.Report("test1")
	require.NoError(t, err)
	require.Contains(t, report, "swarm session report")
	require.Contains(t, report, "journal: "+path)
	require.Contains(t, report, "kills: 1")
	require.Contains(t, report, "Hunt: the recorded line")
}

// TestParseJournalGzip verifies the rotated segment (gzipped) parses
// the same way.
func TestParseJournalGzip(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "session-test.jsonl")
	writeTestJournal(t, source)
	plain, err := os.Open(source)
	require.NoError(t, err)
	target, err := os.Create(source + ".gz")
	require.NoError(t, err)
	sink := gzip.NewWriter(target)
	_, err = io.Copy(sink, plain)
	require.NoError(t, err)
	require.NoError(t, sink.Close())
	require.NoError(t, plain.Close())
	require.NoError(t, target.Close())
	parsed, err := ParseJournalFile(source + ".gz")
	require.NoError(t, err)
	agg := parsed.Aggs["test1"]
	require.Equal(t, 1, agg.kills)
}

// TestParseJournalTornLine verifies a crashed run still parses: the
// torn final line is skipped, the records before it survive.
func TestParseJournalTornLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session-torn.jsonl")
	body := "{\"t\":1700000000,\"b\":\"test1\",\"e\":\"story\"," +
		"\"m\":\"the complete line\"}\n" +
		"{\"t\":1700000001,\"b\":\"test1\",\"e\":\"ki"
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	parsed, err := ParseJournalFile(path)
	require.NoError(t, err)
	agg := parsed.Aggs["test1"]
	require.Len(t, agg.ring, 1)
	require.Equal(t, "the complete line", agg.ring[0].msg)
}

// TestParseJournalMissingFile verifies the open error wraps.
func TestParseJournalMissingFile(t *testing.T) {
	_, err := ParseJournalFile(filepath.Join(t.TempDir(), "nope.jsonl"))
	require.ErrorContains(t, err, "open journal")
}

// TestReportOfflineSections verifies the offline report renders every
// section from a parsed journal.
func TestReportOfflineSections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session-sections.jsonl")
	writeTestJournal(t, path)
	parsed, err := ParseJournalFile(path)
	require.NoError(t, err)
	report, err := parsed.Report("test1")
	require.NoError(t, err)
	for _, section := range []string{
		"== character journey ==",
		"== economy ==",
		"== kills & combat ==",
		"== deaths ==",
		"== downtime ==",
		"== stalls & stucks ==",
		"== session story (newest first) ==",
	} {
		require.Contains(t, report, section)
	}
}
