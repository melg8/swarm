// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package memwatch

import (
    "bytes"
    "context"
    "log"
    "strings"
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

// TestWatchLogsFootprintLines pins the log contract: one line at
// once, then one per period, each carrying the heap, sys, goroutine
// and gc numbers the memory investigation pairs across runs.
func TestWatchLogsFootprintLines(t *testing.T) {
    var out bytes.Buffer
    logger := log.New(&out, "", 0)
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    done := make(chan struct{})
    go func() {
        defer close(done)
        Watch(ctx, logger, 20*time.Millisecond)
    }()
    deadline := time.Now().Add(2 * time.Second)
    for time.Now().Before(deadline) &&
        strings.Count(out.String(), "Memory: heap") < 3 {
        time.Sleep(5 * time.Millisecond)
    }
    cancel()
    <-done
    lines := strings.Count(out.String(), "Memory: heap")
    require.GreaterOrEqual(t, lines, 3, "log lines: %q", out.String())
    line := out.String()
    require.Contains(t, line, "MB")
    require.Contains(t, line, "goroutines")
    require.Contains(t, line, "gc")
}

// TestWatchDisabledGuards pins the guard behavior: a nil logger or a
// non positive period returns at once instead of ticking forever.
func TestWatchDisabledGuards(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())
    cancel()
    done := make(chan struct{})
    go func() {
        defer close(done)
        Watch(ctx, nil, time.Minute)
    }()
    select {
    case <-done:
    case <-time.After(time.Second):
        t.Fatal("Watch with a nil logger did not return")
    }
    done = make(chan struct{})
    go func() {
        defer close(done)
        Watch(ctx, log.New(&bytes.Buffer{}, "", 0), 0)
    }()
    select {
    case <-done:
    case <-time.After(time.Second):
        t.Fatal("Watch with a zero period did not return")
    }
}
