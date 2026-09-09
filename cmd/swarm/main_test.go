// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
	"log"
	"testing"

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

// linesRecorder collects the console lines of the logger.
type linesRecorder struct {
	lines []string
}

// Write implements io.Writer.
func (l *linesRecorder) Write(p []byte) (int, error) {
	l.lines = append(l.lines, string(p))

	return len(p), nil
}
