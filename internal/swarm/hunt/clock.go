// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import "time"

// now returns the time source of the decision logic of the loop: the
// injected clock or the system clock of a zero value. The ticker of
// Run stays on the system clock: it paces the loop, it is not a
// decision input.
func (l *Loop) now() time.Time {
    if l.nowFn != nil {
        return l.nowFn()
    }

    return time.Now()
}

// SetClock replaces the time source of the loop. The production wiring
// keeps the system clock; the tests inject a fake clock so the freshness,
// combat, rest and trip windows advance without sleeping. A nil restores
// the system clock. Call it before Run.
func (l *Loop) SetClock(now func() time.Time) {
    l.nowFn = now
}
