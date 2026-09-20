// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// The public emit surface of the run log: the scenario events, the
// packet taps, the monitor check transitions, the state samples and
// the verdict. Every method is nil-safe and non-blocking (see
// RunLog.emit).

package botlog

import (
    "fmt"
    "strconv"
    "time"
)

// Event tags of the run log lines.
const (
    tagScenario = "scenario"
    tagHunt     = "hunt"
    tagDB       = "db"
    tagSession  = "session"
    tagCheck    = "check"
    tagSample   = "sample"
    tagVerdict  = "verdict"
)

// Event writes one narration line of the scenario or the session
// plumbing (the same lines the web UI test log shows).
func (l *RunLog) Event(format string, args ...any) {
    l.emit(tagScenario, []string{fmt.Sprintf(format, args...)})
}

// Hunt writes one hunt loop decision line (the logger mirror of the
// loop - the lines that start with "Hunt:" in the test log).
func (l *RunLog) Hunt(line string) {
    l.emit(tagHunt, []string{line})
}

// DB writes one database statement or answer line of the character
// injection (the server side preparation of the run).
func (l *RunLog) DB(line string) {
    l.emit(tagDB, []string{line})
}

// Session writes one connection lifecycle line: the dials, the
// handshakes, the world entry, the reconnects.
func (l *RunLog) Session(format string, args ...any) {
    l.emit(tagSession, []string{fmt.Sprintf(format, args...)})
}

// Recv decodes and writes one server packet the session received.
// The payload is the decrypted raw packet (opcode first); the
// callback runs on the session reader goroutine and must copy
// synchronously - the decode renders into fresh strings at once.
func (l *RunLog) Recv(payload []byte) {
    if l == nil || len(payload) == 0 {
        return
    }
    l.recvN.Add(1)
    l.emitPacket("recv", payload, decodeRecv)
}

// Send decodes and writes one client packet the session sent (the
// payload is the serialized plaintext before encryption).
func (l *RunLog) Send(payload []byte) {
    if l == nil || len(payload) == 0 {
        return
    }
    l.sentN.Add(1)
    l.emitPacket("send", payload, decodeSend)
}

// emitPacket renders one packet through the decoder and queues the
// lines under the "recv 0xNN Name" / "send 0xNN Name" tag.
func (l *RunLog) emitPacket(
    side string, payload []byte, decode func([]byte) []string,
) {
    lines := decode(payload)
    if len(lines) == 0 {
        return
    }
    l.emit(side+" "+packetLabel(side, payload), lines)
}

// packetLabel renders the "0xNN Name" identity of a packet payload.
// The recv and the send opcode spaces overlap (the same byte names a
// different packet per direction), so the side picks the table.
func packetLabel(side string, payload []byte) string {
    var name string
    if side == "send" {
        name = sendOpcodeNames[payload[0]]
    } else {
        name = recvOpcodeNames[payload[0]]
    }
    if name == "" {
        name = "unknown"
    }

    return "0x" + hexByte(payload[0]) + " " + name
}

// hexByte renders two lowercase hex digits of a byte.
func hexByte(value byte) string {
    const digits = "0123456789abcdef"

    return string([]byte{digits[value>>4], digits[value&0x0f]})
}

// Checks writes the pass/fail contract block of the run: the
// scenario publishes its check list at the start (the setChecks
// call of the scenario function), and the block lands as the first
// [check] event - the list the monitor watches for the rest of the
// run.
func (l *RunLog) Checks(timeout time.Duration, checks []CheckDesc) {
    if l == nil || len(checks) == 0 {
        return
    }
    lines := make([]string, 0, len(checks)+2)
    lines = append(lines, "the pass/fail contract of this run: the "+
        "monitor rewrites the checks below from the live state;")
    lines = append(lines, "the run passes when the scenario returns "+
        "nil (every check done) and fails on the scenario error,"+
        " the timeout ("+timeout.String()+") or a cancel.")
    for i := range checks {
        lines = append(lines, "  "+checkMark(checks[i].Done)+" "+
            padRight(checks[i].ID, checkIDWidth)+checks[i].Label)
    }
    l.emit(tagCheck, lines)
}

// CheckTransition writes one check state flip the monitor decided
// (done or reopened), with the label for the readers who grep by
// name instead of id. Only the state changes land as lines (a
// steady check rewrites every monitor period - the transitions are
// the story the file needs).
func (l *RunLog) CheckTransition(
    id string, label string, done bool, detail string,
) {
    verb := "completed"
    if !done {
        verb = "reopened"
    }
    l.emit(tagCheck, []string{
        verb + " " + id + " (" + label + "): " + detail,
    })
}

// Sample writes one periodic character state line (the sampler of
// the manager renders the line from the tracker).
func (l *RunLog) Sample(line string) {
    l.emit(tagSample, []string{line})
}

// Stats carries the counters of the finished run.
type Stats struct {
    RecvPackets uint64
    SentPackets uint64
    Dropped     uint64
}

// stats snapshots the counters.
func (l *RunLog) stats() Stats {
    return Stats{
        RecvPackets: l.recvN.Load(),
        SentPackets: l.sentN.Load(),
        Dropped:     l.dropped.Load(),
    }
}

// Verdict writes the outcome block of the run: the verdict, the
// reason, the final state of every check and the packet counters.
// It flushes at once so the block survives a crash right after.
func (l *RunLog) Verdict(
    passed bool, reason string, checks []CheckDesc, err error,
) {
    if l == nil {
        return
    }
    verdict := "FAILED"
    if passed {
        verdict = "PASSED"
    }
    lines := make([]string, 0, len(checks)+8)
    lines = append(lines, verdict+" after "+
        time.Since(l.started).Round(time.Millisecond).String())
    if err != nil {
        lines = append(lines, "error: "+err.Error())
    }
    if reason != "" {
        lines = append(lines, "reason: "+reason)
    }
    done := 0
    for i := range checks {
        if checks[i].Done {
            done++
        }
        lines = append(lines, checkMark(checks[i].Done)+" "+
            checks[i].ID+" ("+checks[i].Label+"): "+checks[i].Detail)
    }
    if len(checks) > 0 {
        lines = append(lines, "checks: "+strconv.Itoa(done)+" of "+
            strconv.Itoa(len(checks))+" done")
    }
    s := l.stats()
    lines = append(lines, fmt.Sprintf(
        "packets: %d received, %d sent, %d log lines dropped",
        s.RecvPackets, s.SentPackets, s.Dropped))
    l.emit(tagVerdict, lines)
}

// Close stops the writer goroutine after it drains the queue. It is
// idempotent and nil-safe; the caller keeps the file path.
func (l *RunLog) Close() {
    if l == nil || l.closed.Swap(true) {
        return
    }
    close(l.done)
    l.wg.Wait()
}
