// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package proxy

import (
    "encoding/binary"
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// The client position report interception of the proxy: the periodic
// ValidatePosition of an attached C1 client carries the client's local
// view of the played character, and the Mobius handler trusts it with
// no distance bound - a report more than one move speed (125 units for
// the level 15 elf) from the server side position hits the "Check out
// of sync" branch of ValidatePosition.runImpl and the server snaps the
// character to the reported place. A spectator client rides the bot
// session while the bot itself drives the character, so its view can
// lag behind (the death spot held across the village restart, the
// frozen pawn of a teleport screen, the railing of the village stairs),
// and every periodic report dragged the character back. The proxy
// severs the loop: a report that contradicts the projected live place
// of the bot beyond one move speed plus a tick of slack never transits,
// and the client receives the same ValidateLocation correction the
// server would have sent for an out of sync report.

// buildTestValidatePosition builds a client position report with the
// exact layout of the Mobius client packet: [0x48][x: 4][y: 4][z: 4]
// [heading: 4][vehicle: 4].
func buildTestValidatePosition(x, y, z int32) []byte {
    payload := []byte{clientOpValidatePosition}
    payload = binary.LittleEndian.AppendUint32(payload, uint32(x))
    payload = binary.LittleEndian.AppendUint32(payload, uint32(y))
    payload = binary.LittleEndian.AppendUint32(payload, uint32(z))
    payload = binary.LittleEndian.AppendUint32(payload, 0) // heading
    payload = binary.LittleEndian.AppendUint32(payload, 0) // vehicle id

    return payload
}

// requireNoServerTransit checks that no client packet reached the fake
// bot session since the last drain.
func requireNoServerTransit(t *testing.T, sender *fakeRawSender) {
    t.Helper()
    select {
    case received := <-sender.sent:
        t.Fatalf("a client packet reached the server: 0x%02x", received[0])
    default:
    }
}

// TestProxyStalePositionReportCorrectedLocally verifies the diverged
// report path: the report never transits to the real server and the
// client receives a ValidateLocation carrying the live place of the
// played character - the correction heals the stale view so the next
// in range report transits again.
func TestProxyStalePositionReportCorrectedLocally(t *testing.T) {
    server := startTestServer(t)
    recorder, tracker, sender := registerFakeBot(t, server, "ViewChar")

    recorder.Record(buildTestCharList("ViewChar"))
    recorder.Record(buildTestCharSelected("ViewChar"))
    recorder.Record(buildTestUserInfoPacket())

    client := enterWorldAsClient(t, server)
    // The replay delivers the recorded user info before the live feed.
    require.Equal(t, byte(0x04), client.readPacket()[0], "the replayed user info")

    // A stale view: the client reports the death spot three thousand
    // units behind the village restart the bot tracker holds.
    stale := buildTestValidatePosition(42000, 53000, -3500)
    client.sendPacket(stale)

    answer := client.readPacket()
    require.Equal(t, byte(serverOpValidateLocation), answer[0],
        "the stale report is answered with a position correction")
    require.Len(t, answer, 21)
    require.EqualValues(t, 1055, int32(binary.LittleEndian.Uint32(answer[1:5])),
        "the correction carries the played character object id")
    require.EqualValues(t, 45000, int32(binary.LittleEndian.Uint32(answer[5:9])),
        "the correction x")
    require.EqualValues(t, 50000, int32(binary.LittleEndian.Uint32(answer[9:13])),
        "the correction y")
    require.EqualValues(t, -3500, int32(binary.LittleEndian.Uint32(answer[13:17])),
        "the correction z")
    requireNoServerTransit(t, sender)
    requireConnOpen(t, client, 150*time.Millisecond)

    // The healed view: the client reports the tracker place again and
    // the report transits like the server contract expects.
    healed := buildTestValidatePosition(45000, 50000, -3500)
    client.sendPacket(healed)
    select {
    case received := <-sender.sent:
        require.Equal(t, byte(clientOpValidatePosition), received[0])
        require.EqualValues(t, 45000,
            int32(binary.LittleEndian.Uint32(received[1:5])))
    case <-time.After(3 * time.Second):
        t.Fatal("the healed position report never transited")
    }
    requireConnOpen(t, client, 150*time.Millisecond)

    // The tracker position feeds the divergence check: moving the bot
    // within the live window of the stale report lets it transit.
    tracker.ApplyTeleport(teleportTo(1055, 42100, 53050, -3500))
    client.sendPacket(stale)
    select {
    case received := <-sender.sent:
        require.Equal(t, byte(clientOpValidatePosition), received[0],
            "the report near the moved bot transits")
    case <-time.After(3 * time.Second):
        t.Fatal("the position report of the moved bot never transited")
    }
}

// TestProxyRailingViewCorrectedLocally pins the owner report of the
// city shopping errand: the attached client froze on the village stair
// railing while the bot stood at the gatekeeper ~500 units away. The
// old 2000 unit window let that report transit, the server snapped the
// character back to the railing (one move speed rule), the open menu
// stayed up and the teleport bypass failed the 250 unit interaction
// range silently, so the errand aborted into the ordinary shop trip.
func TestProxyRailingViewCorrectedLocally(t *testing.T) {
    server := startTestServer(t)
    recorder, tracker, sender := registerFakeBot(t, server, "RailingChar")

    recorder.Record(buildTestCharList("RailingChar"))
    recorder.Record(buildTestCharSelected("RailingChar"))
    recorder.Record(buildTestUserInfoPacket())

    client := enterWorldAsClient(t, server)
    require.Equal(t, byte(0x04), client.readPacket()[0], "the replayed user info")

    // The bot stands at the gatekeeper, the client reports the railing
    // a few hundred units back up the stairs.
    tracker.ApplyTeleport(teleportTo(1055, 46926, 51312, -2992))
    railing := buildTestValidatePosition(46600, 50900, -2800)
    client.sendPacket(railing)

    answer := client.readPacket()
    require.Equal(t, byte(serverOpValidateLocation), answer[0],
        "the railing report is answered with a position correction")
    require.EqualValues(t, 46926, int32(binary.LittleEndian.Uint32(answer[5:9])),
        "the correction carries the gatekeeper x")
    require.EqualValues(t, 51312, int32(binary.LittleEndian.Uint32(answer[9:13])),
        "the correction carries the gatekeeper y")
    requireNoServerTransit(t, sender)
    requireConnOpen(t, client, 150*time.Millisecond)
}

// TestProxyLivePositionReportTransits verifies the in range report
// path: a client that follows its own character keeps the full server
// contract - the report transits unchanged and no correction is
// synthesized. The window is the live run speed plus the slack: the
// fake session never received a UserInfo speed, so the default run
// speed (120) plus the 25 unit slack bounds the live reports at 145.
func TestProxyLivePositionReportTransits(t *testing.T) {
    server := startTestServer(t)
    recorder, _, sender := registerFakeBot(t, server, "LiveView")

    recorder.Record(buildTestCharList("LiveView"))
    recorder.Record(buildTestCharSelected("LiveView"))
    recorder.Record(buildTestUserInfoPacket())

    client := enterWorldAsClient(t, server)
    require.Equal(t, byte(0x04), client.readPacket()[0], "the replayed user info")

    // A live client inside the window: 141 units of interpolation.
    live := buildTestValidatePosition(45100, 50100, -3500)
    client.sendPacket(live)
    select {
    case received := <-sender.sent:
        require.Equal(t, byte(clientOpValidatePosition), received[0])
        require.Len(t, received, 21, "the report transits unchanged")
    case <-time.After(3 * time.Second):
        t.Fatal("the live position report never transited")
    }

    // The zero report of the login edge: the server ignores it itself,
    // the proxy passes it through.
    zero := buildTestValidatePosition(0, 0, 0)
    client.sendPacket(zero)
    select {
    case received := <-sender.sent:
        require.Equal(t, byte(clientOpValidatePosition), received[0])
    case <-time.After(3 * time.Second):
        t.Fatal("the zero position report never transited")
    }
    requireConnOpen(t, client, 150*time.Millisecond)

    // Beyond the window the report is a stale view: it is corrected
    // locally, never reaching the server.
    far := buildTestValidatePosition(45150, 50150, -3500)
    client.sendPacket(far)
    answer := client.readPacket()
    require.Equal(t, byte(serverOpValidateLocation), answer[0],
        "the report beyond the live window is corrected")
    requireNoServerTransit(t, sender)
    requireConnOpen(t, client, 150*time.Millisecond)
}

// teleportTo builds the state teleport notification the tracker test
// uses to move the played character.
func teleportTo(objectID, x, y, z int32) state.Teleport {
    return state.Teleport{
        ObjectID: objectID,
        X:        x,
        Y:        y,
        Z:        z,
        Heading:  0,
    }
}
