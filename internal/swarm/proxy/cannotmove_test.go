// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package proxy

import (
    "encoding/binary"
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

// The client move veto interception of the proxy: the attached C1
// client streams CannotMoveAnymore (0x36) whenever its own pawn hits
// the client side geometry, and the Mobius handler throws the reported
// place away and cancels the player's move intention - the server
// stops the bot's walk at the server place. The frozen pawn of the
// village stair railing vetoed every click of the bot and the
// character never walked (the city shopping errand). The proxy drops
// every veto: a spectator client drives no movement, so its local
// collision has no authority over the bot session, and the client
// receives the ValidateLocation correction the server would have sent
// for an out of sync report.

// clientMoveVeto is the captured CannotMoveAnymore payload of the
// stuck city shopping run: the client pawn at (45604, 51295, -2827),
// heading 64069.
var clientMoveVeto = []byte{
    0x36,
    0x24, 0xb2, 0x00, 0x00,
    0x5f, 0xc8, 0x00, 0x00,
    0xf5, 0xf4, 0xff, 0xff,
    0x45, 0xfa, 0x00, 0x00,
}

// TestProxyMoveVetoDroppedAndCorrected pins the veto policy: the
// client's CannotMoveAnymore never reaches the server and the client
// gets the bot place back, so its local walk ends instead of vetoing
// the next bot click too.
func TestProxyMoveVetoDroppedAndCorrected(t *testing.T) {
    server := startTestServer(t)
    recorder, tracker, sender := registerFakeBot(t, server, "VetoChar")

    recorder.Record(buildTestCharList("VetoChar"))
    recorder.Record(buildTestCharSelected("VetoChar"))
    recorder.Record(buildTestUserInfoPacket())

    client := enterWorldAsClient(t, server)
    require.Equal(t, byte(0x04), client.readPacket()[0],
        "the replayed user info")

    // The bot walks at the village gate, the client pawn froze on the
    // stair railing 47 units behind - the live geometry of the run.
    tracker.ApplyTeleport(teleportTo(1055, 45651, 51288, -2832))
    client.sendPacket(clientMoveVeto)

    answer := client.readPacket()
    require.Equal(t, byte(serverOpValidateLocation), answer[0],
        "the veto is answered with a position correction")
    require.Len(t, answer, 21)
    require.EqualValues(t, 1055,
        int32(binary.LittleEndian.Uint32(answer[1:5])),
        "the correction carries the played character object id")
    require.EqualValues(t, 45651,
        int32(binary.LittleEndian.Uint32(answer[5:9])),
        "the correction carries the bot x, never the veto place")
    require.EqualValues(t, 51288,
        int32(binary.LittleEndian.Uint32(answer[9:13])),
        "the correction y")
    require.EqualValues(t, -2832,
        int32(binary.LittleEndian.Uint32(answer[13:17])),
        "the correction z")
    requireNoServerTransit(t, sender)
    requireConnOpen(t, client, 150*time.Millisecond)

    // The policy holds for a veto matching the bot place too: the
    // client's local collision never cancels the bot's own walk, and
    // a repeated veto still does not transit.
    client.sendPacket(buildTestCannotMoveAnymore(45651, 51288, -2832))
    answer = client.readPacket()
    require.Equal(t, byte(serverOpValidateLocation), answer[0],
        "every veto is answered with the live place")
    requireNoServerTransit(t, sender)

    // The following position report rides the bot again: the veto
    // interception leaves the periodic report path untouched.
    client.sendPacket(buildTestValidatePosition(45651, 51288, -2832))
    select {
    case received := <-sender.sent:
        require.Equal(t, byte(clientOpValidatePosition), received[0])
    case <-time.After(3 * time.Second):
        t.Fatal("the position report after the veto never transited")
    }
    requireConnOpen(t, client, 150*time.Millisecond)
}

// buildTestCannotMoveAnymore builds a client move veto with the exact
// layout of the Mobius client packet: [0x36][x: 4][y: 4][z: 4]
// [heading: 4].
func buildTestCannotMoveAnymore(x, y, z int32) []byte {
    payload := []byte{clientOpCannotMoveAnymore}
    payload = binary.LittleEndian.AppendUint32(payload, uint32(x))
    payload = binary.LittleEndian.AppendUint32(payload, uint32(y))
    payload = binary.LittleEndian.AppendUint32(payload, uint32(z))
    payload = binary.LittleEndian.AppendUint32(payload, 0) // heading

    return payload
}
