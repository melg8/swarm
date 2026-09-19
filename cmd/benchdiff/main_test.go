// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "os"
    "path/filepath"
    "strings"
    "testing"

    "github.com/stretchr/testify/require"
)

// writeTemp writes one benchmark output file into a fresh temp dir
// and returns its path.
func writeTemp(t *testing.T, name, content string) string {
    t.Helper()
    path := filepath.Join(t.TempDir(), name)
    require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

    return path
}

const baseOutput = `goos: linux
goarch: amd64
cpu: test cpu
BenchmarkPlanQuery-8         512       2345678 ns/op      4096 B/op       12 allocs/op
BenchmarkLayerWalk-8        1024        123456 ns/op       512 B/op        4 allocs/op
PASS
ok      github.com/melg8/swarm/internal/swarm/pathfind  1.234s
`

const freshOutput = `goos: linux
goarch: amd64
cpu: test cpu
BenchmarkPlanQuery-8         512       2932098 ns/op      5120 B/op       15 allocs/op
BenchmarkLayerWalk-8        1024        111111 ns/op       512 B/op        4 allocs/op
BenchmarkNewName-8          2048         65536 ns/op       128 B/op        1 allocs/op
PASS
ok      github.com/melg8/swarm/internal/swarm/pathfind  1.345s
`

func TestParseBenchFile(t *testing.T) {
    path := writeTemp(t, "base.txt", baseOutput)
    results, err := parseBenchFile(path)
    require.NoError(t, err)
    require.Len(t, results, 2)

    plan := results["BenchmarkPlanQuery"]
    require.NotNil(t, plan)
    require.InDelta(t, 2345678, plan.metrics["ns/op"], 0.001)
    require.InDelta(t, 4096, plan.metrics["B/op"], 0.001)
    require.InDelta(t, 12, plan.metrics["allocs/op"], 0.001)
}

func TestBenchmarkNameStripsGomaxprocs(t *testing.T) {
    require.Equal(t, "BenchmarkPlanQuery",
        benchmarkName("BenchmarkPlanQuery-8"))
    require.Equal(t, "BenchmarkPlanQuery",
        benchmarkName("BenchmarkPlanQuery"))
    // A dash that is not a GOMAXPROCS suffix stays in the name.
    require.Equal(t, "BenchmarkWith-Dash",
        benchmarkName("BenchmarkWith-Dash"))
}

func TestDiffJoinsSharedMetrics(t *testing.T) {
    base, err := parseBenchFile(
        writeTemp(t, "base.txt", baseOutput))
    require.NoError(t, err)
    fresh, err := parseBenchFile(
        writeTemp(t, "fresh.txt", freshOutput))
    require.NoError(t, err)

    rows := diff(base, fresh)
    // PlanQuery: three shared metrics; LayerWalk: three (all flat
    // except ns/op); NewName has no baseline, so it rides the
    // missing list, not the rows.
    require.Len(t, rows, 6)

    var planNS row
    for _, r := range rows {
        if r.name == "BenchmarkPlanQuery" && r.unit == "ns/op" {
            planNS = r
        }
    }
    require.InDelta(t, 2345678, planNS.old, 0.001)
    require.InDelta(t, 2932098, planNS.new, 0.001)
    // The regression rounds to +25.01 percent.
    require.InDelta(t, 25.01, planNS.delta, 0.01)
}

func TestPrintMissingNamesTheGaps(t *testing.T) {
    base, err := parseBenchFile(
        writeTemp(t, "base.txt", baseOutput))
    require.NoError(t, err)
    fresh, err := parseBenchFile(
        writeTemp(t, "fresh.txt", freshOutput))
    require.NoError(t, err)

    var out strings.Builder
    printMissing(&out, base, fresh)
    rendered := out.String()
    require.Contains(t, rendered, "BenchmarkNewName")
    require.Contains(t, rendered, "no baseline")
    require.NotContains(t, rendered, "vanished from the fresh run")
}
