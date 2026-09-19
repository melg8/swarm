// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

//go:build !race

package pathfind

// raceDetectorBudget scales the wall time budgets of the planning
// tests: the plain binary runs at production speed, the budget is
// the unmodified hunt tick figure. See race_budget_test.go for the
// race instrumented counterpart.
const raceDetectorBudget = 1
