// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package connection

import (
    "context"
    "encoding/binary"
    "errors"
    "net"
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/crypt"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// validatePositionBody decodes the field body of a ValidatePosition
// packet (everything past the 0x48 opcode): x, y, z, heading, vehicle.
func validatePositionBody(t *testing.T, payload []byte) (
    x int32, y int32, z int32, heading int32,
) {
    t.Helper()
    require.Len(t, payload, 21)
    require.Equal(t, byte(0x48), payload[0])
    body := payload[1:]
    require.Len(t, body, 20)

    return int32(binary.LittleEndian.Uint32(body[0:4])),
        int32(binary.LittleEndian.Uint32(body[4:8])),
        int32(binary.LittleEndian.Uint32(body[8:12])),
        int32(binary.LittleEndian.Uint32(body[12:16]))
}

// absorbingFlow runs the character handshake, answers the character
// selection and then absorbs every client packet of the session,
// collecting the ValidatePosition stream (opcode 0x48) into the
// channel.
func absorbingFlow(
    validations chan<- []byte,
) func(s *fakeGameServer, conn net.Conn, cipher *crypt.GameCrypt) {
    return func(s *fakeGameServer, conn net.Conn, cipher *crypt.GameCrypt) {
        s.characterFlow(conn, cipher)

        // The character selection answer (the CharSelected packet the
        // EnterWorld call of the client waits for).
        var selected []byte
        selected = append(selected, 0x21)
        selected = append(selected, utf16Bytes("test1")...)
        selected = binary.LittleEndian.AppendUint32(selected, 100)
        selected = append(selected, utf16Bytes("")...)
        selected = binary.LittleEndian.AppendUint32(selected, 42)
        for range 5 {
            selected = binary.LittleEndian.AppendUint32(selected, 0)
        }
        selected = binary.LittleEndian.AppendUint32(selected, 1) // active
        selected = binary.LittleEndian.AppendUint32(selected, 45000)
        selected = binary.LittleEndian.AppendUint32(selected, 50000)
        selected = binary.LittleEndian.AppendUint32(selected, 0xFFFFF268)
        selected = binary.LittleEndian.AppendUint64(selected, 50)
        selected = binary.LittleEndian.AppendUint64(selected, 30)
        s.writeEncrypted(conn, cipher, selected)

        deadline := time.Now().Add(4 * time.Second)
        for time.Now().Before(deadline) {
            _ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
            payload, err := s.readEncryptedResult(conn, cipher)
            if err != nil {
                // The read deadline between the sparse client
                // packets: keep waiting until the session budget
                // closes (the flow must not tear the connection down
                // while the client session still runs).
                var netErr net.Error
                if errors.As(err, &netErr) && netErr.Timeout() {
                    continue
                }

                return
            }
            if len(payload) > 0 && payload[0] == 0x48 {
                select {
                case validations <- payload:
                default:
                }
            }
        }
    }
}

// newValidatingSession dials the fake server, walks the handshake and
// returns the client with a tracker that knows the dump placement.
func newValidatingSession(
    t *testing.T, server *fakeGameServer,
) (*GameClient, *state.Bot) {
    t.Helper()

    conn, err := net.Dial("tcp", server.Addr())
    require.NoError(t, err)

    client, err := NewGameClient(conn)
    require.NoError(t, err)

    charList, err := client.Authenticate(GameSessionParams{
        Account:    "test1",
        LoginOkID1: 1,
        LoginOkID2: 2,
        PlayOkID1:  3,
        PlayOkID2:  4,
    })
    require.NoError(t, err)

    updated, err := client.EnsureCharacter(CharacterParams{
        Name:      "test1",
        Race:      1,
        Female:    0,
        ClassID:   18,
        HairStyle: 0,
        HairColor: 0,
        Face:      0,
    }, charList)
    require.NoError(t, err)
    slot, _, found := updated.FindCharacterByName("test1")
    require.True(t, found)
    require.NoError(t, client.EnterWorld(int32(slot)))

    tracker := state.NewBot("test1")
    client.SetTracker(tracker)
    tracker.SetCharacter("test1", 100, 18,
        45768, 49848, -3056, 358, 145)

    return client, tracker
}

// awaitValidation waits for the next ValidatePosition packet of the
// stream and returns its decoded placement.
func awaitValidation(
    t *testing.T, validations <-chan []byte,
) (int32, int32, int32, int32) {
    t.Helper()
    select {
    case payload := <-validations:
        return validatePositionBody(t, payload)
    case <-time.After(2 * time.Second):
        t.Fatal("the validation stream never sent the expected report")

        return 0, 0, 0, 0
    }
}

// TestGameClientValidatePositionSendsOnChangeAndIdle pins the cadence
// of the client position validation: a fresh state reports at once, an
// unchanged placement stays quiet, a moved placement reports the new
// cells and the standing heartbeat reports even an unchanged placement
// once the idle gap elapsed (the official client cadence the server
// builds its clientX/clientY/clientZ view from).
func TestGameClientValidatePositionSendsOnChangeAndIdle(t *testing.T) {
    validations := make(chan []byte, 16)
    server := startFakeGameServer(t)
    server.flow = absorbingFlow(validations)

    client, tracker := newValidatingSession(t, server)

    // The first report: the tracker placement of the session start.
    validation := positionValidation{}
    require.NoError(t, client.validatePosition(&validation))
    x, y, z, heading := awaitValidation(t, validations)
    require.Equal(t, int32(45768), x)
    require.Equal(t, int32(49848), y)
    require.Equal(t, int32(-3056), z)
    require.Equal(t, int32(0), heading)

    // The unchanged placement: quiet.
    require.NoError(t, client.validatePosition(&validation))
    select {
    case payload := <-validations:
        t.Fatalf("the unchanged placement leaked a report: %v", payload)
    case <-time.After(300 * time.Millisecond):
    }

    // The placement moved: the stream reports the new cells.
    tracker.ApplyPlacement(state.Placement{
        ObjectID: 100, X: 45400, Y: 50000, Z: -3040, Heading: 32114,
    })
    require.NoError(t, client.validatePosition(&validation))
    x, y, z, heading = awaitValidation(t, validations)
    require.Equal(t, int32(45400), x)
    require.Equal(t, int32(50000), y)
    require.Equal(t, int32(-3040), z)
    require.Equal(t, int32(32114), heading)

    // The standing heartbeat: an unchanged placement still reports
    // once the idle gap elapsed.
    validation.at = time.Now().Add(-validatePositionIdleGap)
    require.NoError(t, client.validatePosition(&validation))
    x, y, _, _ = awaitValidation(t, validations)
    require.Equal(t, int32(45400), x)
    require.Equal(t, int32(50000), y)
}

// TestSendPacketAttributesOnlyTheAnsweredRequests pins the request
// bookkeeping of the refusal channel: the walk click records the move
// channel, an action request (the item use of the gear machinery, the
// rider of the 2026-09-14 12:31 dump) records the other channel, and
// the maintenance stream (the position validation of this change, the
// net ping, the handshake family) never records anything - counting
// it would dismiss the genuine walk refusals that arrive while the
// stream flows.
func TestSendPacketAttributesOnlyTheAnsweredRequests(t *testing.T) {
    validations := make(chan []byte, 16)
    server := startFakeGameServer(t)
    server.flow = absorbingFlow(validations)

    client, tracker := newValidatingSession(t, server)

    before := time.Now()
    window := func() (time.Time, time.Time) {
        return before, time.Now().Add(time.Minute)
    }

    // Nothing sent yet: no arrival may be attributed away.
    from, to := window()
    require.False(t, tracker.OtherRequestBetween(from, to))

    // The position validation stream stays out of the bookkeeping.
    validation := positionValidation{}
    require.NoError(t, client.validatePosition(&validation))
    from, to = window()
    require.False(t, tracker.OtherRequestBetween(from, to),
        "the validation stream never owns an ActionFailed answer")

    // The walk click records the move channel only.
    require.NoError(t, client.WalkTo(45000, 49500, -3000))
    from, to = window()
    require.False(t, tracker.OtherRequestBetween(from, to),
        "the walk click is not an other request")

    // An item use (the equip rider of the dump) records the other
    // channel: its ActionFailed answers stop reading as walk
    // refusals.
    require.NoError(t, client.UseItem(12345))
    from, to = window()
    require.True(t, tracker.OtherRequestBetween(from, to),
        "the item use owns its ActionFailed answers")
}

// TestGameClientRunStreamsThePositionValidation pins the run loop
// wiring: the session ticker drives the validation while the character
// moves (the nudged placement of the tracker stands for the server
// echoes a walking character produces), so a moving session keeps the
// server's client view fresh at the official client rate.
func TestGameClientRunStreamsThePositionValidation(t *testing.T) {
    validations := make(chan []byte, 16)
    server := startFakeGameServer(t)
    server.flow = absorbingFlow(validations)

    client, tracker := newValidatingSession(t, server)

    // The walking character: the tracker placement keeps stepping
    // while the session runs, the way the server echoes of a walk do.
    stopNudger := make(chan struct{})
    go func() {
        step := int32(0)
        ticker := time.NewTicker(300 * time.Millisecond)
        defer ticker.Stop()
        for {
            select {
            case <-stopNudger:
                return
            case <-ticker.C:
                step++
                tracker.ApplyPlacement(state.Placement{
                    ObjectID: 100,
                    X:        45768 - step*100,
                    Y:        49848 + step*100,
                    Z:        -3056,
                })
            }
        }
    }()
    defer close(stopNudger)

    ctx, cancel := context.WithTimeout(context.Background(), 2600*time.Millisecond)
    defer cancel()
    require.NoError(t, client.Run(ctx, "test1"))

    // At least two reports left the session (the first ticker fire
    // and one after a placement change) and every report carries the
    // placement the server itself broadcast. The wait is bounded: a
    // stream that never reports fails for its own reason, not by
    // hanging the suite.
    reports := 0
    pollDeadline := time.Now().Add(3 * time.Second)
    for time.Now().Before(pollDeadline) {
        select {
        case payload := <-validations:
            x, y, z, _ := validatePositionBody(t, payload)
            require.Equal(t, int32(-3056), z)
            // The nudger walks the placement up to 800 units over the
            // session; every report carries a placement the server
            // itself broadcast along that walk.
            require.InDelta(t, 45768, x, 900,
                "the report rides the nudged placement")
            require.InDelta(t, 49848, y, 900,
                "the report rides the nudged placement")
            reports++
        case <-time.After(100 * time.Millisecond):
        }
        if reports >= 2 {
            break
        }
    }
    require.GreaterOrEqual(t, reports, 2,
        "the session ticker drove the validation stream")
}
