// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import "time"

// now returns the time source of the tracker: the injected clock or the
// system clock of a zero value.
func (b *Bot) now() time.Time {
    if b.nowFn != nil {
        return b.nowFn()
    }

    return time.Now()
}

// SetClock replaces the time source of the tracker. The hunt loop injects
// the same clock into both layers so the decision windows read one
// instant; the tests inject a fake clock instead of sleeping. A nil
// restores the system clock. Call it before the tracker is shared with
// the packet producers.
func (b *Bot) SetClock(now func() time.Time) {
    b.nowFn = now
}
