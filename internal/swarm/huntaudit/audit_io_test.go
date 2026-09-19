// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package huntaudit

import (
    "os"
    "path/filepath"
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

// TestLoadAuditBrokenJSONFails pins the corrupt resume state path: a
// half written evidence file fails the run instead of silently
// restarting the measurement.
func TestLoadAuditBrokenJSONFails(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "audit.json")
    require.NoError(t, os.WriteFile(path, []byte("{not json"), 0o600))

    _, err := loadAudit(path, false)
    require.Error(t, err)
    require.Contains(t, err.Error(), "parse audit "+path)
}

// TestLoadAuditUnreadableFileFails pins the read error path: a file
// that exists but cannot be read surfaces the wrapped read error.
func TestLoadAuditUnreadableFileFails(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "audit.json")
    require.NoError(t, os.WriteFile(path, []byte("{}"), 0o600))
    require.NoError(t, os.Remove(path))

    // The removed file rides the ErrNotExist branch (an empty
    // audit), so the read error needs a directory: reading a
    // directory never returns ErrNotExist, it fails.
    _, err := loadAudit(dir, false)
    require.Error(t, err)
    require.Contains(t, err.Error(), "read audit "+dir)
}

// TestWriteAuditPersistsTheSummary pins the atomic write: the temp
// file lands under the final name, the summary fields stamp the
// account and the wait the run carried.
func TestWriteAuditPersistsTheSummary(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "evidence.json")
    audit := newAuditFile()
    audit.Spots = append(audit.Spots, SpotRecord{ID: "elven-spot-53"})

    require.NoError(t, writeAudit(path, "probe", 15*time.Second, audit))

    // The temp artifact never survives the rename.
    _, err := os.Stat(path + ".tmp")
    require.ErrorIs(t, err, os.ErrNotExist)

    loaded, err := loadAudit(path, false)
    require.NoError(t, err)
    require.Equal(t, "probe", loaded.Account)
    require.Equal(t, 15, loaded.WaitSec)
    require.Len(t, loaded.Spots, 1)
    require.Equal(t, "elven-spot-53", loaded.Spots[0].ID)
}

// TestLoadAnchorsMissingFileFails pins the anchor override error: a
// path that does not exist fails the run, only the empty path
// selects the no-overrides mode.
func TestLoadAnchorsMissingFileFails(t *testing.T) {
    missing := filepath.Join(t.TempDir(), "absent.json")

    _, err := loadAnchors(missing)
    require.Error(t, err)
    require.Contains(t, err.Error(), "read anchors "+missing)
}
