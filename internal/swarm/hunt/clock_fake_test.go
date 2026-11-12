// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "sync"
    "time"
)

// fakeClock drives the time source of the loop and the tracker in the
// tests: Advance moves the instant without sleeping. The zero offset
// starts at the real instant of the construction, so the timestamps a
// test records before install stay comparable and a decision window
// reads the same elapsed time it would read live.
type fakeClock struct {
    mu  sync.Mutex
    now time.Time
}

// newFakeClock returns a clock frozen at the current instant.
func newFakeClock() *fakeClock {
    return &fakeClock{mu: sync.Mutex{}, now: time.Now()}
}

// Now reports the frozen instant, moved by every Advance.
func (c *fakeClock) Now() time.Time {
    c.mu.Lock()
    defer c.mu.Unlock()

    return c.now
}

// Advance moves the clock forward by d.
func (c *fakeClock) Advance(d time.Duration) {
    c.mu.Lock()
    defer c.mu.Unlock()

    c.now = c.now.Add(d)
}

// install makes the clock the time source of the loop and its tracker:
// the decision windows of both layers advance together.
func (c *fakeClock) install(loop *Loop) {
    loop.SetClock(c.Now)
    loop.tracker.SetClock(c.Now)
}
