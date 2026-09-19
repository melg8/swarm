// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "testing"

    "github.com/stretchr/testify/require"
)

// TestFleetAccountName pins the fleet naming ladder: the first bot
// keeps the base name, the followers strip the trailing digits and
// append their one based index (bot, bot2, bot3 - the auto-created
// account names of a fleet run).
func TestFleetAccountName(t *testing.T) {
    require.Equal(t, "bot", fleetAccountName("bot", 0))
    require.Equal(t, "bot2", fleetAccountName("bot", 1))
    require.Equal(t, "bot3", fleetAccountName("bot", 2))
    // A numbered base strips its digits before the index lands:
    // temp2 fleet starts temp2, temp3, temp4 - never temp22.
    require.Equal(t, "temp2", fleetAccountName("temp2", 0))
    require.Equal(t, "temp3", fleetAccountName("temp2", 1))
    require.Equal(t, "temp4", fleetAccountName("temp2", 2))
}

// TestFleetAccountListPinsTheStartupLine pins the comma separated
// account list of the startup log: the base name first, the count
// bounds the ladder, zero reports nothing.
func TestFleetAccountList(t *testing.T) {
    require.Empty(t, fleetAccountList("bot", 0))
    require.Equal(t, "bot", fleetAccountList("bot", 1))
    require.Equal(t, "bot, bot2, bot3", fleetAccountList("bot", 3))
    require.Equal(t, "temp2, temp3",
        fleetAccountList("temp2", 2))
}

// TestSplitAddressesPinsTheFlagSplit pins the comma separated flag
// parser: the entries trim their whitespace and the empty entries
// drop out, so " 127.0.0.1:2106 , ,127.0.0.2:2106 " yields the two
// listeners.
func TestSplitAddressesPinsTheFlagSplit(t *testing.T) {
    require.Equal(t,
        []string{"127.0.0.1:2106", "127.0.0.2:2106"},
        splitAddresses(" 127.0.0.1:2106 , ,127.0.0.2:2106 "))
    require.Equal(t, []string{"127.0.0.1:2106"},
        splitAddresses("127.0.0.1:2106"))
    require.Empty(t, splitAddresses(""))
    require.Empty(t, splitAddresses(" , "))
}
