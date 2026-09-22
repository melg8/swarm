// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "flag"
    "os"
    "path/filepath"
    "testing"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// TestExplicitAccountReadsTheCommandLine pins the opt-in bot filter:
// the journal tools treat the default login account as a non filter,
// only an -account flag passed on the command line narrows the report.
func TestExplicitAccountReadsTheCommandLine(t *testing.T) {
    saved := flag.CommandLine
    t.Cleanup(func() { flag.CommandLine = saved })

    t.Run("the default account is not a filter", func(t *testing.T) {
        flag.CommandLine = flag.NewFlagSet("report", flag.ContinueOnError)
        flag.CommandLine.String("account", defaultAccount, "account name")
        require.NoError(t, flag.CommandLine.Parse(nil))
        require.False(t, explicitAccount())
    })

    t.Run("the explicit flag filters", func(t *testing.T) {
        flag.CommandLine = flag.NewFlagSet("report", flag.ContinueOnError)
        flag.CommandLine.String("account", defaultAccount, "account name")
        require.NoError(t,
            flag.CommandLine.Parse([]string{"-account", "bot2"}))
        require.True(t, explicitAccount())
    })
}

// TestDetectGeodataDirPicksTheFirstCandidateDirectory pins the
// autodetection: the first existing candidate of the list wins and a
// tree without geodata falls back to the documented default, so the
// startup log names the same directory either way.
func TestDetectGeodataDirPicksTheFirstCandidateDirectory(t *testing.T) {
    t.Run("the shipped tree layout", func(t *testing.T) {
        t.Chdir(t.TempDir())
        require.NoError(t, os.MkdirAll(
            filepath.Join("data", "geodata"), 0o755))
        require.Equal(t,
            filepath.Join("data", "geodata"), detectGeodataDir())
    })

    t.Run("the missing tree keeps the default", func(t *testing.T) {
        t.Chdir(t.TempDir())
        require.Equal(t,
            defaultGeodataCandidates[0], detectGeodataDir())
    })
}

// TestDetectNavmeshDirNeedsTileFiles pins the mesh autodetection: the
// candidate directory counts only when it holds at least one .nm tile
// file, an empty directory and an absent directory both answer "" so
// the hunt loop stays on the grid engine without noise.
func TestDetectNavmeshDirNeedsTileFiles(t *testing.T) {
    const meshDir = "data" + string(filepath.Separator) + "navmesh"

    t.Run("the directory with tiles", func(t *testing.T) {
        t.Chdir(t.TempDir())
        require.NoError(t, os.MkdirAll(meshDir, 0o755))
        require.NoError(t, os.WriteFile(
            filepath.Join(meshDir, "21_19.nm"), []byte{0}, 0o600))
        require.Equal(t, meshDir, detectNavmeshDir())
    })

    t.Run("the directory without tiles", func(t *testing.T) {
        t.Chdir(t.TempDir())
        require.NoError(t, os.MkdirAll(meshDir, 0o755))
        require.Empty(t, detectNavmeshDir())
    })

    t.Run("the missing directory", func(t *testing.T) {
        t.Chdir(t.TempDir())
        require.Empty(t, detectNavmeshDir())
    })
}

// TestIntersectTilesKeepsTheExistingTiles pins the flag validation of
// the viewer: the requested keys keep their order, the tiles of
// another pack drop out and the non tile files of the directory never
// masquerade as regions.
func TestIntersectTilesKeepsTheExistingTiles(t *testing.T) {
    dir := t.TempDir()
    for _, name := range []string{"21_19.nm", "22_19.nm", "notes.txt"} {
        require.NoError(t, os.WriteFile(
            filepath.Join(dir, name), []byte{0}, 0o600))
    }
    mesh := navmesh.NewMesh(dir)

    found := intersectTiles(mesh, []navmesh.RegionKey{
        {Col: 21, Row: 19},
        {Col: 99, Row: 99},
        {Col: 22, Row: 19},
    })
    require.Equal(t, []navmesh.RegionKey{
        {Col: 21, Row: 19},
        {Col: 22, Row: 19},
    }, found, "the requested order survives, the unknown tile drops")

    require.Empty(t, intersectTiles(mesh, []navmesh.RegionKey{
        {Col: 99, Row: 99},
    }), "a request without a single existing tile answers empty")
}

