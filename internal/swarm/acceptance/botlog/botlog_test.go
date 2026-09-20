// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package botlog_test

import (
    "encoding/binary"
    "os"
    "path/filepath"
    "strings"
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/acceptance/botlog"
    "github.com/melg8/swarm/internal/swarm/packets/packet"
    togameserver "github.com/melg8/swarm/internal/swarm/packets/to_game_server"
)

// openTestLog opens a run log in a temp directory with the given
// header, closes it and answers the path: the writer goroutine
// flushes on close, so the assertions read the settled file.
func openTestLog(t *testing.T, head botlog.Header) string {
    t.Helper()
    dir := t.TempDir()
    log, err := botlog.Open(dir, head)
    if err != nil {
        t.Fatalf("open: %v", err)
    }
    log.Close()

    return log.Path()
}

// readLog reads the whole run log file.
func readLog(t *testing.T, path string) string {
    t.Helper()
    data, err := os.ReadFile(path)
    if err != nil {
        t.Fatalf("read: %v", err)
    }

    return string(data)
}

// TestHeaderCarriesTheContract verifies the identity, description,
// check and environment blocks of the file top: the file must tell
// what it tests and how the run passes or fails without any other
// artifact.
func TestHeaderCarriesTheContract(t *testing.T) {
    head := botlog.Header{
        TestID:     "farm-readiness",
        Title:      "farm readiness - the level 15 walk",
        Account:    "temp1",
        Generation: 3,
        Timeout:    20 * time.Minute,
        Description: "Start: the elven fighter wakes at the spawn. " +
            "Flow: it shops. Pass: it kills.",
        Checks: []botlog.CheckDesc{
            {ID: "online", Label: "entered the world",
                Done: false, Detail: ""},
            {ID: "kill", Label: "killed a mob", Done: false, Detail: ""},
        },
        Environment: []botlog.EnvLine{
            {Key: "login server", Value: "127.0.0.1:2106"},
        },
        Args: "./swarm -acceptance farm-readiness",
    }
    body := readLog(t, openTestLog(t, head))
    wants := []string{
        "swarm acceptance run log",
        "test      : farm-readiness - farm readiness",
        "account   : temp1",
        "run       : generation 3",
        "timeout   : 20m0s",
        "args      : ./swarm -acceptance farm-readiness",
        "Start: the elven fighter wakes at the spawn",
        "the scenario publishes its check list at the run start",
        "login server: 127.0.0.1:2106",
        "the timeout (20m0s) or a cancel",
    }
    for _, want := range wants {
        if !strings.Contains(body, want) {
            t.Errorf("the header misses %q", want)
        }
    }
}

// TestNilLogIsSilent verifies every emit method is a no-op on a nil
// log (the disabled log directory must not crash the runner).
func TestNilLogIsSilent(t *testing.T) {
    var log *botlog.RunLog
    log.Event("nothing")
    log.Hunt("Hunt: nothing")
    log.DB("SELECT nothing")
    log.Session("nothing")
    log.Recv([]byte{0x01})
    log.Send([]byte{0x01})
    log.Sample("nothing")
    log.CheckTransition("online", "entered", true, "done")
    log.Verdict(true, "", nil, nil)
    log.Close()
    if log.Path() != "" {
        t.Error("the nil log has a path")
    }
}

// buildMoveRequest serializes one walk click for the send decoder.
func buildMoveRequest(t *testing.T) []byte {
    t.Helper()
    request := togameserver.NewMoveToLocationRequestPacket()
    request.TargetX = 45678
    request.TargetY = 51234
    request.TargetZ = -3456
    request.OriginX = 45123
    request.OriginY = 50123
    request.OriginZ = -3400
    writer := packet.NewWriter()
    if err := request.ToBytes(writer); err != nil {
        t.Fatalf("serialize: %v", err)
    }

    return writer.Bytes()
}

// TestSendDecodeMoveClick verifies the walk click line: the
// destination, the origin and the mouse mode.
func TestSendDecodeMoveClick(t *testing.T) {
    dir := t.TempDir()
    log, err := botlog.Open(dir, botlog.Header{TestID: "t"})
    if err != nil {
        t.Fatalf("open: %v", err)
    }
    log.Send(buildMoveRequest(t))
    log.Close()
    body := readLog(t, log.Path())
    want := "move to location dest=(45678,51234,-3456) " +
        "origin=(45123,50123,-3400) mode=mouse"
    if !strings.Contains(body, want) {
        t.Errorf("the send line misses the click: %q", want)
    }
    if !strings.Contains(body, "[send 0x01 MoveToLocation]") {
        t.Error("the send tag misses the opcode name")
    }
}

// appendInt32 appends a little endian int32 value to the packet.
func appendInt32(dst []byte, value int32) []byte {
    var buf [4]byte
    binary.LittleEndian.PutUint32(buf[:], uint32(value))

    return append(dst, buf[:]...)
}

