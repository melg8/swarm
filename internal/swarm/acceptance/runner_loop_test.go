// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
    "context"
    "testing"
    "time"
)

// The session loop lifecycle tests (the temp18 acceptance dump of
// 2026-11-09): the hunt loop rides a per-session context and the
// supervisor joins it before reconnecting, so a relogin never leaves
// the previous loop ticking against the closed game client.

// waitClosed waits for the channel to close and fails the test when
// it does not within the generous bound.
func waitClosed(t *testing.T, ch <-chan struct{}, message string) {
    t.Helper()
    select {
    case <-ch:
    case <-time.After(5 * time.Second):
        t.Fatal(message)
    }
}

// TestStartSessionLoopJoinsTheLoop pins the stop contract: the stop
// function cancels the loop context and waits for the goroutine, and
// calling it twice stays harmless.
func TestStartSessionLoopJoinsTheLoop(t *testing.T) {
    parent, cancelParent := context.WithCancel(context.Background())
    defer cancelParent()

    started := make(chan struct{})
    stopped := make(chan struct{})
    stop := startSessionLoop(parent, func(ctx context.Context) {
        close(started)
        <-ctx.Done()
        close(stopped)
    })
    waitClosed(t, started, "the loop must start")

    stop()
    waitClosed(t, stopped, "the stop must join the loop goroutine")
    stop()
}

// TestStartSessionLoopFollowsTheParent pins the other end of the
// lifetime: cancelling the supervisor context ends the loop even
// without the stop call, so the shutdown path never waits forever.
func TestStartSessionLoopFollowsTheParent(t *testing.T) {
    parent, cancelParent := context.WithCancel(context.Background())

    started := make(chan struct{})
    stopped := make(chan struct{})
    stop := startSessionLoop(parent, func(ctx context.Context) {
        close(started)
        <-ctx.Done()
        close(stopped)
    })
    waitClosed(t, started, "the loop must start")

    cancelParent()
    waitClosed(t, stopped, "the parent cancel must end the loop")
    stop()
}
