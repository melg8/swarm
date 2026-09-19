// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "os"
    "strings"
    "testing"

    "github.com/stretchr/testify/require"
)

// TestPrintRowsRendersMetricBlocks pins the table rendering: one
// block per metric unit in the pinned column order, the signed delta
// with two decimals.
func TestPrintRowsRendersMetricBlocks(t *testing.T) {
    base, err := parseBenchFile(
        writeTemp(t, "base.txt", baseOutput))
    require.NoError(t, err)
    fresh, err := parseBenchFile(
        writeTemp(t, "fresh.txt", freshOutput))
    require.NoError(t, err)
    rows := diff(base, fresh)
    require.NotEmpty(t, rows)

    var out strings.Builder
    printRows(&out, rows)
    rendered := out.String()

    // The header block comes first for ns/op, the column order
    // follows metricOrder, every row carries the signed delta.
    require.Contains(t, rendered, "benchmark (ns/op)")
    require.Contains(t, rendered, "benchmark (B/op)")
    require.Contains(t, rendered, "benchmark (allocs/op)")
    require.Contains(t, rendered, "base")
    require.Contains(t, rendered, "delta")
    // The PlanQuery regression renders as the signed percentage.
    require.Contains(t, rendered, "+25.00%")
    // The flat metric renders with the +0.00% delta.
    require.Contains(t, rendered, "+0.00%")
}

// TestPrintRowsSkipsEmptyUnits pins the no-block-no-header rule: a
// unit without rows never prints a header.
func TestPrintRowsSkipsEmptyUnits(t *testing.T) {
    rows := []row{{
        name: "BenchmarkPlanQuery", unit: "ns/op",
        old: 100, new: 150, delta: 50,
    }}

    var out strings.Builder
    printRows(&out, rows)
    rendered := out.String()

    require.Contains(t, rendered, "benchmark (ns/op)")
    require.NotContains(t, rendered, "benchmark (B/op)")
    require.NotContains(t, rendered, "benchmark (MB/s)")
}

// TestParseBenchFileMissingFileErrors pins the error path: an
// unreadable file fails with the wrapped open error.
func TestParseBenchFileMissingFileErrors(t *testing.T) {
    missing := writeTemp(t, "absent.txt", "x")
    require.NoError(t, os.Remove(missing))
    _, err := parseBenchFile(missing)
    require.Error(t, err)
    require.Contains(t, err.Error(), "open "+missing)
}

// TestDiffZeroBaseValueKeepsTheDeltaZero pins the division guard: a
// zero baseline renders the flat delta instead of dividing by zero.
func TestDiffZeroBaseValueKeepsTheDeltaZero(t *testing.T) {
    base := map[string]*result{
        "BenchmarkZero": {metrics: map[string]float64{"ns/op": 0}},
    }
    fresh := map[string]*result{
        "BenchmarkZero": {metrics: map[string]float64{"ns/op": 42}},
    }

    rows := diff(base, fresh)
    require.Len(t, rows, 1)
    require.InDelta(t, 0.0, rows[0].delta, 0.0001)
    require.InDelta(t, 42.0, rows[0].new, 0.001)
}

// TestPrintMissingNamesVanishedBenchmarks pins the vanished side of
// the gap report: a baseline benchmark the fresh run stopped
// matching renders under its own label.
func TestPrintMissingNamesVanishedBenchmarks(t *testing.T) {
    base := map[string]*result{
        "BenchmarkGone": {metrics: map[string]float64{"ns/op": 1}},
    }
    fresh := map[string]*result{
        "BenchmarkHere": {metrics: map[string]float64{"ns/op": 1}},
    }

    var out strings.Builder
    printMissing(&out, base, fresh)
    rendered := out.String()

    require.Contains(t, rendered, "vanished from the fresh run")
    require.Contains(t, rendered, "BenchmarkGone")
    require.Contains(t, rendered, "no baseline")
    require.Contains(t, rendered, "BenchmarkHere")
}

// TestDiffSkipsMetricsMissingOnOneSide pins the metric join: a unit
// present only in the baseline (or only in the fresh run) never
// renders a row.
func TestDiffSkipsMetricsMissingOnOneSide(t *testing.T) {
    base := map[string]*result{
        "BenchmarkHalf": {metrics: map[string]float64{
            "ns/op": 100, "B/op": 8,
        }},
    }
    fresh := map[string]*result{
        "BenchmarkHalf": {metrics: map[string]float64{"ns/op": 120}},
    }

    rows := diff(base, fresh)
    require.Len(t, rows, 1)
    require.Equal(t, "ns/op", rows[0].unit)
}

// TestBenchmarkNameStripsOnlyParsedSuffixes pins the boundary of the
// GOMAXPROCS strip: the suffix leaves only when the digit tail
// parses, a dash word pair stays part of the name.
func TestBenchmarkNameStripsOnlyParsedSuffixes(t *testing.T) {
    require.Equal(t, "BenchmarkX", benchmarkName("BenchmarkX-8"))
    require.Equal(t, "BenchmarkX", benchmarkName("BenchmarkX-16"))
    require.Equal(t, "BenchmarkX-abc", benchmarkName("BenchmarkX-abc"))
}
