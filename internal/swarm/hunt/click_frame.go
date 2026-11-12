// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// The server frame click transport: every plan derived click z rides a measured
// offset that carries it from the mesh frame of the bot's own geodata pack into
// the frame the live game server resolves clicks in (docs/hunting.md, the
// server frame click transport section).
package hunt

import "math"

// frameOffsetLimit caps a measured z frame offset: a real pack vintage shift
// moves a surface by tens to a few hundred units, while a larger measurement is
// a layer snap of the sandwich and anchoring by it would corrupt every click of
// the plan. The measurement is discarded then (the offset stays zero) and the
// refusal ladder of the walker owns the plan.
const frameOffsetLimit = 500.0

// measureFrameOffset measures the z frame offset between the server frame and
// the mesh frame from one ground truth pair: the character's server vouched
// standing z and the mesh height of the same cell. A difference past
// frameOffsetLimit is a layer snap and answers zero.
func measureFrameOffset(selfZ int32, meshZ float64) float64 {
    offset := float64(selfZ) - meshZ
    if math.Abs(offset) > frameOffsetLimit {
        return 0
    }

    return offset
}

// anchorZToServerFrame transports a mesh frame z into the server frame: the
// mesh's relative geometry (the slopes, the stairs, the rises) rides unchanged
// on top of the server vouched surface height.
func anchorZToServerFrame(meshZ float64, offset float64) float64 {
    return meshZ + offset
}
