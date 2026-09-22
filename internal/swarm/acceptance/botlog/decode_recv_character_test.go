// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package botlog_test

import (
    "encoding/binary"
    "math"
    "strings"
    "testing"

    fromgameserver "github.com/melg8/swarm/internal/swarm/packets/from_game_server"
    "github.com/melg8/swarm/internal/swarm/packets/packet"
)

// The character wire format round of issue #9: the recv decoders of
// the session handover (the char select list, the char selected
// handover, the char info spawn, the user info self state) and the
// inventory list packets run under hand built payloads - the heaviest
// wire shapes the log renders, built the same way the packet package
// tests build theirs (the skipped filler groups stay zero bytes, the
// real fields carry the values the rendered lines assert).

// appendInt16 appends a little endian int16 value to the packet.
func appendInt16(dst []byte, value int16) []byte {
    var buf [2]byte
    binary.LittleEndian.PutUint16(buf[:], uint16(value))

    return append(dst, buf[:]...)
}

// appendFloat64 appends a little endian float64 value to the packet.
func appendFloat64(dst []byte, value float64) []byte {
    var buf [8]byte
    binary.LittleEndian.PutUint64(buf[:], math.Float64bits(value))

    return append(dst, buf[:]...)
}

// appendItemEntry appends one 28 byte inventory item entry (the
// AbstractItemPacket.writeItem shape the inventory packets repeat
// per stack: the type pair, the ids and count, the equipped flag,
// the body part mask and the enchant level).
func appendItemEntry(
    dst []byte, objectID int32, itemID int32, count int32,
    equipped int16, enchant int16,
) []byte {
    dst = appendInt16(dst, 4) // the item type of the template
    dst = appendInt32(dst, objectID)
    dst = appendInt32(dst, itemID)
    dst = appendInt32(dst, count)
    dst = appendInt16(dst, 5)        // the inventory slot class
    dst = append(dst, 0, 0)          // the custom type1 field
    dst = appendInt16(dst, equipped) // the equipped flag
    dst = appendInt32(dst, 0)        // the body part mask
    dst = appendInt16(dst, enchant)  // the enchant level
    dst = append(dst, 0, 0)          // the custom type2 field

    return dst
}

// buildCharInfoPayload builds one player spawn payload (the wire
// shape of CharInfo.writeImpl up to the state flags: the position,
// the identity block, the speed block with its skipped groups, the
// collision pair, the title and the clan tail). The flags slice is
// the seven state bytes the parser reads after the clan tail.
func buildCharInfoPayload(
    objectID int32, name string, title string, flags [7]byte,
) []byte {
    data := []byte{0x03}
    data = appendInt32(data, 45200) // x
    data = appendInt32(data, 50200) // y
    data = appendInt32(data, -3480) // z
    data = appendInt32(data, 0)     // the vehicle id
    data = appendInt32(data, objectID)
    data = append(data, utf16BytesOf(name)...)
    data = appendInt32(data, 0)  // the race: human
    data = appendInt32(data, 0)  // the sex: male
    data = appendInt32(data, 10) // the class: elven mystic
    // The 12 paperdoll ints, the attack speed pair, the pvp flag and
    // the karma between the class and the run speed.
    data = append(data, make([]byte, 64)...)
    data = appendInt32(data, 150) // the run speed
    data = appendInt32(data, 75)  // the walk speed
    // The two swim and the four fly speed ints (the fly pair written
    // twice by the server).
    data = append(data, make([]byte, 24)...)
    data = appendFloat64(data, 1.0)         // the move multiplier
    data = append(data, make([]byte, 8)...) // the attack multiplier
    data = appendFloat64(data, 9)           // the collision radius
    // The collision height and the hair and face ints.
    data = append(data, make([]byte, 20)...)
    data = append(data, utf16BytesOf(title)...)
    // The clan id, the ally id and the relation.
    data = append(data, make([]byte, 20)...)

    return append(data, flags[:]...)
}

