// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Package memwatch logs the process memory footprint on a fixed
// cadence: a long unattended run (the 24/7 fleet) demonstrates its
// memory behavior from the log alone - the slow growth the web
// statistics chart shows becomes a searchable log line once a minute,
// comparable across runs and visible without the web interface.
package memwatch

import (
    "context"
    "log"
    "runtime"
    "time"
)

// DefaultPeriod is the log cadence: one line a minute keeps the
// evidence trail readable without drowning the bot log.
const DefaultPeriod = time.Minute

// Watch logs the memory footprint line every period until the context
// ends. The first line lands at once (the baseline of the run), then
// the ticker owns the rhythm. A nil logger or a non positive period
// disables the watch.
func Watch(ctx context.Context, logger *log.Logger, period time.Duration) {
    if logger == nil || period <= 0 {
        return
    }
    writeLine(logger)
    ticker := time.NewTicker(period)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            writeLine(logger)
        }
    }
}

// writeLine renders one footprint line: the live heap, the runtime
// reserved memory, the goroutine count and the GC cycle count - the
// four numbers a leak investigation starts from. The one decimal
// keeps the small single bot runs readable while the fleet sizes
// round fine.
func writeLine(logger *log.Logger) {
    var mem runtime.MemStats
    runtime.ReadMemStats(&mem)
    logger.Printf("Memory: heap %.1f MB, sys %.1f MB, goroutines %d, gc %d",
        float64(mem.HeapAlloc)/(1<<20), float64(mem.Sys)/(1<<20),
        runtime.NumGoroutine(), mem.NumGC)
}
