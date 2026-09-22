// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import "testing"

// The smoke pass of issue #9: the stuck points probe runs against
// a missing navmesh directory (the CI checkout carries none) and
// answers with the "no start poly" line for every probed point
// instead of panicking - the graceful missing data contract of the
// offline probes. On a dev host with data/navmesh beside the repo
// it prints the real verdicts; either way the contract is: no
// panic, a clean exit.
func TestStuckprobeSmoke(_ *testing.T) {
    main()
}
