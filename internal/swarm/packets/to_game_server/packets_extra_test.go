// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
    "testing"

    "github.com/melg8/swarm/internal/swarm/packets/packet"
    "github.com/stretchr/testify/require"
)

// TestCharacterCreatePacketLayout pins the full wire form of the
// creation request: the 0x0B opcode, the null terminated UTF-16LE
// name and the twelve fixed int32 fields in the readImpl order
// (race, female, classId, int, str, con, men, dex, wit, hairStyle,
// hairColor, face).
func TestCharacterCreatePacketLayout(t *testing.T) {
    request := &CharacterCreate{
        Name:      "Melg",
        Race:      1,
        Female:    1,
        ClassID:   18,
        INT:       21,
        STR:       22,
        CON:       23,
        MEN:       24,
        DEX:       25,
        WIT:       26,
        HairStyle: 1,
        HairColor: 2,
        Face:      3,
    }
    writer := packet.NewWriter()
    require.NoError(t, request.ToBytes(writer))
    require.Equal(t, []byte{
        0x0B,
        // "Melg" as UTF-16LE plus the two byte null terminator.
        0x4D, 0x00, 0x65, 0x00, 0x6C, 0x00, 0x67, 0x00, 0x00, 0x00,
        0x01, 0x00, 0x00, 0x00, // race 1
        0x01, 0x00, 0x00, 0x00, // female 1
        0x12, 0x00, 0x00, 0x00, // classId 18
        0x15, 0x00, 0x00, 0x00, // int 21
        0x16, 0x00, 0x00, 0x00, // str 22
        0x17, 0x00, 0x00, 0x00, // con 23
        0x18, 0x00, 0x00, 0x00, // men 24
        0x19, 0x00, 0x00, 0x00, // dex 25
        0x1A, 0x00, 0x00, 0x00, // wit 26
        0x01, 0x00, 0x00, 0x00, // hairStyle 1
        0x02, 0x00, 0x00, 0x00, // hairColor 2
        0x03, 0x00, 0x00, 0x00, // face 3
    }, writer.Bytes())
}

// TestCharacterCreateValidation pins the two refusal paths of the
// creation request: the empty name and the name above the sixteen
// character limit of the C1 client both fail before any byte lands
// in the writer.
func TestCharacterCreateValidation(t *testing.T) {
    writer := packet.NewWriter()

    empty := &CharacterCreate{Name: ""}
    require.ErrorContains(t, empty.ToBytes(writer),
        "character name is empty")
    require.Empty(t, writer.Bytes(), "no bytes for a refused name")

    long := &CharacterCreate{
        Name: "seventeenchars16x",
    }
    require.Error(t, long.ToBytes(writer))
    require.Empty(t, writer.Bytes())

    limit := &CharacterCreate{Name: "0123456789abcdef"}
    require.NoError(t, limit.ToBytes(packet.NewWriter()),
        "the sixteen character name passes")
}

// TestAuthLoginNonAsciiLogin pins the non-ASCII login encoding: the
// account name rides the UTF-16LE slow path of the writer (surrogate
// pairs included) and the session key block keeps the PlayOkID2
// before PlayOkID1 order of AuthLogin.readImpl.
func TestAuthLoginNonAsciiLogin(t *testing.T) {
    request := &AuthLogin{
        Login:      "\u00e9l",
        PlayOkID1:  1,
        PlayOkID2:  2,
        LoginOkID1: 3,
        LoginOkID2: 4,
    }
    writer := packet.NewWriter()
    require.NoError(t, request.ToBytes(writer))
    require.Equal(t, []byte{
        0x08,
        0xE9, 0x00, // U+00E9 as UTF-16LE
        0x6C, 0x00, // "l"
        0x00, 0x00, // null terminator
        0x02, 0x00, 0x00, 0x00, // PlayOkID2 2 first
        0x01, 0x00, 0x00, 0x00, // PlayOkID1 1
        0x03, 0x00, 0x00, 0x00, // LoginOkID1 3
        0x04, 0x00, 0x00, 0x00, // LoginOkID2 4
    }, writer.Bytes())
}

