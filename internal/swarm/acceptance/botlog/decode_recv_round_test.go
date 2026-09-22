// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package botlog_test

import (
    "strings"
    "testing"

    "github.com/melg8/swarm/internal/swarm/acceptance/botlog"
)

// The decoder round of issue #9: the recv decoders the live runs
// exercise (the movement family, the target family, the item
// ground flow, the status update and the npc dialog) run under
// hand built packets - the same wire formats the connection
// harness tests pin, asserted through the rendered log lines.

// recvLinesOf opens a botlog in a temp dir, feeds the packet and
// returns the rendered body.
func recvLinesOf(t *testing.T, packet []byte) string {
    t.Helper()
    log, err := botlog.Open(t.TempDir(), botlog.Header{TestID: "t"})
    if err != nil {
        t.Fatalf("open: %v", err)
    }
    log.Recv(packet)
    log.Close()

    return readLog(t, log.Path())
}

func TestRecvDecodeMoveToLocation(t *testing.T) {
    data := []byte{0x01}
    data = appendInt32(data, 42) // object id
    data = appendInt32(data, 46000)
    data = appendInt32(data, 51000)
    data = appendInt32(data, -3500) // destination
    data = appendInt32(data, 45000)
    data = appendInt32(data, 50000)
    data = appendInt32(data, -3400) // current

    body := recvLinesOf(t, data)
    if !strings.Contains(body, "object 42 moves to") {
        t.Error("the move line misses the object")
    }
    if !strings.Contains(body, "(46000,51000,-3500)") {
        t.Error("the move line misses the destination")
    }
    if !strings.Contains(body, "from (45000,50000,-3400)") {
        t.Error("the move line misses the origin")
    }
}

func TestRecvDecodeTargetFamily(t *testing.T) {
    t.Run("target selected", func(t *testing.T) {
        data := []byte{0x39}
        data = appendInt32(data, 7)  // object id
        data = appendInt32(data, 8)  // target id
        data = appendInt32(data, 10) // x
        data = appendInt32(data, 20) // y
        data = appendInt32(data, 30) // z
        data = appendInt32(data, 0)  // unknown

        body := recvLinesOf(t, data)
        if !strings.Contains(body, "object 7 targeted object 8") {
            t.Error("the target selected line is wrong")
        }
    })

    t.Run("target unselected", func(t *testing.T) {
        data := []byte{0x3A}
        data = appendInt32(data, 7)
        data = appendInt32(data, 10)
        data = appendInt32(data, 20)
        data = appendInt32(data, 30)
        data = appendInt32(data, 0)
        data = appendInt32(data, 0)

        body := recvLinesOf(t, data)
        if !strings.Contains(body, "object 7 dropped its target") {
            t.Error("the target unselected line is wrong")
        }
    })

    t.Run("my target selected", func(t *testing.T) {
        data := []byte{0xBF}
        data = appendInt32(data, 8)
        data = append(data, 0, 0) // the target color

        body := recvLinesOf(t, data)
        if !strings.Contains(body, "own target is object 8") {
            t.Error("the my target selected line is wrong")
        }
    })
}

func TestRecvDecodeStatusUpdate(t *testing.T) {
    data := []byte{0x1A}
    data = appendInt32(data, 100) // the played character
    data = appendInt32(data, 2)   // two attributes
    data = appendInt32(data, 9)   // cur hp
    data = appendInt32(data, 87)
    data = appendInt32(data, 12) // max mp
    data = appendInt32(data, 30)

    body := recvLinesOf(t, data)
    if !strings.Contains(body, "object 100 status:") {
        t.Error("the status line misses the object")
    }
    if !strings.Contains(body, "curHp=87") {
        t.Error("the status line misses the hp attribute")
    }
    if !strings.Contains(body, "maxMp=30") {
        t.Error("the status line misses the mp attribute")
    }
}