func TestRecvDecodeCharInfoRendersThePlayer(t *testing.T) {
    t.Run("running player", func(t *testing.T) {
        var flags [7]byte
        flags[0] = 1 // standing
        flags[1] = 1 // running
        data := buildCharInfoPayload(2001, "Neighbor", "Duelist", flags)

        body := recvLinesOf(t, data)
        want := `player "Neighbor" (object 2001) at (45200,50200,-3480)` +
            " running standing"
        if !strings.Contains(body, want) {
            t.Errorf("the player line misses the running spawn: %s", body)
        }
    })

    t.Run("dead player", func(t *testing.T) {
        var flags [7]byte
        flags[3] = 1 // dead (standing, running stay zero)
        data := buildCharInfoPayload(2002, "Fallen", "", flags)

        body := recvLinesOf(t, data)
        if !strings.Contains(body, `player "Fallen" (object 2002)`) {
            t.Errorf("the player line misses the identity: %s", body)
        }
        if !strings.Contains(body, " dead") {
            t.Errorf("the player line misses the dead flag: %s", body)
        }
    })
}

// buildUserInfoPayload builds one self state payload (the wire shape
// of UserInfo.writeImpl up to the move multiplier: the identity, the
// stat block, the load pair, the paperdoll blocks with their skipped
// display ids and combat stats, the speed pair and the multiplier).
func buildUserInfoPayload() []byte {
    data := []byte{0x04}
    data = appendInt32(data, 45000) // x
    data = appendInt32(data, 50000) // y
    data = appendInt32(data, -3500) // z
    data = appendInt32(data, 0)     // the vehicle id
    data = appendInt32(data, 100)   // the object id
    data = append(data, utf16BytesOf("unittest1")...)
    data = appendInt32(data, 1)       // the race: elf
    data = appendInt32(data, 0)       // the sex
    data = appendInt32(data, 18)      // the base class
    data = appendInt32(data, 9)       // the level
    data = appendInt32(data, 2000)    // the exp
    data = appendInt32(data, 36)      // str
    data = appendInt32(data, 35)      // dex
    data = appendInt32(data, 36)      // con
    data = appendInt32(data, 23)      // int
    data = appendInt32(data, 14)      // wit
    data = appendInt32(data, 25)      // men
    data = appendInt32(data, 122)     // the max hp
    data = appendInt32(data, 100)     // the current hp
    data = appendInt32(data, 40)      // the max mp
    data = appendInt32(data, 39)      // the current mp
    data = appendInt32(data, 10)      // the sp
    data = appendInt32(data, 640)     // the current load
    data = appendInt32(data, 6400000) // the max load
    data = appendInt32(data, 0)       // the weapon flag
    // The 15 paperdoll object ids (all slots empty), then the 15
    // display ids and the 12 combat stat ints the parser skips.
    data = append(data, make([]byte, 15*4+15*4+12*4)...)
    data = appendInt32(data, 165)            // the run speed
    data = appendInt32(data, 80)             // the walk speed
    data = append(data, make([]byte, 24)...) // the swim and fly speeds
    data = appendFloat64(data, 1.1)          // the move multiplier

    return data
}

func TestRecvDecodeUserInfoRendersTheSelfState(t *testing.T) {
    body := recvLinesOf(t, buildUserInfoPayload())
    want := `self "unittest1" (object 100) level 9 hp 100/122 mp 39/40` +
        " exp 2000 sp 10 load 640/6400000"
    if !strings.Contains(body, want) {
        t.Errorf("the self line misses the vitals and the load: %s", body)
    }
}

// buildCharSelectInfoPayload serializes a two character account list
// through the exported ToBytes serializer of the packet package (the
// emulated game server writes the same wire the live server does).
func buildCharSelectInfoPayload(t *testing.T) []byte {
    t.Helper()
    list := fromgameserver.NewCharSelectInfoPacket()
    list.Count = 2
    list.Characters = []fromgameserver.CharacterInfo{
        {
            Name:        "proxybot",
            ObjectID:    1055,
            Account:     "unittest1",
            SessionID:   0,
            ClanID:      0,
            Sex:         0,
            Race:        1,
            BaseClassID: 18,
            X:           45000,
            Y:           50000,
            Z:           -3500,
            CurrentHP:   85.0,
            CurrentMP:   30.0,
            Level:       9,
            HairStyle:   0,
            HairColor:   1,
            Face:        2,
            MaxHP:       100.5,
            MaxMP:       40.5,
            DeleteTimer: 0,
        },
        {
            Name:        "farmbot",
            ObjectID:    1056,
            Account:     "unittest1",
            SessionID:   0,
            ClanID:      0,
            Sex:         1,
            Race:        1,
            BaseClassID: 18,
            X:           45100,
            Y:           50100,
            Z:           -3460,
            CurrentHP:   50.0,
            CurrentMP:   12.0,
            Level:       3,
            HairStyle:   0,
            HairColor:   0,
            Face:        0,
            MaxHP:       72.0,
            MaxMP:       28.0,
            DeleteTimer: 0,
        },
    }
    writer := packet.NewWriter()
    if err := list.ToBytes(writer); err != nil {
        t.Fatalf("serialize: %v", err)
    }

    return writer.Bytes()
}