// TestAuthLoginEmptyRefused pins the empty login refusal: the error
// fires before any byte lands in the writer.
func TestAuthLoginEmptyRefused(t *testing.T) {
    writer := packet.NewWriter()
    require.ErrorContains(t, (&AuthLogin{}).ToBytes(writer),
        "login is empty")
    require.Empty(t, writer.Bytes())
}

// TestRequestSellItemMultiEntry pins the repeated item body of the
// sell request: the list id, the entry count and then every entry as
// the objectId, itemId, count triple.
func TestRequestSellItemMultiEntry(t *testing.T) {
    request := &RequestSellItemPacket{
        ListID: SellListIDCustom,
        Items: []SellItemEntry{
            {ObjectID: 268439506, ItemID: 57, Count: 100},
            {ObjectID: 268439508, ItemID: 34, Count: 1},
        },
    }
    writer := packet.NewWriter()
    require.NoError(t, request.ToBytes(writer))
    require.Equal(t, []byte{
        0x1E,
        0x00, 0x00, 0x00, 0x00, // listId 0 (custom)
        0x02, 0x00, 0x00, 0x00, // count 2
        0xD2, 0x0F, 0x00, 0x10, // objectId 268439506
        0x39, 0x00, 0x00, 0x00, // itemId 57
        0x64, 0x00, 0x00, 0x00, // count 100
        0xD4, 0x0F, 0x00, 0x10, // objectId 268439508
        0x22, 0x00, 0x00, 0x00, // itemId 34
        0x01, 0x00, 0x00, 0x00, // count 1
    }, writer.Bytes())
}

// TestRequestActionUseFlags pins the ctrl and shift wire forms: both
// flags render as the little endian 1 the readImpl reads.
func TestRequestActionUseFlags(t *testing.T) {
    request := &RequestActionUsePacket{
        ActionID: ActionSitStand,
        Ctrl:     true,
        Shift:    true,
    }
    writer := packet.NewWriter()
    require.NoError(t, request.ToBytes(writer))
    require.Equal(t, []byte{
        0x45,
        0x00, 0x00, 0x00, 0x00, // actionId 0 (sit/stand)
        0x01, 0x00, 0x00, 0x00, // ctrl pressed
        0x01, // shift pressed
    }, writer.Bytes())
}

// TestRequestMagicSkillUseFlags pins the cast request with both
// modifiers pressed: the skill id, the ctrl dword and the shift byte.
func TestRequestMagicSkillUseFlags(t *testing.T) {
    request := &RequestMagicSkillUsePacket{
        SkillID: 1177,
        Ctrl:    true,
        Shift:   true,
    }
    writer := packet.NewWriter()
    require.NoError(t, request.ToBytes(writer))
    require.Equal(t, []byte{
        0x2F,
        0x99, 0x04, 0x00, 0x00, // skillId 1177
        0x01, 0x00, 0x00, 0x00, // ctrl pressed
        0x01, // shift pressed
    }, writer.Bytes())
}

// TestMoveToLocationCursorKeysMode pins the cursor key movement mode:
// the zero mode asks the server for the keyboard movement the escape
// recovery drives (the mouse mode 1 is the default walk).
func TestMoveToLocationCursorKeysMode(t *testing.T) {
    request := &MoveToLocationRequestPacket{
        TargetX: 100, TargetY: 200, TargetZ: -300,
        OriginX: 10, OriginY: 20, OriginZ: -30,
        Mode: MoveModeCursorKeys,
    }
    writer := packet.NewWriter()
    require.NoError(t, request.ToBytes(writer))
    require.Equal(t, []byte{
        0x01,
        0x64, 0x00, 0x00, 0x00, // targetX 100
        0xC8, 0x00, 0x00, 0x00, // targetY 200
        0xD4, 0xFE, 0xFF, 0xFF, // targetZ -300
        0x0A, 0x00, 0x00, 0x00, // originX 10
        0x14, 0x00, 0x00, 0x00, // originY 20
        0xE2, 0xFF, 0xFF, 0xFF, // originZ -30
        0x00, 0x00, 0x00, 0x00, // mode 0 (cursor keys)
    }, writer.Bytes())
}
