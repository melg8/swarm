// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromauthserver

import (
        "testing"

        "github.com/melg8/swarm/internal/swarm/packets/packet"
        "github.com/stretchr/testify/require"
)

// serializePacket runs a packet through ToBytes and returns the content
// bytes (without the login wire framing).
func serializePacket(t *testing.T, p interface {
        ToBytes(*packet.Writer) error
}) []byte {
        t.Helper()
        writer := packet.NewWriter()
        require.NoError(t, p.ToBytes(writer))

        return writer.Bytes()
}

func TestLoginOkToBytesRoundTrip(t *testing.T) {
        t.Parallel()
        original := &LoginOkPacket{LoginOkID1: 0x12345678, LoginOkID2: -559038737}

        content := serializePacket(t, original)
        require.Len(t, content, 1+4+4+5*4)

        parsed := NewLoginOkPacket()
        require.NoError(t, ParseLoginOkPacket(parsed, content))
        require.Equal(t, original.LoginOkID1, parsed.LoginOkID1)
        require.Equal(t, original.LoginOkID2, parsed.LoginOkID2)
}

func TestPlayOkToBytesRoundTrip(t *testing.T) {
        t.Parallel()
        original := &PlayOkPacket{PlayOkID1: 42, PlayOkID2: -1}

        content := serializePacket(t, original)
        require.Len(t, content, 1+4+4)

        parsed := NewPlayOkPacket()
        require.NoError(t, ParsePlayOkPacket(parsed, content))
        require.Equal(t, original.PlayOkID1, parsed.PlayOkID1)
        require.Equal(t, original.PlayOkID2, parsed.PlayOkID2)
}

func TestServerListToBytesRoundTrip(t *testing.T) {
        t.Parallel()
        original := &ServerListPacket{
                Count:      2,
                LastServer: 1,
                Servers: []ServerListEntry{
                        {
                                ServerID:       1,
                                IP:             [4]byte{127, 0, 0, 1},
                                Port:           7778,
                                AgeLimit:       0,
                                Pvp:            0,
                                CurrentPlayers: 5,
                                MaxPlayers:     100,
                                Status:         1,
                                Bits:           0,
                                Brackets:       0,
                        },
                        {
                                ServerID:       2,
                                IP:             [4]byte{192, 168, 1, 10},
                                Port:           7777,
                                AgeLimit:       18,
                                Pvp:            1,
                                CurrentPlayers: 700,
                                MaxPlayers:     900,
                                Status:         1,
                                Bits:           6,
                                Brackets:       1,
                        },
                },
        }

        content := serializePacket(t, original)
        require.Len(t, content, 3+2*serverListEntryWireSize)

        parsed := NewServerListPacket()
        require.NoError(t, ParseServerListPacket(parsed, content))
        require.Equal(t, original.Count, parsed.Count)
        require.Equal(t, original.LastServer, parsed.LastServer)
        require.Len(t, parsed.Servers, 2)
        for i := range original.Servers {
                require.Equal(t, original.Servers[i], parsed.Servers[i])
        }
}

func TestServerListToBytesRejectsTooManyServers(t *testing.T) {
        t.Parallel()
        servers := make([]ServerListEntry, 256)
        for i := range servers {
                servers[i].ServerID = int8(i % 128)
        }
        list := &ServerListPacket{Count: 0, LastServer: 0, Servers: servers}

        writer := packet.NewWriter()
        require.Error(t, list.ToBytes(writer))
}

// serverListEntryWireSize is the wire size of one server list entry:
// id, ip, port, age, pvp, current, max, status, bits, brackets.
const serverListEntryWireSize = 1 + 4 + 4 + 1 + 1 + 2 + 2 + 1 + 4 + 1

// gameGuard3Value mirrors the unsigned Mobius constant 0x97ADB620 as
// the signed int32 the packet struct stores.
const gameGuard3Value int32 = -1750223328

func TestInitPacketToBytesRoundTrip(t *testing.T) {
        t.Parallel()
        rsaKey := make([]byte, 128)
        for i := range rsaKey {
                rsaKey[i] = byte(i)
        }
        original := &InitPacket{
                SessionID:       0x11223344,
                ProtocolVersion: 0x0000c621,
                RsaPublicKey:    rsaKey,
                GameGuard1:      0x29DD954E,
                GameGuard2:      0x77C39CFC,
                GameGuard3:      gameGuard3Value,
                GameGuard4:      0x07BDE0F7,
                BlowfishKey:     nil,
        }

        content := serializePacket(t, original)
        require.Len(t, content, 4+4+128+4*4)

        parsed := NewInitPacket()
        require.NoError(t, ParseInitPacket(parsed, content))
        require.Equal(t, original.SessionID, parsed.SessionID)
        require.Equal(t, original.ProtocolVersion, parsed.ProtocolVersion)
        require.Equal(t, original.RsaPublicKey, parsed.RsaPublicKey)
        require.Equal(t, original.GameGuard1, parsed.GameGuard1)
        require.Equal(t, original.GameGuard2, parsed.GameGuard2)
        require.Equal(t, original.GameGuard3, parsed.GameGuard3)
        require.Equal(t, original.GameGuard4, parsed.GameGuard4)
        require.Nil(t, parsed.BlowfishKey)
}
