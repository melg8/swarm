// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package connection

import (
    "context"
    "encoding/binary"
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
    return absorbingFlowWithMoves(validations, nil)
}

// forwardCapture fans a received client packet into the capture
// channels: the validation stream (0x48) and the move requests
// (0x01) - a nil channel skips its family.
func forwardCapture(
    payload []byte, validations, moves chan<- []byte,
) {
    if len(payload) > 0 && payload[0] == 0x48 && validations != nil {
        select {
        case validations <- payload:
        default:
        }
    }
    if len(payload) > 0 && payload[0] == 0x01 && moves != nil {
        select {
        case moves <- payload:
        default:
        }
    }
}

// absorbingFlowWithMoves extends the absorbing flow with the move
// request channel (opcode 0x01, the mouse and the cursor key walks
// the session sends) alongside the validation stream.
func absorbingFlowWithMoves(
    validations, moves chan<- []byte,
) func(s *fakeGameServer, conn net.Conn, cipher *crypt.GameCrypt) {
    return func(s *fakeGameServer, conn net.Conn, cipher *crypt.GameCrypt) {
        s.characterFlow(conn, cipher)

        // The character selection answer (the CharSelected packet the
        // EnterWorld call of the client waits for).
        var selected []byte
        selected = append(selected, 0x21)
        selected = append(selected, utf16Bytes("unittest1")...)
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

        // The absorb budget: ONE read deadline at the budget end,
        // never a rolling per-iteration deadline. A rolling short
        // deadline races the client's 1 s validation ticker: when it
        // fires mid frame, io.ReadFull has consumed the leading bytes
        // of the packet and loses them, the framing desyncs and every
        // later read returns garbage opcodes - the observed flake
        // mode where the client sent its validations and the server
        // captured none of them. The single budget-end deadline keeps
        // every read whole: a packet arrives complete or the budget
        // closes the flow (the client's own logout close ends it
        // earlier on every clean session). The budget must outlive
        // the longest session window of the callers (5 s today) with
        // margin for a loaded runner.
        const absorbBudget = 10 * time.Second
        _ = conn.SetReadDeadline(time.Now().Add(absorbBudget))
        for {
            payload, err := s.readEncryptedResult(conn, cipher)
            if err != nil {
                // The budget closed or the client went away: both end
                // the flow without tearing down a live session (the
                // flow must not close the connection while the client
                // session still runs - the budget guarantees it).
                return
            }
            forwardCapture(payload, validations, moves)
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
        Account:    "unittest1",
        LoginOkID1: 1,
        LoginOkID2: 2,
        PlayOkID1:  3,
        PlayOkID2:  4,
    })
    require.NoError(t, err)

    updated, err := client.EnsureCharacter(CharacterParams{
        Name:      "unittest1",
        Race:      1,
        Female:    0,
        ClassID:   18,
        HairStyle: 0,
        HairColor: 0,
        Face:      0,
    }, charList)
    require.NoError(t, err)
    slot, _, found := updated.FindCharacterByName("unittest1")
    require.True(t, found)
    require.NoError(t, client.EnterWorld(int32(slot)))

    tracker := state.NewBot("unittest1")
    client.SetTracker(tracker)
    tracker.SetCharacter("unittest1", 100, 18,
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
    // The walk caps at 800 units (the report envelope the assertions
    // below pin) and then holds, so a longer session window changes
    // how long the placement sits at the walk end, not the envelope.
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
                if step < 8 {
                    step++
                }
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

    // The window covers the handshake (the char-create drain alone
    // waits up to charCreateOkWait) plus at least two fires of the
    // one second validation ticker: under -race on a loaded CI
    // runner the 2600 ms window starved the ticker (0-1 fires where
    // 2 are asserted, issue #38) - five seconds leaves the ticker
    // its margin without weakening the assertion.
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    require.NoError(t, client.Run(ctx, "unittest1"))

    // At least two reports left the session (the first ticker fire
    // and one after a placement change) and every report carries the
    // placement the server itself broadcast. The wait is bounded: a
    // stream that never reports fails for its own reason, not by
    // hanging the suite.
    reports := 0
    pollDeadline := time.Now().Add(5 * time.Second)
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

// awaitMove waits for the next move request (opcode 0x01) of the
// session and returns its decoded fields: the target, the origin and
// the movement mode.
func awaitMove(
    t *testing.T, moves <-chan []byte,
) (target [3]int32, origin [3]int32, mode int32) {
    t.Helper()
    select {
    case payload := <-moves:
        require.Len(t, payload, 29)
        require.Equal(t, byte(0x01), payload[0])
        for i := range 3 {
            target[i] = int32(binary.LittleEndian.Uint32(
                payload[1+i*4 : 5+i*4]))
            origin[i] = int32(binary.LittleEndian.Uint32(
                payload[13+i*4 : 17+i*4]))
        }
        mode = int32(binary.LittleEndian.Uint32(payload[25:29]))

        return target, origin, mode
    case <-time.After(2 * time.Second):
        t.Fatal("the session never sent the expected move request")

        return [3]int32{}, [3]int32{}, 0
    }
}

// TestGameClientCursorKeyWalkSendsTheKeyboardMode pins the cursor key
// arm of the escape: the request carries the movement mode 0 (the
// cursor keys of the official client - the mode the server's
// MoveToLocation.readImpl reads as "cursor keys are used") and the
// origin resolves to the tracked placement, exactly the packet the
// official client sends when the player starts an arrow walk.
func TestGameClientCursorKeyWalkSendsTheKeyboardMode(t *testing.T) {
    validations := make(chan []byte, 16)
    moves := make(chan []byte, 16)
    server := startFakeGameServer(t)
    server.flow = absorbingFlowWithMoves(validations, moves)

    client, tracker := newValidatingSession(t, server)
    tracker.ApplyPlacement(state.Placement{
        ObjectID: 100, X: 45400, Y: 50000, Z: -3040, Heading: 32114,
    })

    require.NoError(t, client.CursorKeyWalkTo(43000, 51000, -2992))
    target, origin, mode := awaitMove(t, moves)
    require.Equal(t, [3]int32{43000, 51000, -2992}, target)
    require.Equal(t, [3]int32{45400, 50000, -3040}, origin,
        "the cursor key arm carries the tracked origin")
    require.Equal(t, int32(0), mode,
        "the cursor key arm walks in the keyboard movement mode")

    // The mouse click of the same session walks in the mouse mode -
    // the mode the server reads as a ground click.
    require.NoError(t, client.WalkTo(44000, 50500, -3000))
    _, _, mode = awaitMove(t, moves)
    require.Equal(t, int32(1), mode,
        "the plain walk stays in the mouse movement mode")
}

// TestGameClientClaimsOwnTheValidationStream pins the claim semantics
// of the cursor key escape: the claimed placement is reported as
// claimed (never the tracker echo), the echo ticker of the validation
// stands down while the claims own the stream - the echo of the
// broadcast position would lag the claims by one broadcast and snap
// the character back a step each tick while the server's cursor key
// flag holds - and the first mouse-mode walk returns the stream to
// the echo.
func TestGameClientClaimsOwnTheValidationStream(t *testing.T) {
    validations := make(chan []byte, 16)
    server := startFakeGameServer(t)
    server.flow = absorbingFlow(validations)

    client, tracker := newValidatingSession(t, server)

    // The claim: the claimed placement, not the tracker echo.
    require.NoError(t, client.ClaimValidatePosition(
        45600, 49900, -3050, 16000))
    x, y, z, heading := awaitValidation(t, validations)
    require.Equal(t, int32(45600), x)
    require.Equal(t, int32(49900), y)
    require.Equal(t, int32(-3050), z)
    require.Equal(t, int32(16000), heading)

    // While the claims own the stream, the echo stays quiet even for
    // a changed tracker placement.
    tracker.ApplyPlacement(state.Placement{
        ObjectID: 100, X: 45400, Y: 50000, Z: -3040, Heading: 32114,
    })
    validation := positionValidation{}
    require.NoError(t, client.validatePosition(&validation))
    select {
    case payload := <-validations:
        t.Fatalf("the echo leaked past the claims: %v", payload)
    case <-time.After(300 * time.Millisecond):
    }

    // The mouse click returns the stream to the echo.
    require.NoError(t, client.WalkTo(45000, 50000, -3040))
    require.NoError(t, client.validatePosition(&validation))
    x, y, z, heading = awaitValidation(t, validations)
    require.Equal(t, int32(45400), x)
    require.Equal(t, int32(50000), y)
    require.Equal(t, int32(-3040), z)
    require.Equal(t, int32(32114), heading)
}

// TestGameClientClaimEchoKeepsTheClaimFacing pins the heading of the
// claim echoes: while the claims own the stream, the ValidateLocation
// and StopMove answers of the claimed placements carry the SERVER
// side move heading (the arm direction - the mobius ValidateLocation
// reads the location's heading, the claimed facing lands in the
// client heading field the server does not broadcast back), so an
// echo that applied its heading would flip the displayed facing back
// to the arm line every claim (the 2026-09-19 17:18 report: the web
// UI showed a direction the character never walked). The claim facing
// (state.Bot.ApplySelfFacing) stays the truth while the echo applies
// only the placement; the normal placements own the heading again
// once the mouse walk clears the claims gate.
func TestGameClientClaimEchoKeepsTheClaimFacing(t *testing.T) {
    server := startFakeGameServer(t)
    server.flow = absorbingFlow(nil)

    client, tracker := newValidatingSession(t, server)

    // The claim facing latches (the cursor key escape's optimistic
    // facing) and the claims gate closes.
    tracker.ApplySelfFacing(16000)
    client.claimsOwnStream.Store(true)

    // The echo of the claim: the placement applies, the arm heading
    // in the echo does not flip the facing.
    client.applyValidateLocation(
        buildValidateLocationAt(100, 45600, 49900, -3050, 32768))
    x, y, z, ok := tracker.SelfPosition()
    require.True(t, ok)
    require.Equal(t, int32(45600), x, "the echo placement applies")
    require.Equal(t, int32(49900), y)
    require.Equal(t, int32(-3050), z)
    require.Equal(t, int32(16000), tracker.SelfHeading(),
        "the echo heading must not flip the claim facing")

    // The stop move of the armed walk: the same contract.
    client.applyStopMove(buildStopMove(100, 45500, 49800, -3050, 32768))
    require.Equal(t, int32(16000), tracker.SelfHeading(),
        "the stop move heading must not flip the claim facing either")

    // Another character's placement keeps its own heading: the gate
    // protects the self facing only.
    tracker.ApplyNpcInfo(state.NpcInfo{
        ObjectID: 2, TemplateID: 1,
        X: 44900, Y: 49900, Z: -3050, Heading: 4096,
    })
    client.applyValidateLocation(
        buildValidateLocationAt(2, 44900, 49900, -3050, 4096))
    otherHeading, fok := snapshotObjectHeading(tracker, 2)
    require.True(t, fok)
    require.Equal(t, int32(4096), otherHeading,
        "the other character's heading passes through")

    // The mouse walk clears the claims gate: the echo heading owns
    // the facing again.
    client.claimsOwnStream.Store(false)
    client.applyValidateLocation(
        buildValidateLocationAt(100, 45500, 49850, -3050, 32768))
    require.Equal(t, int32(32768), tracker.SelfHeading(),
        "the normal placement broadcast owns the facing again")
}

// buildValidateLocationAt builds a ValidateLocation packet with an
// explicit z (the placement variant of buildValidateLocation).
func buildValidateLocationAt(
    objectID int32, x int32, y int32, z int32, heading int32,
) []byte {
    data := []byte{0x76}
    data = appendInt32(data, objectID)
    data = appendInt32(data, x)
    data = appendInt32(data, y)
    data = appendInt32(data, z)
    data = appendInt32(data, heading)

    return data
}

// buildStopMove builds a StopMove packet (opcode 0x59, the object id
// with the placement it stopped at).
func buildStopMove(
    objectID int32, x int32, y int32, z int32, heading int32,
) []byte {
    data := []byte{0x59}
    data = appendInt32(data, objectID)
    data = appendInt32(data, x)
    data = appendInt32(data, y)
    data = appendInt32(data, z)
    data = appendInt32(data, heading)

    return data
}

// snapshotObjectHeading reads the heading of a world object from the
// tracker snapshot (false when the object is unknown).
func snapshotObjectHeading(tracker *state.Bot, objectID int32) (int32, bool) {
    for _, obj := range tracker.Snapshot().Objects {
        if obj.ObjectID == objectID {
            return obj.Heading, true
        }
    }

    return 0, false
}