func TestRecvDecodeCharSelectInfoRendersTheList(t *testing.T) {
    body := recvLinesOf(t, buildCharSelectInfoPayload(t))
    if !strings.Contains(body, "character list: 2 characters") {
        t.Errorf("the list line misses the count: %s", body)
    }
    want := `  slot 0: "proxybot" level 9 at (45000,50000,-3500)`
    if !strings.Contains(body, want) {
        t.Errorf("the list misses the first character: %s", body)
    }
    want = `  slot 1: "farmbot" level 3 at (45100,50100,-3460)`
    if !strings.Contains(body, want) {
        t.Errorf("the list misses the second character: %s", body)
    }
}

func TestRecvDecodeCharSelectedRendersTheHandover(t *testing.T) {
    data := []byte{0x21}
    data = append(data, utf16BytesOf("unittest1")...)
    data = appendInt32(data, 1055)           // the object id
    data = append(data, utf16BytesOf("")...) // the empty title
    data = appendInt32(data, 7)              // the session id
    // The clan id, the unknown int, the sex and the race.
    data = append(data, make([]byte, 16)...)
    data = appendInt32(data, 18)     // the class
    data = appendInt32(data, 0)      // the active flag
    data = appendInt32(data, 45000)  // x
    data = appendInt32(data, 50000)  // y
    data = appendInt32(data, -3500)  // z
    data = appendFloat64(data, 85.0) // the current hp
    data = appendFloat64(data, 30.0) // the current mp

    body := recvLinesOf(t, data)
    want := `playing "unittest1" (object 1055, class 18)` +
        " at (45000,50000,-3500) hp 85 mp 30"
    if !strings.Contains(body, want) {
        t.Errorf("the handover line misses the playing state: %s", body)
    }
}

func TestRecvDecodeItemListRendersTheInventory(t *testing.T) {
    data := []byte{0x27}
    data = appendInt16(data, 1) // the show window flag
    data = appendInt16(data, 2) // two stacks
    // The adena pile: not equipped, no enchant.
    data = appendItemEntry(data, 3001, 57, 1000000, 0, 0)
    // The equipped enchanted dagger of the right hand.
    data = appendItemEntry(data, 3002, 10, 1, 1, 3)

    body := recvLinesOf(t, data)
    if !strings.Contains(body, "inventory list: 2 items") {
        t.Errorf("the list line misses the count: %s", body)
    }
    want := `  item "Adena" (item 57, object 3001) x1000000`
    if !strings.Contains(body, want) {
        t.Errorf("the list misses the adena stack: %s", body)
    }
    want = `  item "Dagger" (item 10, object 3002) x1 equipped +3`
    if !strings.Contains(body, want) {
        t.Errorf("the list misses the equipped enchant: %s", body)
    }
}

func TestRecvDecodeInventoryUpdateRendersTheDelta(t *testing.T) {
    data := []byte{0x37}
    data = appendInt16(data, 3) // three changes
    data = appendInt16(data, 1) // add
    data = appendItemEntry(data, 3003, 57, 500, 0, 0)
    data = appendInt16(data, 2) // modify
    data = appendItemEntry(data, 3004, 17, 250, 0, 0)
    data = appendInt16(data, 3) // remove
    data = appendItemEntry(data, 3005, 10, 1, 0, 0)

    body := recvLinesOf(t, data)
    if !strings.Contains(body, "inventory update: 3 changes") {
        t.Errorf("the update line misses the count: %s", body)
    }
    want := `  add "Adena" (item 57, object 3003) x500`
    if !strings.Contains(body, want) {
        t.Errorf("the update misses the add verb: %s", body)
    }
    want = `  modify "Wooden Arrow" (item 17, object 3004) x250`
    if !strings.Contains(body, want) {
        t.Errorf("the update misses the modify verb: %s", body)
    }
    want = `  remove "Dagger" (item 10, object 3005) x1`
    if !strings.Contains(body, want) {
        t.Errorf("the update misses the remove verb: %s", body)
    }
}
