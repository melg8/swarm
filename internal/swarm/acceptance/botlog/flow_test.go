// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package botlog_test

import (
    "strings"
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/acceptance/botlog"
)

// TestFullRunFlow drives every emit path of one run in order (the
// header, the narration, the sql, the session, the packets, the
// checks, the sample and the verdict) and asserts the rendered file:
// the integration view of the wire format the unit tests split.
func TestFullRunFlow(t *testing.T) {
    dir := t.TempDir()
    head := botlog.Header{
        TestID:     "farm-readiness",
        Title:      "farm readiness - the level 15 walk",
        Account:    "temp1",
        Generation: 1,
        Timeout:    20 * time.Minute,
        Description: "Start: the elven fighter wakes at the spawn. " +
            "Flow: it shops. Pass: it kills.",
        Checks: []botlog.CheckDesc{
            {ID: "online", Label: "entered the world"},
            {ID: "equipped", Label: "wears weapon, armor and jewels"},
            {ID: "kill", Label: "killed a mob"},
        },
        Environment: []botlog.EnvLine{
            {Key: "login server", Value: "127.0.0.1:2106"},
        },
        Args: "./swarm -acceptance farm-readiness",
    }
    log, err := botlog.Open(dir, head)
    if err != nil {
        t.Fatalf("open: %v", err)
    }
    log.Event("run started")
    log.Session("login dial %s", "127.0.0.1:2106")
    log.Event("acceptance: character temp1 ready on account temp1")
    log.DB("DELETE FROM items WHERE owner_id=268435457")
    log.Checks(head.Timeout, head.Checks)
    log.Recv(buildNpcInfo(t))
    log.Send(buildMoveRequest(t))
    log.Hunt("Hunt: no weapon in hand, holding the target")
    log.CheckTransition("online", "entered the world", true,
        "online as temp1")
    log.Sample("state status=online phase=townWalk")
    bypass := []byte{0x21, 'n', 0, 'p', 0, 'c', 0, '_', 0, '1', 0, 0, 0}
    log.Send(bypass)
    log.CheckTransition("equipped",
        "wears weapon, armor and jewels", true,
        "6 armor slots, 5 jewels, weapon in hand")
    log.Verdict(true, "", []botlog.CheckDesc{
        {ID: "online", Label: "entered the world", Done: true,
            Detail: "online as temp1"},
        {ID: "equipped", Label: "wears weapon, armor and jewels",
            Done: true, Detail: "6 armor slots, 5 jewels"},
        {ID: "kill", Label: "killed a mob", Done: false,
            Detail: "no kills yet"},
    }, nil)
    log.Close()
    body := readLog(t, log.Path())

    // The header blocks and the contract publication.
    for _, want := range []string{
        "swarm acceptance run log",
        "test      : farm-readiness",
        "the scenario publishes its check list at the run start",
        "login server: 127.0.0.1:2106",
    } {
        if !strings.Contains(body, want) {
            t.Errorf("the file misses %q", want)
        }
    }

    // The event flow in order: the narration, the sql, the session,
    // the packets (both directions, the opcode names resolve per
    // side - 0x21 is RequestBypassToServer from the client and NOT
    // the server CharSelected of the same byte), the check flips,
    // the sample and the verdict.
    order := []string{
        "[scenario] run started",
        "[session] login dial 127.0.0.1:2106",
        "[db] DELETE FROM items WHERE owner_id=268435457",
        "[check] the pass/fail contract of this run",
        "[ ] online      entered the world",
        "[ ] equipped    wears weapon, armor and jewels",
        "[ ] kill        killed a mob",
        "[recv 0x22 NpcInfo]",
        "[send 0x01 MoveToLocation]",
        "[hunt] Hunt: no weapon in hand",
        "[check] completed online (entered the world)",
        "[sample] state status=online",
        "[send 0x21 RequestBypassToServer] bypass \"npc_1\"",
        "[check] completed equipped",
        "[verdict] PASSED after",
        "checks: 2 of 3 done",
        "packets: 1 received, 2 sent, 0 log lines dropped",
    }
    at := 0
    for _, want := range order {
        index := strings.Index(body[at:], want)
        if index < 0 {
            t.Errorf("the flow misses %q after offset %d", want, at)

            continue
        }
        at += index + len(want)
    }
}
