// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package botlog_test

import (
    "strings"
    "testing"
)

// The world broadcast round of issue #9: the remaining recv
// decoders of the known list (the stance and rotation family, the
// chase and placement pair, the skill book, the buff bar and the
// long link trimming of the dialog pages) run under hand built
// packets - the last uncovered branches of the recv dispatch.

func TestRecvDecodeStanceAndRotationFamily(t *testing.T) {
    t.Run("run stance", func(t *testing.T) {
        data := []byte{0x3E}
        data = appendInt32(data, 20538) // the object id
        data = appendInt32(data, 1)     // the move type: run

        body := recvLinesOf(t, data)
        if !strings.Contains(body, "object 20538 switches to run") {
            t.Errorf("the stance line misses the run word: %s", body)
        }
    })

    t.Run("walk stance", func(t *testing.T) {
        data := []byte{0x3E}
        data = appendInt32(data, 20538)
        data = appendInt32(data, 0) // the move type: walk

        body := recvLinesOf(t, data)
        if !strings.Contains(body, "object 20538 switches to walk") {
            t.Errorf("the stance line misses the walk word: %s", body)
        }
    })

    t.Run("stop move", func(t *testing.T) {
        data := []byte{0x59}
        data = appendInt32(data, 42)    // the object id
        data = appendInt32(data, 45000) // x
        data = appendInt32(data, 50000) // y
        data = appendInt32(data, -3500) // z
        data = appendInt32(data, 16384) // the heading

        body := recvLinesOf(t, data)
        want := "object 42 stopped at (45000,50000,-3500) heading 16384"
        if !strings.Contains(body, want) {
            t.Errorf("the stop line misses the placement: %s", body)
        }
    })

    t.Run("begin rotation", func(t *testing.T) {
        data := []byte{0x77}
        data = appendInt32(data, 42)   // the object id
        data = appendInt32(data, 8192) // the heading
        data = appendInt32(data, 0)    // the side
        data = appendInt32(data, 20)   // the speed

        body := recvLinesOf(t, data)
        if !strings.Contains(body, "begin rotation: object 42 heading 8192") {
            t.Errorf("the rotation line misses the heading: %s", body)
        }
    })

    t.Run("stop rotation", func(t *testing.T) {
        // The stop wire is two ints shorter than the begin wire: it
        // ends with the speed int and one unknown byte.
        data := []byte{0x78}
        data = appendInt32(data, 42)   // the object id
        data = appendInt32(data, 8192) // the heading
        data = appendInt32(data, 20)   // the speed
        data = append(data, 0)         // the unknown byte

        body := recvLinesOf(t, data)
        if !strings.Contains(body, "stop rotation: object 42 heading 8192") {
            t.Errorf("the rotation line misses the heading: %s", body)
        }
    })

    t.Run("auto attack start and stop", func(t *testing.T) {
        start := []byte{0x3B}
        start = appendInt32(start, 20538)
        body := recvLinesOf(t, start)
        if !strings.Contains(body, "object 20538 starts the auto attack") {
            t.Errorf("the attack start line is wrong: %s", body)
        }

        stop := []byte{0x3C}
        stop = appendInt32(stop, 20538)
        body = recvLinesOf(t, stop)
        if !strings.Contains(body, "object 20538 stops the auto attack") {
            t.Errorf("the attack stop line is wrong: %s", body)
        }
    })
}

func TestRecvDecodeChaseAndPlacement(t *testing.T) {
    t.Run("move to pawn", func(t *testing.T) {
        data := []byte{0x75}
        data = appendInt32(data, 42)    // the chaser
        data = appendInt32(data, 20538) // the target
        data = appendInt32(data, 120)   // the distance
        data = appendInt32(data, 45000) // x
        data = appendInt32(data, 50000) // y
        data = appendInt32(data, -3500) // z
        data = appendInt32(data, 45200) // the target x
        data = appendInt32(data, 50100) // the target y
        data = appendInt32(data, -3480) // the target z

        body := recvLinesOf(t, data)
        want := "object 42 chases object 20538 at distance 120" +
            " from (45000,50000,-3500)"
        if !strings.Contains(body, want) {
            t.Errorf("the chase line misses the pair: %s", body)
        }
    })

    t.Run("validate location", func(t *testing.T) {
        data := []byte{0x76}
        data = appendInt32(data, 42)    // the object id
        data = appendInt32(data, 45100) // x
        data = appendInt32(data, 50100) // y
        data = appendInt32(data, -3480) // z
        data = appendInt32(data, 32768) // the heading

        body := recvLinesOf(t, data)
        want := "object 42 placed at (45100,50100,-3480) heading 32768"
        if !strings.Contains(body, want) {
            t.Errorf("the placement line misses the heading: %s", body)
        }
    })
}

func TestRecvDecodeSkillListRendersTheBook(t *testing.T) {
    data := []byte{0x6D}
    data = appendInt32(data, 2) // two skills
    // The passive Long Shot of the archers.
    data = appendInt32(data, 1)   // the passive flag
    data = appendInt32(data, 1)   // the level
    data = appendInt32(data, 113) // the skill id
    // The active Power Strike level 3.
    data = appendInt32(data, 0) // the passive flag
    data = appendInt32(data, 3) // the level
    data = appendInt32(data, 3) // the skill id

    body := recvLinesOf(t, data)
    if !strings.Contains(body, "skill list: 2 skills") {
        t.Errorf("the book line misses the count: %s", body)
    }
    want := "  skill Long Shot (id 113) level 1 passive"
    if !strings.Contains(body, want) {
        t.Errorf("the book misses the passive entry: %s", body)
    }
    want = "  skill Power Strike (id 3) level 3"
    if !strings.Contains(body, want) {
        t.Errorf("the book misses the active entry: %s", body)
    }
}

func TestRecvDecodeAbnormalStatusRendersTheBuffs(t *testing.T) {
    data := []byte{0x97}
    data = appendInt16(data, 2) // two effects
    // The Wind Strike cast on the bot.
    data = appendInt32(data, 1177) // the skill id
    data = appendInt16(data, 1)    // the level
    data = appendInt32(data, 30)   // the remaining time
    // The self heal of the rest cycle.
    data = appendInt32(data, 1216) // the skill id
    data = appendInt16(data, 2)    // the level
    data = appendInt32(data, 40)   // the remaining time

    body := recvLinesOf(t, data)
    want := "effects: Wind Strike (id 1177) level 1" +
        " Self Heal (id 1216) level 2"
    if !strings.Contains(body, want) {
        t.Errorf("the effects line misses the buff pair: %s", body)
    }
}

func TestRecvDecodeNpcHTMLTrimsLongLinks(t *testing.T) {
    command := "bypass -h menu_select?ask=-303&reply=" +
        strings.Repeat("a", 100)
    text := strings.Repeat("t", 60)
    html := `<html><a action="` + command + `">` + text + `</a></html>`
    data := []byte{0x1B}
    data = appendInt32(data, 7155)
    data = append(data, utf16BytesOf(html)...)
    data = appendInt32(data, 0)

    body := recvLinesOf(t, data)
    if !strings.Contains(body, "1 links") {
        t.Errorf("the dialog line misses the link count: %s", body)
    }
    // The command renders at most 83 characters (the parser strips
    // the "-h " prefix, then trimText keeps 80 + "..."), the link
    // text at most 51 (48 kept + "...").
    want := `link "menu_select?ask=-303&reply=` +
        strings.Repeat("a", 53) + `..." text "` +
        strings.Repeat("t", 48) + `..."`
    if !strings.Contains(body, want) {
        t.Errorf("the link line misses the trimmed pair: %s", body)
    }
}