// TestNavmeshViewerInitialResolvesTheTileSelection pins the viewer
// opening set: the named tiles open alone, a request with no single
// existing tile falls back to every tile and the empty flag opens
// every tile as is.
func TestNavmeshViewerInitialResolvesTheTileSelection(t *testing.T) {
    dir := t.TempDir()
    for _, name := range []string{"21_19.nm", "22_19.nm"} {
        require.NoError(t, os.WriteFile(
            filepath.Join(dir, name), []byte{0}, 0o600))
    }
    mesh := navmesh.NewMesh(dir)
    stats := mesh.Stats()

    t.Run("the named tiles open alone", func(t *testing.T) {
        initial := navmeshViewerInitial(mesh, dir, []navmesh.RegionKey{
            {Col: 22, Row: 19},
        }, stats.TileFiles, stats.Dir)
        require.Equal(t, []navmesh.RegionKey{{Col: 22, Row: 19}}, initial)
    })

    t.Run("the unknown selection falls back to every tile", func(t *testing.T) {
        initial := navmeshViewerInitial(mesh, dir, []navmesh.RegionKey{
            {Col: 99, Row: 99},
        }, stats.TileFiles, stats.Dir)
        require.Empty(t, initial, "the fallback clears the selection")
    })

    t.Run("the bare flag opens every tile", func(t *testing.T) {
        initial := navmeshViewerInitial(
            mesh, dir, nil, stats.TileFiles, stats.Dir)
        require.Empty(t, initial, "the empty selection stays empty")
    })
}

// TestLoadNavmeshAnswersNilWithoutTiles pins the mesh builder of the
// bot modes: the tile directory with files builds the mesh, the empty
// or absent directories answer nil so the navigator installs the pure
// grid engine.
func TestLoadNavmeshAnswersNilWithoutTiles(t *testing.T) {
    t.Run("the tiles build the mesh", func(t *testing.T) {
        dir := t.TempDir()
        for _, name := range []string{"21_19.nm", "22_19.nm"} {
            require.NoError(t, os.WriteFile(
                filepath.Join(dir, name), []byte{0}, 0o600))
        }
        mesh := loadNavmesh(config{navmeshDir: dir})
        require.NotNil(t, mesh)
        require.Equal(t, 2, mesh.Stats().TileFiles)
    })

    t.Run("the empty tile directory stays on the grid", func(t *testing.T) {
        t.Chdir(t.TempDir())
        require.Nil(t, loadNavmesh(config{
            navmeshDir: filepath.Join("data", "navmesh"),
        }))
    })

    t.Run("the empty flag falls back to the autodetection", func(t *testing.T) {
        t.Chdir(t.TempDir())
        require.Nil(t, loadNavmesh(config{navmeshDir: ""}))
    })
}

// TestHuntEventMirrorStreamsHuntLines pins the event mirror: the hunt
// log lines land in the tracker event log without the console
// timestamp prefix, the non hunt lines mirror as is and the blank
// lines never reach the event log.
func TestHuntEventMirrorStreamsHuntLines(t *testing.T) {
    bot := state.NewBot("acc1")
    mirror := huntEventMirror{tracker: bot}

    n, err := mirror.Write([]byte(
        "2026/09/21 20:00:00 Hunt: fleeing the fight with 7\n"))
    require.NoError(t, err)
    require.Equal(t, len("2026/09/21 20:00:00 Hunt: fleeing the fight with 7\n"), n)

    _, err = mirror.Write([]byte("plain console line\n"))
    require.NoError(t, err)

    _, err = mirror.Write([]byte("   \n"))
    require.NoError(t, err)

    events := bot.NewestEvents(10)
    require.Len(t, events, 2)
    require.Equal(t, "Hunt: fleeing the fight with 7", events[0].Message)
    require.Equal(t, "plain console line", events[1].Message)
}
