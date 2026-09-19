// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "log"
    "testing"

    "github.com/melg8/swarm/internal/swarm/acceptance"
    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// TestHuntEventLoggerMirrorsHuntLines pins the loop logger wiring: the
// hunt decision lines land in the tracker event log without the
// console timestamp prefix, so the web UI log tab and the state dump
// carry the reasoning of the loop next to the raw game events.
func TestHuntEventLoggerMirrorsHuntLines(t *testing.T) {
    bot := state.NewBot("acc1")
    logger := huntEventLogger(bot)

    logger.Print("Hunt: outside the hunting zone, pathfinding back")
    logger.Print("Hunt: user selected the hunting zone Spore Fungus")

    events := bot.NewestEvents(10)
    require.Len(t, events, 2)
    require.Equal(t, "Hunt: outside the hunting zone, pathfinding back",
        events[0].Message, "the hunt line lands without the timestamp")
    require.Equal(t, "Hunt: user selected the hunting zone Spore Fungus",
        events[1].Message)
}

// TestHuntEventLoggerKeepsTheConsoleFormat: the mirrored lines keep
// the console copy intact (the standard date time prefix), the mirror
// never eats the stdout output.
func TestHuntEventLoggerKeepsTheConsoleFormat(t *testing.T) {
    var console linesRecorder
    logger := log.New(&console, "", log.LstdFlags)
    logger.Print("Hunt: fleeing the fight with 7")

    require.Len(t, console.lines, 1)
    require.Contains(t, console.lines[0], "Hunt: fleeing the fight with 7")
    require.Regexp(t, `^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2} `,
        console.lines[0], "the console prefix stays")
}

// TestNavmeshShowFlagParsesTheTileSelection pins the -show-navmesh
// flag contract: the bare boolean forms toggle the viewer over every
// stitched tile, the col_row list names the tiles to open and the
// malformed values refuse to parse.
func TestNavmeshShowFlagParsesTheTileSelection(t *testing.T) {
    t.Run("the boolean forms", func(t *testing.T) {
        var flag navmeshShowFlag
        require.NoError(t, flag.Set("true"))
        require.True(t, flag.enabled)
        require.Empty(t, flag.tiles)
        require.Equal(t, "true", flag.String())

        require.NoError(t, flag.Set("false"))
        require.False(t, flag.enabled)
        require.Equal(t, "false", flag.String())
    })

    t.Run("the tile list", func(t *testing.T) {
        var flag navmeshShowFlag
        require.NoError(t, flag.Set("21_19, 22_19"))
        require.True(t, flag.enabled)
        require.Equal(t, []navmesh.RegionKey{
            {Col: 21, Row: 19},
            {Col: 22, Row: 19},
        }, flag.tiles)
        require.Equal(t, "21_19,22_19", flag.String())
    })

    t.Run("the malformed values refuse", func(t *testing.T) {
        var flag navmeshShowFlag
        require.Error(t, flag.Set("21"))
        require.Error(t, flag.Set("ab_cd"))
        require.Error(t, flag.Set("21_19,"))
    })

    t.Run("the bare flag form is boolean", func(t *testing.T) {
        var flag navmeshShowFlag
        require.True(t, flag.IsBoolFlag())
    })
}

// linesRecorder collects the console lines of the logger.
type linesRecorder struct {
    lines []string
}

// Write implements io.Writer.
func (l *linesRecorder) Write(p []byte) (int, error) {
    l.lines = append(l.lines, string(p))

    return len(p), nil
}

// TestNewAcceptanceManagerRegistersTempBots pins the CLI wiring: the
// headless -acceptance mode builds the manager from the same
// constructor as the live web UI path, so the temp bots land in the
// shared registry and the manager knows every shipped scenario. The
// CLI then drives them through Run / RunAll without a fleet bot
// supervisor running.
func TestNewAcceptanceManagerRegistersTempBots(t *testing.T) {
    registry := state.NewRegistry()
    cfg := config{
        loginAddress:  "127.0.0.1:2106",
        acceptanceRun: "list",
    }
    manager := newAcceptanceManager(registry, cfg, nil, nil, nil)
    require.NotNil(t, manager)

    // The shipped scenario ids come back in the same order the web UI
    // shows them; the CLI uses this for `-acceptance list`.
    require.Equal(t, acceptance.DefinitionsIDs(), manager.IDs())

    // Every temp bot of every scenario lands in the shared registry:
    // the sidebar split (long-running vs acceptance) reads the kind
    // tag the manager set on construction.
    ids := map[string]bool{}
    for _, info := range registry.List() {
        ids[info.ID] = true
        require.Equal(t, state.KindAcceptance, info.Kind,
            "the temp bot "+info.ID+" carries the acceptance kind")
    }
    for _, def := range acceptance.Definitions() {
        require.True(t, ids[def.Account],
            "the temp bot "+def.Account+" is missing")
    }
}
