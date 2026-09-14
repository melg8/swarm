// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

// TestRefusalEvidenceAttributesOnlyTheClicksOwnAnswer pins the honest
// attribution of the ActionFailed answer: the packet carries no
// request identity, so an arrival with a NON walk request in flight
// (the equip and skill requests of the gear machinery answer
// ActionFailed the same way) may answer that request instead of the
// walk click - the 2026-09-14 12:31 dump rerun showed the equip
// refusals landing one to three seconds after the walk clicks of the
// same window, and attributing them to the clicks lashed refusals the
// walk never saw.
func TestRefusalEvidenceAttributesOnlyTheClicksOwnAnswer(t *testing.T) {
    loop, _, bot, _ := newTripLoop()
    base := time.Now()
    loop.moveAt = base

    // A refusal answer with no other request in flight belongs to the
    // click.
    bot.ApplyActionFailed(base.Add(time.Second))
    require.True(t, loop.refusalEvidence(),
        "the fresh answer of the click counts")

    // An equip request sent after the click owns the next arrival: the
    // gear swap refusals ride the walk clicks exactly this way.
    loop.moveAt = base.Add(4 * time.Second)
    bot.ApplyOtherRequestSent(base.Add(5 * time.Second))
    bot.ApplyActionFailed(base.Add(6 * time.Second))
    require.False(t, loop.refusalEvidence(),
        "the answer of the equip request is not a walk refusal")

    // The same answer without the equip in flight is the click's own.
    bot.ApplyOtherRequestSent(time.Time{})
    require.True(t, loop.refusalEvidence(),
        "with no other request in flight the answer counts again")

    // A request sent BEFORE the click does not own the arrival: the
    // click is the younger request.
    bot.ApplyOtherRequestSent(base.Add(2 * time.Second))
    loop.moveAt = base.Add(8 * time.Second)
    bot.ApplyActionFailed(base.Add(9 * time.Second))
    require.True(t, loop.refusalEvidence(),
        "an older request does not steal the answer of the click")

    // An other request sent exactly AFTER the arrival window does not
    // own the arrival either.
    bot.ApplyOtherRequestSent(base.Add(20 * time.Second))
    require.True(t, loop.refusalEvidence(),
        "a request sent after the arrival keeps the attribution")
}