func TestRecvDecodeGroundItems(t *testing.T) {
    t.Run("spawn item", func(t *testing.T) {
        data := []byte{0x15}
        data = appendInt32(data, 200) // object id
        data = appendInt32(data, 57)  // adena
        data = appendInt32(data, 45200)
        data = appendInt32(data, 50200)
        data = appendInt32(data, -3480)
        data = appendInt32(data, 1)  // stackable
        data = appendInt32(data, 25) // count

        body := recvLinesOf(t, data)
        if !strings.Contains(body, `item "Adena" (object 200) x25`) {
            t.Error("the spawn item line is wrong")
        }
        if !strings.Contains(body, "appeared at") {
            t.Error("the spawn item line misses the position")
        }
    })

    t.Run("drop item", func(t *testing.T) {
        data := []byte{0x16}
        data = appendInt32(data, 100) // dropper
        data = appendInt32(data, 201) // object id
        data = appendInt32(data, 57)  // adena
        data = appendInt32(data, 45300)
        data = appendInt32(data, 50300)
        data = appendInt32(data, -3480)
        data = appendInt32(data, 1)  // stackable
        data = appendInt32(data, 10) // count
        data = appendInt32(data, 0)  // unknown

        body := recvLinesOf(t, data)
        if !strings.Contains(body, "dropped") {
            t.Error("the drop item line misses the verb")
        }
    })

    t.Run("get item", func(t *testing.T) {
        data := []byte{0x17}
        data = appendInt32(data, 100) // player
        data = appendInt32(data, 202) // object id
        data = appendInt32(data, 45400)
        data = appendInt32(data, 50400)
        data = appendInt32(data, -3480)

        body := recvLinesOf(t, data)
        if !strings.Contains(body, "picked up") {
            t.Error("the get item line misses the verb")
        }
    })

    t.Run("delete object", func(t *testing.T) {
        data := []byte{0x1E}
        data = appendInt32(data, 42)

        body := recvLinesOf(t, data)
        if !strings.Contains(body, "object 42 removed") {
            t.Error("the delete object line is wrong")
        }
    })
}

func TestRecvDecodeNpcHTMLRendersTheDialog(t *testing.T) {
    html := "<html>Hello.</html>"
    data := []byte{0x1B}
    data = appendInt32(data, 7155)
    data = append(data, utf16BytesOf(html)...)
    data = appendInt32(data, 0)

    body := recvLinesOf(t, data)
    if !strings.Contains(body, "npc 7155 dialog") {
        t.Error("the npc html line misses the npc")
    }
    if !strings.Contains(body, "links") {
        t.Error("the npc html line misses the link count")
    }
}

func TestRecvDecodeSocialTeleportAndWaitType(t *testing.T) {
    t.Run("social action", func(t *testing.T) {
        data := []byte{0x3D}
        data = appendInt32(data, 7)
        data = appendInt32(data, 2) // the laugh

        body := recvLinesOf(t, data)
        if !strings.Contains(body, "object 7 social") {
            t.Error("the social line misses the object")
        }
    })

    t.Run("teleport", func(t *testing.T) {
        data := []byte{0x38}
        data = appendInt32(data, 42)
        data = appendInt32(data, 46000)
        data = appendInt32(data, 51000)
        data = appendInt32(data, -3500)
        data = appendInt32(data, 0)  // fade
        data = appendInt32(data, 90) // heading

        body := recvLinesOf(t, data)
        if !strings.Contains(body, "object 42 teleported to") {
            t.Error("the teleport line is wrong")
        }
        if !strings.Contains(body, "(46000,51000,-3500)") {
            t.Error("the teleport line misses the position")
        }
    })

    t.Run("change wait type", func(t *testing.T) {
        data := []byte{0x3F}
        data = appendInt32(data, 100)
        data = appendInt32(data, 0) // sitting
        data = appendInt32(data, 45000)
        data = appendInt32(data, 50000)
        data = appendInt32(data, -3500)

        body := recvLinesOf(t, data)
        if !strings.Contains(body, "object 100 is sitting") {
            t.Error("the wait type line misses the sitting stance")
        }
    })
}

// TestRecvDecodeTruncatedPacketsFallBackToShortLines walks the
// dispatch table with one byte packets: every decoder answers its
// short fallback line instead of panicking on the missing fields.
func TestRecvDecodeTruncatedPacketsFallBackToShortLines(t *testing.T) {
    cases := map[byte]string{
        0x01: "move to location",
        0x06: "attack",
        0x15: "spawn item",
        0x16: "drop item",
        0x17: "get item",
        0x1B: "npc html",
        0x1E: "delete object",
        0x27: "item list",
        0x37: "inventory update",
        0x39: "target selected",
        0x3A: "target unselected",
        0xBF: "my target selected",
    }
    for opcode, want := range cases {
        body := recvLinesOf(t, []byte{opcode})
        if !strings.Contains(body, want) {
            t.Errorf("opcode %#x must fall back to %q, got: %s",
                opcode, want, body)
        }
    }
}

// utf16BytesOf encodes the string as null terminated UTF-16LE (the
// packet string format the decoders read).
func utf16BytesOf(value string) []byte {
    result := make([]byte, 0, len(value)*2+2)
    for _, r := range value {
        result = append(result, byte(r), byte(r>>8))
    }

    return append(result, 0, 0)
}