// utf16Bytes renders a null-terminated UTF-16LE string.
func utf16Bytes(value string) []byte {
    out := make([]byte, 0, len(value)*2+2)
    for _, r := range value {
        out = append(out, byte(r), byte(r>>8))
    }

    return append(out, 0, 0)
}

// buildNpcInfo builds one npc spawn payload for the recv decoder
// (the wire shape of AbstractNpcInfo.writeImpl).
func buildNpcInfo(t *testing.T) []byte {
    t.Helper()
    data := []byte{0x22}
    data = appendInt32(data, 268501317)
    data = appendInt32(data, 20538)
    data = appendInt32(data, 1)
    data = appendInt32(data, 43896)
    data = appendInt32(data, 54088)
    data = appendInt32(data, -3640)
    data = appendInt32(data, 0)
    data = append(data, make([]byte, 88)...)
    data = append(data, 1, 1, 0, 0, 0)
    data = append(data, utf16Bytes("Kaboo Orc")...)
    data = append(data, utf16Bytes("")...)

    return data
}

// TestRecvDecodeNpcInfo verifies the npc spawn line: the name, the
// position and the attackable flag.
func TestRecvDecodeNpcInfo(t *testing.T) {
    dir := t.TempDir()
    log, err := botlog.Open(dir, botlog.Header{TestID: "t"})
    if err != nil {
        t.Fatalf("open: %v", err)
    }
    log.Recv(buildNpcInfo(t))
    log.Close()
    body := readLog(t, log.Path())
    if !strings.Contains(body, `npc "Kaboo Orc"`) {
        t.Error("the recv line misses the npc name")
    }
    if !strings.Contains(body, "(43896,54088,-3640)") {
        t.Error("the recv line misses the position")
    }
    if !strings.Contains(body, "attackable") {
        t.Error("the recv line misses the attackable flag")
    }
    if !strings.Contains(body, "[recv 0x22 NpcInfo]") {
        t.Error("the recv tag misses the opcode name")
    }
}

// TestUnknownPacketFallsBackToHex verifies the unrecognized opcode
// keeps a readable fallback instead of an empty line.
func TestUnknownPacketFallsBackToHex(t *testing.T) {
    dir := t.TempDir()
    log, err := botlog.Open(dir, botlog.Header{TestID: "t"})
    if err != nil {
        t.Fatalf("open: %v", err)
    }
    log.Recv([]byte{0xFE, 0x01, 0x02})
    log.Close()
    body := readLog(t, log.Path())
    if !strings.Contains(body, "unknown recv packet, 3 bytes") {
        t.Error("the unknown fallback line is missing")
    }
}

// TestVerdictBlock verifies the outcome block: the verdict, the
// final check list and the packet counters.
func TestVerdictBlock(t *testing.T) {
    dir := t.TempDir()
    log, err := botlog.Open(dir, botlog.Header{TestID: "t"})
    if err != nil {
        t.Fatalf("open: %v", err)
    }
    log.Recv(buildNpcInfo(t))
    log.Send(buildMoveRequest(t))
    log.Sample("state status=online phase=engage")
    log.CheckTransition("kill", "killed a mob", true,
        "killed at 44000,54000")
    log.Verdict(true, "", []botlog.CheckDesc{
        {ID: "kill", Label: "killed a mob", Done: true,
            Detail: "killed at 44000,54000"},
    }, nil)
    log.Close()
    body := readLog(t, log.Path())
    wants := []string{
        "[verdict] PASSED after",
        "[x] kill (killed a mob): killed at 44000,54000",
        "checks: 1 of 1 done",
        "packets: 1 received, 1 sent, 0 log lines dropped",
        "[check] completed kill",
        "[sample] state status=online",
    }
    for _, want := range wants {
        if !strings.Contains(body, want) {
            t.Errorf("the verdict block misses %q", want)
        }
    }
}

// TestFileNamesCarryTheRun verifies the file name tells the test,
// the time and the process so a directory listing sorts the runs.
func TestFileNamesCarryTheRun(t *testing.T) {
    dir := t.TempDir()
    log, err := botlog.Open(dir, botlog.Header{TestID: "church entry"})
    if err != nil {
        t.Fatalf("open: %v", err)
    }
    log.Close()
    name := filepath.Base(log.Path())
    if !strings.HasPrefix(name, "church-entry-") {
        t.Errorf("the file name misses the test: %s", name)
    }
    if !strings.HasSuffix(name, ".log") {
        t.Errorf("the file name misses the extension: %s", name)
    }
}

// TestCloseIsIdempotent verifies a double close does not panic (the
// manager close path and the cleanup path may race).
func TestCloseIsIdempotent(t *testing.T) {
    dir := t.TempDir()
    log, err := botlog.Open(dir, botlog.Header{TestID: "t"})
    if err != nil {
        t.Fatalf("open: %v", err)
    }
    log.Close()
    log.Close()
}
