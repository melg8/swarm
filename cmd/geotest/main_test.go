// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import "testing"

// The smoke pass of issue #9: the geodata probe runs against a
// missing geodata directory (the CI checkout carries none) and
// answers with the stats line and the path error instead of
// panicking - the graceful missing data contract of the offline
// probes. On a dev host with the l2j tree beside the repo the run
// plans the real guard route; either way the contract is: no
// panic, a clean exit.
func TestGeotestSmoke(_ *testing.T) {
    main()
}
