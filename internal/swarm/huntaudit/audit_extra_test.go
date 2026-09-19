// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package huntaudit

import (
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/connection"
    "github.com/stretchr/testify/require"
)

// TestAuditFilterMatches pins the spot filter of the audit run: the
// empty filter audits everything, the comma separated substrings
// match by containment and the empty needles never match.
func TestAuditFilterMatches(t *testing.T) {
    require.True(t, auditFilterMatches("", "elven-spot-53"),
        "the empty filter audits every cell")
    require.True(t, auditFilterMatches("53", "elven-spot-53"))
    require.False(t, auditFilterMatches("54", "elven-spot-53"))
    require.True(t, auditFilterMatches("51,54,57", "elven-spot-57"),
        "one matching needle is enough")
    require.False(t, auditFilterMatches("51,54", "elven-spot-53"))
    require.False(t, auditFilterMatches(",", "elven-spot-53"),
        "the empty needles never match")
    require.False(t, auditFilterMatches("ELVEN", "elven-spot-53"),
        "the match is case sensitive")
}

// TestGameAddress pins the game endpoint rendering of the auth
// result: the four server ip bytes in dotted form and the port.
func TestGameAddress(t *testing.T) {
    auth := &connection.AuthResult{
        Account:    "probe",
        LoginOkID1: 1,
        LoginOkID2: 2,
        PlayOkID1:  3,
        PlayOkID2:  4,
        ServerID:   1,
        ServerIP:   [4]byte{192, 168, 56, 10},
        ServerPort: 7777,
    }
    require.Equal(t, "192.168.56.10:7777", gameAddress(auth))
}

// TestWriteAuditEmptyPathIsNoop pins the draft mode of the audit:
// the empty output path skips the evidence write (the run logs its
// spots but never stores them, the file struct stays untouched).
func TestWriteAuditEmptyPathIsNoop(t *testing.T) {
    audit := newAuditFile()
    audit.Account = "kept-untouched"
    require.NoError(t, writeAudit("", "probe", 12*time.Second, audit))
    require.Equal(t, "kept-untouched", audit.Account,
        "the draft mode never rewrites the file struct")
}

// TestNewAuditFileStartsEmpty pins the fresh run constructor.
func TestNewAuditFileStartsEmpty(t *testing.T) {
    audit := newAuditFile()
    require.Empty(t, audit.Account)
    require.Zero(t, audit.WaitSec)
    require.Empty(t, audit.Spots)
}
