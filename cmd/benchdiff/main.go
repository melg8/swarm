// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Command benchdiff compares two `go test -bench` output files and
// prints the per benchmark delta of every shared metric (ns/op, B/op,
// allocs/op, MB/s). It is the repository-side plumbing of the
// benchmark baseline workflow: `task bench:save` commits the output
// of a touched package into runs/bench-<name>.txt and `task
// bench:diff` wraps this tool, so the "compare allocations
// before/after" rule reads the numbers from the committed baseline
// instead of the agent's memory. The offline twin of benchstat - no
// network, no module downloads, one parse walk.
package main

import (
    "bufio"
    "flag"
    "fmt"
    "io"
    "os"
    "sort"
    "strconv"
    "strings"
)

// result is one benchmark's metric values: the benchmark name (the
// -N GOMAXPROCS suffix stripped) mapped by metric unit.
type result struct {
    metrics map[string]float64
}

// metricOrder pins the printed column order (the output of a
// -benchmem run lists them in this order).
var metricOrder = []string{"ns/op", "B/op", "allocs/op", "MB/s"}

// parseBenchFile reads one benchmark output file into name -> result.
// A line counts when it starts with "Benchmark" and carries at least
// one metric pair (value unit); everything else (the goos/goarch
// header, the PASS tail, the test log noise) is skipped.
func parseBenchFile(path string) (map[string]*result, error) {
    file, err := os.Open(path)
    if err != nil {
        return nil, fmt.Errorf("open %s: %w", path, err)
    }
    defer file.Close()

    results := map[string]*result{}
    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        fields := strings.Fields(scanner.Text())
        if len(fields) < 3 || !strings.HasPrefix(fields[0], "Benchmark") {
            continue
        }
        name := benchmarkName(fields[0])
        res, ok := results[name]
        if !ok {
            res = &result{metrics: map[string]float64{}}
            results[name] = res
        }
        // The pairs start at field 2 (name, runs, value unit, ...).
        for i := 2; i+1 < len(fields); i += 2 {
            value, err := strconv.ParseFloat(fields[i], 64)
            if err != nil {
                continue
            }
            res.metrics[fields[i+1]] = value
        }
    }
    if err := scanner.Err(); err != nil {
        return nil, fmt.Errorf("scan %s: %w", path, err)
    }

    return results, nil
}

// benchmarkName strips the -N GOMAXPROCS suffix the testing package
// appends, so the same benchmark measured on different hosts joins.
func benchmarkName(raw string) string {
    if idx := strings.LastIndex(raw, "-"); idx > len("Benchmark") {
        if _, err := strconv.Atoi(raw[idx+1:]); err == nil {
            return raw[:idx]
        }
    }

    return raw
}

// row is one printed comparison: a benchmark, a metric unit and the
// old/new values with the relative delta.
type row struct {
    name  string
    unit  string
    old   float64
    new   float64
    delta float64
}

// diff joins the two result sets on the benchmark names and returns
// the comparison rows of every shared metric, sorted by name and the
// metric order. Benchmarks or metrics missing on one side are
// reported separately (the caller prints them as the no-baseline
// list).
func diff(base, fresh map[string]*result) []row {
    names := make([]string, 0, len(base))
    for name := range base {
        if _, ok := fresh[name]; ok {
            names = append(names, name)
        }
    }
    sort.Strings(names)

    rows := make([]row, 0, len(names)*len(metricOrder))
    for _, name := range names {
        for _, unit := range metricOrder {
            oldValue, ok := base[name].metrics[unit]
            if !ok {
                continue
            }
            newValue, ok := fresh[name].metrics[unit]
            if !ok {
                continue
            }
            delta := 0.0
            if oldValue != 0 {
                delta = (newValue - oldValue) / oldValue * 100
            }
            rows = append(rows, row{
                name:  name,
                unit:  unit,
                old:   oldValue,
                new:   newValue,
                delta: delta,
            })
        }
    }

    return rows
}

// printRows renders the comparison table: one block per metric unit,
// the delta signed and scaled to two decimals.
func printRows(out io.Writer, rows []row) {
    for _, unit := range metricOrder {
        block := make([]row, 0, len(rows))
        for _, r := range rows {
            if r.unit == unit {
                block = append(block, r)
            }
        }
        if len(block) == 0 {
            continue
        }
        fmt.Fprintf(out, "%-40s %14s %14s %9s\n",
            "benchmark ("+unit+")", "base", "new", "delta")
        for _, r := range block {
            fmt.Fprintf(out, "%-40s %14.6g %14.6g %+8.2f%%\n",
                r.name, r.old, r.new, r.delta)
        }
        fmt.Fprintln(out)
    }
}

// printMissing renders the benchmarks that live on one side only -
// the new names have no baseline yet, the vanished names stopped
// matching the -run filter of the fresh run.
func printMissing(out io.Writer, base, fresh map[string]*result) {
    for _, side := range []struct {
        label string
        set   map[string]*result
        other map[string]*result
    }{
        {label: "no baseline (commit one with bench:save)", set: fresh,
            other: base},
        {label: "vanished from the fresh run", set: base, other: fresh},
    } {
        names := make([]string, 0, len(side.set))
        for name := range side.set {
            if _, ok := side.other[name]; !ok {
                names = append(names, name)
            }
        }
        if len(names) == 0 {
            continue
        }
        sort.Strings(names)
        fmt.Fprintf(out, "%s:\n", side.label)
        for _, name := range names {
            fmt.Fprintf(out, "  %s\n", name)
        }
        fmt.Fprintln(out)
    }
}

func main() {
    basePath := flag.String("base", "",
        "the baseline benchmark output file (runs/bench-<name>.txt)")
    freshPath := flag.String("new", "",
        "the fresh benchmark output file to compare against it")
    flag.Parse()
    if *basePath == "" || *freshPath == "" {
        fmt.Fprintln(os.Stderr,
            "Error both -base and -new benchmark files are required")
        os.Exit(2)
    }
    base, err := parseBenchFile(*basePath)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error %v\n", err)
        os.Exit(1)
    }
    fresh, err := parseBenchFile(*freshPath)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error %v\n", err)
        os.Exit(1)
    }
    rows := diff(base, fresh)
    if len(rows) == 0 {
        fmt.Fprintln(os.Stderr,
            "Error no shared benchmark metrics between the files")
        os.Exit(1)
    }
    printRows(os.Stdout, rows)
    printMissing(os.Stdout, base, fresh)
}
