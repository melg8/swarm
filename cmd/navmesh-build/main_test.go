// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
    "github.com/stretchr/testify/require"
)

// The smoke pass of issue #9: the region selection of the build
// tool - the explicit spec parsing and the empty spec falling back
// to the directory scan.

func TestRegionKeysSpec(t *testing.T) {
    t.Run("the explicit spec parses", func(t *testing.T) {
        keys, err := regionKeys("ignored", "24_19,25_20")
        require.NoError(t, err)
        require.Equal(t, []navmesh.RegionKey{
            {Col: 24, Row: 19}, {Col: 25, Row: 20},
        }, keys)
    })

    t.Run("a spec without the underscore errors", func(t *testing.T) {
        _, err := regionKeys("ignored", "2419")
        require.ErrorContains(t, err, "bad region spec")
    })

    t.Run("a non numeric column errors", func(t *testing.T) {
        _, err := regionKeys("ignored", "ab_19")
        require.ErrorContains(t, err, "bad region col")
    })

    t.Run("a non numeric row errors", func(t *testing.T) {
        _, err := regionKeys("ignored", "24_cd")
        require.ErrorContains(t, err, "bad region row")
    })

    t.Run("an out of range row errors", func(t *testing.T) {
        _, err := regionKeys("ignored", "24_99999")
        require.ErrorContains(t, err, "bad region row")
    })
}

func TestRegionKeysDirScan(t *testing.T) {
    t.Run("the empty spec scans the l2j files of the dir", func(t *testing.T) {
        dir := t.TempDir()
        for _, name := range []string{
            "24_19.l2j", "24_20.l2j", "25_21.l2j",
            "notes.txt", "broken.l2j",
        } {
            require.NoError(t, os.WriteFile(
                filepath.Join(dir, name), []byte{0}, 0o600))
        }

        keys, err := regionKeys(dir, "")
        require.NoError(t, err)
        require.Equal(t, []navmesh.RegionKey{
            {Col: 24, Row: 19}, {Col: 24, Row: 20}, {Col: 25, Row: 21},
        }, keys)
    })

    t.Run("a missing dir errors", func(t *testing.T) {
        _, err := regionKeys(filepath.Join(t.TempDir(), "gone"), "")
        require.ErrorContains(t, err, "geodata directory")
    })
}
