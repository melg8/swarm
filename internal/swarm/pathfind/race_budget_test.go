// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

//go:build race

package pathfind

// raceDetectorBudget scales the wall time budgets of the planning
// tests while the race detector instruments the binary: the detector
// slows the A* hot loop roughly six fold (the zone route suite runs
// 146 s plain and 761 s under -race on the same sandbox), so the
// production hunt tick budget stays meaningful only when scaled by
// the same factor.
const raceDetectorBudget = 10
