// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// The server frame click transport: every plan derived click z the
// bot sends rides a measured offset that carries it from the mesh
// frame of the bot's own geodata pack into the frame the live game
// server answers clicks in.
//
// Why the transport exists (the village refused click root cause,
// development log round 85): the Mobius C1 server resolves the
// destination layer of a mouse click by the NEAREST height to the z
// the request carries - isCompletelyBlocked(target, targetZ) in the
// packet handler and the final layer rule of getValidLocation
// (previousZ != nearestToZ collapses the destination back onto the
// walker, the distance drops below the cancellation limit and the
// deployed build answers ActionFailed). The plan waypoints carry the
// layer heights of the bot's own pack (node.layer.Height), and where
// the two packs disagree about the absolute height of a surface - the
// village plaza deck stacked over the water floor 872 units apart,
// the teacher hall interior whose flip margins shrink to 164 units -
// the mesh z names the wrong layer, the walk arrives on the standing
// surface, the final rule collapses the click and the character never
// moves.
//
// The transport is the honest bot side answer with the server kept as
// the source of truth: the one z the server itself vouches for is the
// character's standing z (every position broadcast carries it), and
// the mesh knows the same standing cell in its own frame (the plan's
// first waypoint IS the character's cell resolved on the pack). The
// difference of the two is the measured frame offset of the surface
// the character stands on, and adding it to every plan waypoint z
// re-anchors the plan into the server frame while keeping the mesh's
// own relative geometry - the slopes, the stairs, the 864 unit rises
// the quest walks proved the raw self z can never ride (the Gludio
// segment run: every click on the self z answered ActionFailed, the
// waypoint height walked).
package hunt

import "math"

// frameOffsetLimit caps a measured z frame offset. A real pack
// vintage disagreement shifts a surface by tens to a few hundred
// units (the measured flip margins sit at 164 for the teacher hall
// interior and 436 for the plaza deck over the water floor); a
// measurement beyond the limit is not a shift but a layer snap - the
// pack resolved the standing cell onto a different layer of the
// sandwich (the 872 unit deck over water gap) - and anchoring by a
// layer gap would corrupt every click of the plan. The measurement is
// discarded (the offset stays zero) and the raw mesh z rides instead:
// the refusal ladder of the walker owns the plans whose start the
// pack cannot resolve onto the standing layer.
const frameOffsetLimit = 500.0

// measureFrameOffset measures the z frame offset between the server
// frame and the mesh frame from one ground truth pair: the character's
// server vouched standing z and the mesh frame height of the same
// cell. A pair whose difference exceeds frameOffsetLimit measures a
// layer snap, not a vintage shift, and answers zero (see the limit).
func measureFrameOffset(selfZ int32, meshZ float64) float64 {
    offset := float64(selfZ) - meshZ
    if math.Abs(offset) > frameOffsetLimit {
        return 0
    }

    return offset
}

// anchorZToServerFrame transports a mesh frame z into the server
// frame through a measured frame offset: the plan's relative geometry
// (the slopes, the stairs, the rises) rides unchanged on top of the
// server vouched surface height.
func anchorZToServerFrame(meshZ float64, offset float64) float64 {
    return meshZ + offset
}
