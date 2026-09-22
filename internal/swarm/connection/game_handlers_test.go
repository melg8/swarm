// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package connection

import (
    "bytes"
    "context"
    "encoding/binary"
    "io"
    "log"
    "math"
    "net"
    "testing"
    "time"

    "github.com/melg8/swarm/internal/swarm/crypt"
    "github.com/melg8/swarm/internal/swarm/gear"
    fromgameserver "github.com/melg8/swarm/internal/swarm/packets/from_game_server"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/stretchr/testify/require"
)

// The object ids of the world flood session: the played character, the
// observed npc, the observed player and two ground items.
const (
    floodSelfID   int32 = 100
    floodNpcID    int32 = 1
    floodPlayerID int32 = 300
    floodItemAID  int32 = 200
    floodItemBID  int32 = 201
)

// startFakeGameServerFlow starts a fake game server running the given
// flow after the handshake.
func startFakeGameServerFlow(
    t *testing.T,
    flow func(s *fakeGameServer, conn net.Conn, cipher *crypt.GameCrypt),
) *fakeGameServer {
    t.Helper()

    listener, err := net.Listen("tcp", "127.0.0.1:0")
    require.NoError(t, err)

    server := &fakeGameServer{listener: listener, t: t, flow: flow}
    go server.serve()

    return server
}

// TestGameClientAppliesWorldPackets drives one world session through
// every observed packet handler: the fake server floods the client with
// the whole known packet set plus truncated garbage, and the tracker
// must reflect the happy path while surviving the malformed packets.
func TestGameClientAppliesWorldPackets(t *testing.T) {
    t.Setenv(packetTraceEnv, "1")

    server := startFakeGameServerFlow(t, (*fakeGameServer).worldFloodFlow)

    conn, err := net.Dial("tcp", server.Addr())
    require.NoError(t, err)

    client, err := NewGameClient(conn)
    require.NoError(t, err)
    client.SetLogger(log.New(io.Discard, "", 0))

    tracker := state.NewBot("flood1")
    client.SetTracker(tracker)
    tracker.SetCharacter("flood1", floodSelfID, 18,
        45000, 50000, -3500, 75, 30)

    ctx, cancel := context.WithTimeout(context.Background(), time.Second)
    defer cancel()
    require.NoError(t, client.Run(ctx, "flood1"))

    snap := tracker.Snapshot()

    // The self state comes from UserInfo, StatusUpdate, ChangeWaitType,
    // TeleportToLocation, Attack and MyTargetSelected.
    require.Equal(t, "flood1", snap.Character.Name)
    require.Equal(t, int32(10), snap.Character.Level)
    require.Equal(t, int32(1), snap.Character.Race)
    require.Equal(t, int32(18), snap.Character.ClassID)
    require.Equal(t, int32(25), snap.Character.STR)
    require.Equal(t, int32(500), snap.Character.CurrentLoad)
    require.Equal(t, int32(24000), snap.Character.MaxLoad)
    require.InDelta(t, 45, snap.Character.CurHP, 0.001)
    require.InDelta(t, 100, snap.Character.MaxHP, 0.001)
    require.Equal(t, int32(44000), snap.Character.X)
    require.Equal(t, int32(49000), snap.Character.Y)
    require.Equal(t, int32(-3400), snap.Character.Z)
    require.Equal(t, int32(4096), snap.Character.Heading)
    require.True(t, snap.Character.Sitting)
    require.Equal(t, floodNpcID, snap.Character.TargetID)

    // The npc chased the player through MoveToPawn and walked through
    // ChangeMoveType.
    npc := findTrackedObject(snap, floodNpcID)
    require.NotNil(t, npc)
    require.Equal(t, state.KindNPC, npc.Kind)
    require.True(t, npc.Attackable)
    require.True(t, npc.Moving)
    require.False(t, npc.Running)
    require.Equal(t, floodPlayerID, npc.TargetID)

    // The player appeared through CharInfo, turned through the rotation
    // pair and dropped its target through TargetUnselected.
    player := findTrackedObject(snap, floodPlayerID)
    require.NotNil(t, player)
    require.Equal(t, state.KindPlayer, player.Kind)
    require.Equal(t, "Player2", player.Name)
    require.Equal(t, "Duelist", player.Title)
    require.True(t, player.Running)
    require.True(t, player.InCombat)
    require.Equal(t, int32(32768), player.Heading)
    require.Zero(t, player.TargetID)

    // The dropped item was picked up and removed, the spawned one stays.
    require.Nil(t, findTrackedObject(snap, floodItemAID))
    item := findTrackedObject(snap, floodItemBID)
    require.NotNil(t, item)
    require.Equal(t, state.KindItem, item.Kind)
    require.Equal(t, int32(34), item.TemplateID)
    require.Equal(t, int32(1), item.Count)

    // The inventory comes from ItemList plus InventoryUpdate.
    require.Len(t, snap.Inventory, 3)
    require.Equal(t, int32(5000), snap.Character.Adena)

    // The unknown packet and the level up animation left their traces.
    var unknownLogged, levelUpChatted bool
    for _, event := range snap.Events {
        unknownLogged = unknownLogged ||
            event.Message == "packet 0xff with 5 bytes"
    }
    for _, line := range snap.Chat {
        levelUpChatted = levelUpChatted ||
            line.Text == "Keltir reached a new level"
    }
    require.True(t, unknownLogged, "the unknown packet must be logged")
    require.True(t, levelUpChatted, "the level up animation must chat")
}

// worldFloodFlow floods the client with every known world packet, then
// with truncated garbage, and waits for the logout of the session.
func (s *fakeGameServer) worldFloodFlow(
    conn net.Conn, cipher *crypt.GameCrypt,
) {
    flood := [][]byte{
        buildFloodUserInfo(),
        buildFloodStatusUpdate(),
        buildNpcInfo(floodNpcID, "Keltir", 45100, 50100, 8192),
        buildFloodCharInfo(),
        buildFloodMoveToPawn(),
        buildFloodAttack(),
        buildFloodAutoAttackStart(),
        buildFloodAutoAttackStop(),
        buildFloodBeginRotation(),
        buildFloodStopRotation(),
        buildFloodChangeMoveType(),
        buildFloodDropItem(),
        buildFloodSpawnItem(),
        buildFloodGetItem(),
        buildFloodTargetSelected(),
        buildFloodTargetUnselected(),
        buildFloodMyTargetSelected(),
        buildFloodSocialAction(),
        buildFloodItemList(),
        buildFloodInventoryUpdate(),
        buildFloodTeleport(),
        buildFloodChangeWaitType(),
        buildFloodSystemMessage(),
        buildFloodNetPing(),
        {0x96},             // leave world confirmation
        {0x36},             // server close announcement
        {0x35},             // action failed
        {0xFF, 1, 2, 3, 4}, // unknown packet id
    }
    for _, packet := range flood {
        s.writeEncrypted(conn, cipher, packet)
    }

    // Every known packet id truncated to one byte exercises the parse
    // error paths: the session must survive the garbage.
    for _, id := range []byte{
        0x04, 0x03, 0x22, 0x01, 0x75, 0x59, 0x76, 0x1A, 0x06,
        0x3B, 0x3C, 0xBF, 0x39, 0x3A, 0x27, 0x37, 0x16, 0x15,
        0x17, 0x77, 0x78, 0x3E, 0x3F, 0x38, 0x7A, 0x3D, 0xEC,
        0x1E,
    } {
        s.writeEncrypted(conn, cipher, []byte{id})
    }

    // An empty frame carries no opcode: the client must skip it.
    s.writeFrame(conn, []byte{})

    s.awaitFloodLogout(conn, cipher)
}

// awaitFloodLogout reads client packets until the logout arrives and
// verifies the teleport confirmation of the self teleport.
func (s *fakeGameServer) awaitFloodLogout(
    conn net.Conn, cipher *crypt.GameCrypt,
) {
    sawAppearing := false
    for {
        payload, err := s.readEncryptedResult(conn, cipher)
        if err != nil {
            break
        }
        if len(payload) == 0 {
            continue
        }
        if payload[0] == 0x30 {
            sawAppearing = true
        }
        if payload[0] == 0x09 {
            break
        }
    }
    require.True(s.t, sawAppearing,
        "the client must confirm the self teleport with Appearing")
}

// TestNetPingAnswersStaySilent floods the session with net ping
// replies and drops the connection: the valid answers must stay
// silent while the malformed one still logs. One log line per reply
// used to flood the process log with several lines per second
// whenever a C1 client was attached through the proxy (the client
// pings continuously and the proxy relays every request through the
// bot session).
func TestNetPingAnswersStaySilent(t *testing.T) {
    server := startFakeGameServerFlow(t, (*fakeGameServer).netPingFloodFlow)

    conn, err := net.Dial("tcp", server.Addr())
    require.NoError(t, err)

    client, err := NewGameClient(conn)
    require.NoError(t, err)

    logBuf := &bytes.Buffer{}
    client.SetLogger(log.New(logBuf, "", 0))

    // The flow closes the server side: Run reports the connection
    // loss after draining the ping burst.
    require.Error(t, client.Run(context.Background(), "pingflood"))

    output := logBuf.String()
    require.NotContains(t, output, "Net ping with game time",
        "the valid net ping replies must not log")
    require.Contains(t, output, "Failed to parse net ping",
        "a malformed reply must still log the parse failure")
}

// netPingFloodCount is the burst size of the net ping silence test:
// the live client produced several replies per second, the burst
// exaggerates it so any regression is obvious.
const netPingFloodCount = 100

// netPingFloodFlow answers the session with a burst of net ping
// replies followed by one truncated packet, then drops the
// connection.
func (s *fakeGameServer) netPingFloodFlow(
    conn net.Conn, cipher *crypt.GameCrypt,
) {
    for range netPingFloodCount {
        s.writeEncrypted(conn, cipher, buildFloodNetPing())
    }
    s.writeEncrypted(conn, cipher, []byte{0xEC}) // truncated game time
    _ = conn.Close()
}

// appendFloat64 appends a little endian float64 value to the packet.
func appendFloat64(dst []byte, value float64) []byte {
    var buf [8]byte
    binary.LittleEndian.PutUint64(buf[:], math.Float64bits(value))

    return append(dst, buf[:]...)
}

// appendUint16 appends a little endian uint16 value to the packet.
func appendUint16(dst []byte, value uint16) []byte {
    var buf [2]byte
    binary.LittleEndian.PutUint16(buf[:], value)

    return append(dst, buf[:]...)
}

// buildFloodUserInfo builds a UserInfo packet of the played character.
func buildFloodUserInfo() []byte {
    data := []byte{0x04}
    data = appendInt32(data, 45000) // x
    data = appendInt32(data, 50000) // y
    data = appendInt32(data, -3500) // z
    data = appendInt32(data, 0)     // vehicle id
    data = appendInt32(data, floodSelfID)
    data = append(data, utf16Bytes("flood1")...)
    data = appendInt32(data, 1)     // race
    data = appendInt32(data, 0)     // female
    data = appendInt32(data, 18)    // base class
    data = appendInt32(data, 10)    // level
    data = appendInt32(data, 48229) // exp
    data = appendInt32(data, 25)    // str
    data = appendInt32(data, 30)    // dex
    data = appendInt32(data, 28)    // con
    data = appendInt32(data, 20)    // int
    data = appendInt32(data, 18)    // wit
    data = appendInt32(data, 22)    // men
    data = appendInt32(data, 100)   // max hp
    data = appendInt32(data, 75)    // cur hp
    data = appendInt32(data, 40)    // max mp
    data = appendInt32(data, 30)    // cur mp
    data = appendInt32(data, 500)   // sp
    data = appendInt32(data, 500)   // cur load
    data = appendInt32(data, 24000) // max load
    data = appendInt32(data, 0)     // weapon flag
    for slot := range 15 {          // paperdoll object ids
        if slot == 7 {
            data = appendInt32(data, 268476112) // right hand
        } else {
            data = appendInt32(data, 0)
        }
    }
    data = append(data, make([]byte, 15*4+12*4)...) // display ids, stats
    data = appendInt32(data, 120)                   // run speed
    data = appendInt32(data, 60)                    // walk speed
    data = append(data, make([]byte, 24)...)        // swim and fly speeds
    data = appendFloat64(data, 1.0)                 // move multiplier

    return data
}

// buildFloodStatusUpdate builds a StatusUpdate of the played character.
func buildFloodStatusUpdate() []byte {
    data := []byte{0x1A}
    data = appendInt32(data, floodSelfID)
    data = appendInt32(data, 4) // attribute count
    data = appendInt32(data, 0x09)
    data = appendInt32(data, 45) // cur hp
    data = appendInt32(data, 0x0A)
    data = appendInt32(data, 100) // max hp
    data = appendInt32(data, 0x0E)
    data = appendInt32(data, 500) // cur load
    data = appendInt32(data, 0x0F)
    data = appendInt32(data, 24000) // max load

    return data
}

// buildFloodCharInfo builds a CharInfo packet of an observed player.
func buildFloodCharInfo() []byte {
    data := []byte{0x03}
    data = appendInt32(data, 45100) // x
    data = appendInt32(data, 50100) // y
    data = appendInt32(data, -3500) // z
    data = appendInt32(data, 0)     // vehicle id
    data = appendInt32(data, floodPlayerID)
    data = append(data, utf16Bytes("Player2")...)
    data = appendInt32(data, 1)                      // race
    data = appendInt32(data, 0)                      // female
    data = appendInt32(data, 18)                     // base class
    data = append(data, make([]byte, (12+2+2)*4)...) // paperdoll, stats
    data = appendInt32(data, 130)                    // run speed
    data = appendInt32(data, 65)                     // walk speed
    data = append(data, make([]byte, 24)...)         // swim and fly speeds
    data = appendFloat64(data, 1.0)                  // move multiplier
    data = append(data, make([]byte, 8)...)          // attack speed multiplier
    data = appendFloat64(data, 8.0)                  // collision radius
    data = append(data, make([]byte, 8+3*4)...)      // height, hair, face
    data = append(data, utf16Bytes("Duelist")...)
    data = append(data, make([]byte, 4*5)...) // clan, ally, relation
    data = append(data, 1, 1, 1, 0, 0, 0, 0)  // standing, running, combat

    return data
}

// buildFloodMoveToPawn builds a MoveToPawn of the npc chasing the player.
func buildFloodMoveToPawn() []byte {
    data := []byte{0x75}
    data = appendInt32(data, floodNpcID)
    data = appendInt32(data, floodPlayerID)
    data = appendInt32(data, 60) // stop distance
    data = appendInt32(data, 45100)
    data = appendInt32(data, 50100)
    data = appendInt32(data, -3500)
    data = appendInt32(data, 45000)
    data = appendInt32(data, 50000)
    data = appendInt32(data, -3500)

    return data
}

// buildFloodAttack builds an Attack of the player hitting the character.
func buildFloodAttack() []byte {
    data := []byte{0x06}
    data = appendInt32(data, floodPlayerID) // attacker
    data = appendInt32(data, floodSelfID)   // first hit target
    data = appendInt32(data, 25)            // damage
    data = append(data, 0)                  // hit flags
    data = appendInt32(data, 44900)         // attacker x
    data = appendInt32(data, 49900)         // attacker y
    data = appendInt32(data, -3500)         // attacker z
    data = appendUint16(data, 0)            // hits left
    data = appendInt32(data, 44800)         // target x
    data = appendInt32(data, 49800)         // target y
    data = appendInt32(data, -3500)         // target z

    return data
}

// buildFloodAutoAttackStart builds an AutoAttackStart of the player.
func buildFloodAutoAttackStart() []byte {
    return appendInt32([]byte{0x3B}, floodPlayerID)
}

// buildFloodAutoAttackStop builds an AutoAttackStop of the player.
func buildFloodAutoAttackStop() []byte {
    return appendInt32([]byte{0x3C}, floodPlayerID)
}

// buildFloodChangeMoveType switches the chasing npc to walking.
func buildFloodChangeMoveType() []byte {
    data := appendInt32([]byte{0x3E}, floodNpcID)

    return appendInt32(data, 0) // walk
}

// buildFloodSocialAction builds the level up animation of the npc.
func buildFloodSocialAction() []byte {
    data := appendInt32([]byte{0x3D}, floodNpcID)

    return appendInt32(data, 15) // level up
}

// buildFloodNetPing builds a NetPing response with a game time.
func buildFloodNetPing() []byte {
    return appendInt32([]byte{0xEC}, 12345)
}

// buildFloodBeginRotation builds a BeginRotation of the player.
func buildFloodBeginRotation() []byte {
    data := []byte{0x77}
    data = appendInt32(data, floodPlayerID)
    data = appendInt32(data, 16384)
    data = appendInt32(data, 0) // side
    data = appendInt32(data, 0) // speed

    return data
}

// buildFloodStopRotation builds a StopRotation of the player.
func buildFloodStopRotation() []byte {
    data := []byte{0x78}
    data = appendInt32(data, floodPlayerID)
    data = appendInt32(data, 32768)
    data = appendInt32(data, 0) // speed
    data = append(data, 0)      // unknown

    return data
}

// buildFloodDropItem builds a DropItem of adena dropped by the npc.
func buildFloodDropItem() []byte {
    data := []byte{0x16}
    data = appendInt32(data, floodNpcID) // dropper
    data = appendInt32(data, floodItemAID)
    data = appendInt32(data, 57) // display id (adena)
    data = appendInt32(data, 45200)
    data = appendInt32(data, 50200)
    data = appendInt32(data, -3480)
    data = appendInt32(data, 1)    // stackable
    data = appendInt32(data, 1000) // count
    data = appendInt32(data, 0)    // unknown

    return data
}

// buildFloodSpawnItem builds a SpawnItem of a stem lying on the ground.
func buildFloodSpawnItem() []byte {
    data := []byte{0x15}
    data = appendInt32(data, floodItemBID)
    data = appendInt32(data, 34) // display id (stem)
    data = appendInt32(data, 45250)
    data = appendInt32(data, 50250)
    data = appendInt32(data, -3480)
    data = appendInt32(data, 0) // stackable
    data = appendInt32(data, 1) // count

    return data
}

// buildFloodGetItem builds a GetItem of the player picking the drop up.
func buildFloodGetItem() []byte {
    data := []byte{0x17}
    data = appendInt32(data, floodPlayerID)
    data = appendInt32(data, floodItemAID)
    data = appendInt32(data, 45200)
    data = appendInt32(data, 50200)
    data = appendInt32(data, -3480)

    return data
}

// buildFloodTargetSelected builds a TargetSelected of the player.
func buildFloodTargetSelected() []byte {
    data := []byte{0x39}
    data = appendInt32(data, floodPlayerID)
    data = appendInt32(data, floodNpcID)
    data = appendInt32(data, 45100)
    data = appendInt32(data, 50100)
    data = appendInt32(data, -3500)
    data = appendInt32(data, 0) // unknown

    return data
}

// buildFloodTargetUnselected builds a TargetUnselected of the player.
func buildFloodTargetUnselected() []byte {
    data := []byte{0x3A}
    data = appendInt32(data, floodPlayerID)

    return append(data, make([]byte, 16)...)
}

// buildFloodMyTargetSelected builds a MyTargetSelected of the character.
func buildFloodMyTargetSelected() []byte {
    data := []byte{0xBF}
    data = appendInt32(data, floodNpcID)

    return append(data, 0, 0) // target color
}

// buildFloodItemList builds an ItemList with a weapon and adena.
func buildFloodItemList() []byte {
    data := []byte{0x27}
    data = appendUint16(data, 0) // show window
    data = appendUint16(data, 2) // item count
    data = append(data, buildFloodItem(301, 34, 1, 0, 1, 0x80)...)
    data = append(data, buildFloodItem(302, 57, 5000, 4, 0, 0)...)

    return data
}

// buildFloodInventoryUpdate builds an InventoryUpdate adding two jewels.
func buildFloodInventoryUpdate() []byte {
    data := []byte{0x37}
    data = appendUint16(data, 1) // entry count
    data = appendUint16(data, 1) // change: added
    data = append(data, buildFloodItem(303, 1061, 2, 2, 0, 0)...)

    return data
}

// buildFloodItem builds one inventory entry of the item packets.
func buildFloodItem(
    objectID int32, itemID int32, count int32,
    type2 int16, equipped int16, bodyPart int32,
) []byte {
    data := appendUint16(nil, 4) // type1
    data = appendInt32(data, objectID)
    data = appendInt32(data, itemID)
    data = appendInt32(data, count)
    data = appendUint16(data, uint16(type2))
    data = appendUint16(data, 0) // custom type1
    data = appendUint16(data, uint16(equipped))
    data = appendInt32(data, bodyPart)
    data = appendUint16(data, 0) // enchant
    data = appendUint16(data, 0) // custom type2

    return data
}

// buildFloodTeleport builds a TeleportToLocation of the played character.
func buildFloodTeleport() []byte {
    data := []byte{0x38}
    data = appendInt32(data, floodSelfID)
    data = appendInt32(data, 44000)
    data = appendInt32(data, 49000)
    data = appendInt32(data, -3400)
    data = appendInt32(data, 0)    // fade
    data = appendInt32(data, 4096) // heading

    return data
}

// buildFloodChangeWaitType builds the sitting transition of the character.
func buildFloodChangeWaitType() []byte {
    data := []byte{0x3F}
    data = appendInt32(data, floodSelfID)
    data = appendInt32(data, 0) // sitting
    data = appendInt32(data, 44000)
    data = appendInt32(data, 49000)
    data = appendInt32(data, -3400)

    return data
}

// buildFloodSystemMessage builds a SystemMessage with one int parameter.
func buildFloodSystemMessage() []byte {
    data := []byte{0x7A}
    data = appendInt32(data, 34)  // message id
    data = appendInt32(data, 1)   // parameter count
    data = appendInt32(data, 1)   // int parameter type
    data = appendInt32(data, 100) // parameter value

    return data
}

// TestGameClientSendsClientActions fires every client action method of
// the GameClient and checks the fake server received the right opcodes.
func TestGameClientSendsClientActions(t *testing.T) {
    server := startFakeGameServerFlow(t, (*fakeGameServer).actionsFlow)

    conn, err := net.Dial("tcp", server.Addr())
    require.NoError(t, err)

    client, err := NewGameClient(conn)
    require.NoError(t, err)
    client.SetLogger(log.New(io.Discard, "", 0))

    tracker := state.NewBot("actor")
    client.SetTracker(tracker)
    tracker.SetCharacter("actor", floodSelfID, 18,
        45000, 50000, -3500, 50, 30)
    tracker.ApplyNpcInfo(state.NpcInfo{
        ObjectID:   floodNpcID,
        TemplateID: 1001277,
        Attackable: true,
        X:          45100,
        Y:          50100,
        Z:          -3500,
        RunSpeed:   120,
        Name:       "Keltir",
    })

    // A zero target is a no-op, an unknown target is an error.
    require.NoError(t, client.AttackTarget(0))
    require.Error(t, client.AttackTarget(999))
    require.NoError(t, client.AttackTarget(floodNpcID))
    require.NoError(t, client.PickupItem(state.LootItem{
        ObjectID: floodItemAID,
        X:        45200,
        Y:        50200,
        Z:        -3480,
        Name:     "Adena",
    }))
    require.NoError(t, client.WalkTo(46000, 51000, -3500))
    require.NoError(t, client.UseItem(268476112))
    require.NoError(t, client.DestroyItem(301, 1))
    require.NoError(t, client.DropItem(302, 50, 45000, 50000, -3500))
    require.NoError(t, client.SellItems([]state.InventoryItem{
        {ObjectID: 301, ItemID: 34, Count: 1},
    }))
    require.NoError(t, client.BuyItems(7150, []gear.Purchase{
        {
            ItemID: 34, ListID: 0, MerchantTemplateID: 0, Count: 1,
            Price: 0, Reason: "", SellFirst: nil, SellCredit: 0,
            Gain: 0, Affordable: true, Missing: 0,
        },
    }))
    require.NoError(t, client.RequestInventory())
    require.NoError(t, client.ActionSitStand())
    require.NoError(t, client.RestartAtVillage())
    require.NoError(t, client.RequestLogout())

    // RequestLogout closes the socket: the server loop ends and checks
    // the received opcode set.
}

// actionsFlow collects the client packet opcodes of the action session.
func (s *fakeGameServer) actionsFlow(
    conn net.Conn, cipher *crypt.GameCrypt,
) {
    expected := map[byte]bool{
        0x0A: false, // attack request
        0x04: false, // pickup action
        0x01: false, // walk to location
        0x14: false, // use item
        0x59: false, // destroy item
        0x12: false, // drop item
        0x1E: false, // sell items
        0x1F: false, // buy items
        0x0F: false, // request item list
        0x45: false, // action use (sit/stand)
        0x6D: false, // restart point
        0x09: false, // logout
    }
    for {
        payload, err := s.readEncryptedResult(conn, cipher)
        if err != nil {
            break
        }
        if len(payload) == 0 {
            continue
        }
        if _, known := expected[payload[0]]; known {
            expected[payload[0]] = true
        }
    }
    for opcode, seen := range expected {
        require.True(s.t, seen, "expected client packet 0x%02x", opcode)
    }
}

// interactionActionsFlow collects the client packet opcodes of the
// interaction session (the say, the npc clicks, the skill rounds).
func (s *fakeGameServer) interactionActionsFlow(
    conn net.Conn, cipher *crypt.GameCrypt,
) {
    expected := map[byte]bool{
        0x38: false, // say2 (the chat text)
        0x04: false, // action request (the clicks, the self clear)
        0x6C: false, // request acquire skill
        0x2F: false, // request magic skill use
    }
    for {
        payload, err := s.readEncryptedResult(conn, cipher)
        if err != nil {
            break
        }
        if len(payload) == 0 {
            continue
        }
        if _, known := expected[payload[0]]; known {
            expected[payload[0]] = true
        }
    }
    for opcode, seen := range expected {
        require.True(s.t, seen, "expected client packet 0x%02x", opcode)
    }
}

// TestGameClientSendsInteractionActions fires the interaction action
// methods of the GameClient - the chat say, the npc clicks of the
// teacher trips, the selection clear and the skill rounds - and
// checks the fake server received their opcodes. The send tap
// observes the same sends on the plaintext side.
func TestGameClientSendsInteractionActions(t *testing.T) {
    server := startFakeGameServerFlow(
        t, (*fakeGameServer).interactionActionsFlow)

    conn, err := net.Dial("tcp", server.Addr())
    require.NoError(t, err)

    client, err := NewGameClient(conn)
    require.NoError(t, err)
    client.SetLogger(log.New(io.Discard, "", 0))

    tracker := state.NewBot("actor")
    client.SetTracker(tracker)
    tracker.SetCharacter("actor", floodSelfID, 18,
        45000, 50000, -3500, 50, 30)
    tracker.ApplyNpcInfo(state.NpcInfo{
        ObjectID:   floodNpcID,
        TemplateID: 1001277,
        Attackable: true,
        X:          45100,
        Y:          50100,
        Z:          -3500,
        RunSpeed:   120,
        Name:       "Keltir",
    })

    // The send tap reads the plaintext of every outbound packet.
    sends := &tapCollector{}
    client.SetSendTap(sends.tap)

    // A chat line on the general channel.
    require.NoError(t, client.Say("hello village", 0, ""))

    // A zero click is a no-op, an unknown object is an error and a
    // known npc resolves its position into the action request.
    require.NoError(t, client.ClickObject(0))
    require.Error(t, client.ClickObject(999))
    require.NoError(t, client.ClickObject(floodNpcID))

    // The interact pull repeats the action request shape.
    require.NoError(t, client.InteractPull(0))
    require.Error(t, client.InteractPull(999))
    require.NoError(t, client.InteractPull(floodNpcID))

    // Without a selection the clear is a no-op; a self movement
    // packet that targets the npc gives the character a selection
    // and the clear click fires.
    require.NoError(t, client.ClearTarget())
    tracker.ApplyPawnMovement(state.PawnMovement{
        ObjectID: floodSelfID,
        TargetID: floodNpcID,
        X:        45000,
        Y:        50000,
        Z:        -3500,
        TargetX:  45100,
        TargetY:  50100,
        TargetZ:  -3500,
    })
    require.Equal(t, floodNpcID, tracker.SelfTargetID())
    require.NoError(t, client.ClearTarget())

    // The skill rounds: a lesson at the teacher and a cast.
    require.NoError(t, client.AcquireSkill(1177, 1))
    require.NoError(t, client.UseMagicSkill(1177))

    // The tap observed the plaintext of the fired sends: the chat,
    // the action requests, the acquire and the cast.
    seen := map[byte]bool{}
    for _, opcode := range sends.opcodes() {
        seen[opcode] = true
    }
    require.True(t, seen[0x38], "the tap must see the say packet")
    require.True(t, seen[0x04], "the tap must see the action requests")
    require.True(t, seen[0x6C], "the tap must see the acquire skill")
    require.True(t, seen[0x2F], "the tap must see the magic skill use")

    // RequestLogout closes the socket: the server loop ends and
    // checks the received opcode set.
    require.NoError(t, client.RequestLogout())
}

// buildFloodCreatureSay builds a CreatureSay packet: the villager
// greets the character on the general channel.
func buildFloodCreatureSay() []byte {
    data := []byte{0x5D}
    data = appendInt32(data, floodNpcID)
    data = appendInt32(data, 0) // channel: say
    data = append(data, utf16Bytes("Keltir Fan")...)
    data = append(data, utf16Bytes("welcome to the village")...)

    return data
}

// creatureSayFlow feeds one valid and one truncated world chat line,
// then drops the connection.
func (s *fakeGameServer) creatureSayFlow(
    conn net.Conn, cipher *crypt.GameCrypt,
) {
    s.writeEncrypted(conn, cipher, buildFloodCreatureSay())
    s.writeEncrypted(conn, cipher, []byte{0x5D}) // truncated say
    _ = conn.Close()
}

// TestGameClientAppliesCreatureSay checks the world chat dispatch:
// the valid line lands in the tracker chat window, the truncated
// packet logs its parse failure and never reaches the window.
func TestGameClientAppliesCreatureSay(t *testing.T) {
    server := startFakeGameServerFlow(t, (*fakeGameServer).creatureSayFlow)

    conn, err := net.Dial("tcp", server.Addr())
    require.NoError(t, err)

    client, err := NewGameClient(conn)
    require.NoError(t, err)

    logBuf := &bytes.Buffer{}
    client.SetLogger(log.New(logBuf, "", 0))

    tracker := state.NewBot("chatter")
    client.SetTracker(tracker)

    // The flow closes the server side: Run reports the loss after
    // draining the chat lines.
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    require.Error(t, client.Run(ctx, "chatter"))

    chat := tracker.Snapshot().Chat
    require.Len(t, chat, 1, "only the valid line reaches the window")
    require.Equal(t, "Keltir Fan", chat[0].From)
    require.Equal(t, "welcome to the village", chat[0].Text)
    require.Equal(t, "say", chat[0].Kind)

    require.Contains(t, logBuf.String(), "Failed to parse creature say",
        "the truncated packet must log its parse failure")
}

// buildFloodSkillList builds a SkillList packet: the mystic knows
// Wind Strike (active) and one passive.
func buildFloodSkillList() []byte {
    data := []byte{0x6D}
    data = appendInt32(data, 2) // skill count
    data = appendInt32(data, 0) // active
    data = appendInt32(data, 1) // level 1
    data = appendInt32(data, 1177)
    data = appendInt32(data, 1) // passive
    data = appendInt32(data, 5) // level 5
    data = appendInt32(data, 194)

    return data
}

// buildFloodMagicSkillUse builds a MagicSkillUse packet: the played
// character casts Wind Strike with a cast bar and a reuse window.
func buildFloodMagicSkillUse() []byte {
    data := []byte{0x5A}
    data = appendInt32(data, floodSelfID) // caster: the character
    data = appendInt32(data, floodNpcID)  // target: the npc
    data = appendInt32(data, 1177)        // skill id
    data = appendInt32(data, 1)           // skill level
    data = appendInt32(data, 1200)        // hit time ms
    data = appendInt32(data, 30000)       // reuse delay ms
    data = appendInt32(data, 45000)       // caster x
    data = appendInt32(data, 50000)       // caster y
    data = appendInt32(data, -3500)       // caster z
    data = appendInt32(data, 0)           // critical flag: no pad
    data = appendInt32(data, 45100)       // target x
    data = appendInt32(data, 50100)       // target y
    data = appendInt32(data, -3500)       // target z

    return data
}

// skillRoundFlow feeds the skill list, one self cast and one
// truncated cast, then drops the connection.
func (s *fakeGameServer) skillRoundFlow(
    conn net.Conn, cipher *crypt.GameCrypt,
) {
    s.writeEncrypted(conn, cipher, buildFloodSkillList())
    s.writeEncrypted(conn, cipher, buildFloodMagicSkillUse())
    s.writeEncrypted(conn, cipher, []byte{0x5A}) // truncated cast
    _ = conn.Close()
}

// TestGameClientAppliesSkillRound checks the skill bookkeeping: the
// SkillList lands in the learned skills of the tracker, the self
// cast opens the cast and reuse windows and the truncated cast logs
// its parse failure.
func TestGameClientAppliesSkillRound(t *testing.T) {
    server := startFakeGameServerFlow(t, (*fakeGameServer).skillRoundFlow)

    conn, err := net.Dial("tcp", server.Addr())
    require.NoError(t, err)

    client, err := NewGameClient(conn)
    require.NoError(t, err)

    logBuf := &bytes.Buffer{}
    client.SetLogger(log.New(logBuf, "", 0))

    tracker := state.NewBot("caster")
    client.SetTracker(tracker)
    tracker.SetCharacter("caster", floodSelfID, 18,
        45000, 50000, -3500, 50, 30)

    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    require.Error(t, client.Run(ctx, "caster"))

    // The skill list landed: the snapshot book keeps both skills in
    // id order (the passive distinction reads through ActiveSkills
    // below, the snapshot names come from the npc dictionary).
    require.True(t, tracker.SkillsListed())
    skills := tracker.Snapshot().Skills
    require.Len(t, skills, 2)
    require.Equal(t, int32(194), skills[0].SkillID)
    require.Equal(t, int32(5), skills[0].Level)
    require.Equal(t, int32(1177), skills[1].SkillID)
    require.Equal(t, int32(1), skills[1].Level)
    active := tracker.ActiveSkills()
    require.Len(t, active, 1)
    require.Equal(t, int32(1177), active[0].SkillID)

    // The self cast opened both windows of Wind Strike.
    states := tracker.Snapshot().SkillStates
    require.Len(t, states, 1)
    require.Equal(t, int32(1177), states[0].SkillID)
    require.Equal(t, int64(1200), states[0].CastTotalMs)
    require.Equal(t, int64(30000), states[0].ReuseTotalMs)
    require.Positive(t, states[0].CastLeftMs)
    require.Positive(t, states[0].ReuseLeftMs)

    require.Contains(t, logBuf.String(), "Failed to parse magic skill use",
        "the truncated cast must log its parse failure")
    require.Contains(t, logBuf.String(), "Skill list with 2 skills")
}

// buildFloodQuestList builds a QuestList packet: two active quests
// and two quest bound item stacks.
func buildFloodQuestList() []byte {
    data := []byte{0x98}
    data = appendUint16(data, 2)  // quest count
    data = appendInt32(data, 406) // the starter quest
    data = appendInt32(data, 2)   // state: cond 2
    data = appendInt32(data, 1)   // a second quest
    data = appendInt32(data, 5)
    data = appendUint16(data, 2)  // quest item count
    data = appendInt32(data, 500) // object id
    data = appendInt32(data, 756) // item id
    data = appendInt32(data, 3)   // count
    data = appendInt32(data, 0)   // body part
    data = appendInt32(data, 501)
    data = appendInt32(data, 757)
    data = appendInt32(data, 1)
    data = appendInt32(data, 0)

    return data
}

// buildFloodAbnormalStatus builds an AbnormalStatusUpdate packet:
// two running effects on the character.
func buildFloodAbnormalStatus() []byte {
    data := []byte{0x97}
    data = appendUint16(data, 2)   // effect count
    data = appendInt32(data, 1204) // Wind Walk
    data = appendUint16(data, 2)   // level 2
    data = appendInt32(data, 300)  // 300 seconds left
    data = appendInt32(data, 1068) // Shield
    data = appendUint16(data, 3)   // level 3
    data = appendInt32(data, 1200) // 1200 seconds left

    return data
}

// buildFloodNpcHTML builds an NpcHTMLMessage packet: the gatekeeper
// dialog with two bypass buttons.
func buildFloodNpcHTML() []byte {
    html := "<html>Greetings.<br><a action=\"bypass -h npc_7155_Chat 0\">Talk</a>" +
        "<a action=\"bypass -h npc_7155_teleport 1 2\">Teleport</a></html>"
    data := []byte{0x1B}
    data = appendInt32(data, 7155) // the gatekeeper npc
    data = append(data, utf16Bytes(html)...)
    data = appendInt32(data, 0) // the npc html scope carries no item

    return data
}

// journalFlow feeds the quest journal, the buff bar, the npc dialog
// and one truncated journal packet, then drops the connection.
func (s *fakeGameServer) journalFlow(
    conn net.Conn, cipher *crypt.GameCrypt,
) {
    s.writeEncrypted(conn, cipher, buildFloodQuestList())
    s.writeEncrypted(conn, cipher, buildFloodAbnormalStatus())
    s.writeEncrypted(conn, cipher, buildFloodNpcHTML())
    s.writeEncrypted(conn, cipher, []byte{0x98}) // truncated journal
    _ = conn.Close()
}

// TestGameClientAppliesQuestJournal checks the journal round: the
// quest journal lands in the tracker (the states and the quest bound
// items), the buff bar replaces the effect list and the npc dialog
// reaches both the last-html latch and the tracker dialog page.
func TestGameClientAppliesQuestJournal(t *testing.T) {
    server := startFakeGameServerFlow(t, (*fakeGameServer).journalFlow)

    conn, err := net.Dial("tcp", server.Addr())
    require.NoError(t, err)

    client, err := NewGameClient(conn)
    require.NoError(t, err)

    logBuf := &bytes.Buffer{}
    client.SetLogger(log.New(logBuf, "", 0))

    tracker := state.NewBot("journal")
    client.SetTracker(tracker)

    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    require.Error(t, client.Run(ctx, "journal"))

    // The quest journal landed with both quests and both items.
    require.Equal(t, 2, tracker.QuestCount())
    cond, active := tracker.QuestCond(406)
    require.True(t, active)
    require.Equal(t, int32(2), cond)
    _, active = tracker.QuestCond(999)
    require.False(t, active)
    require.True(t, tracker.IsQuestItem(756))
    require.False(t, tracker.IsQuestItem(999))

    // The buff bar carries both effects with their levels (the
    // snapshot orders them by skill id).
    buffs := tracker.Snapshot().Buffs
    require.Len(t, buffs, 2)
    require.Equal(t, int32(1068), buffs[0].SkillID)
    require.Equal(t, int32(3), buffs[0].Level)
    require.Equal(t, int32(1204), buffs[1].SkillID)
    require.Equal(t, int32(2), buffs[1].Level)

    // The npc dialog reached the latch and the tracker page.
    npcID, html := client.LastHTMLDialog()
    require.Equal(t, int32(7155), npcID)
    require.Contains(t, html, "Greetings")
    require.Equal(t, int32(7155), tracker.DialogOrigin())
    links := tracker.DialogLinks()
    require.Len(t, links, 2)
    require.Equal(t, "npc_7155_Chat 0", links[0].Command)
    require.Equal(t, "Talk", links[0].Text)
    require.Equal(t, "npc_7155_teleport 1 2", links[1].Command)

    // The truncated journal logged its parse failure.
    require.Contains(t, logBuf.String(), "Failed to parse quest list")
    require.Contains(t, logBuf.String(),
        "Quest journal with 2 quests, 2 quest items")
}

// quietFlow drains the connection until the client side closes.
func (s *fakeGameServer) quietFlow(
    conn net.Conn, _ *crypt.GameCrypt,
) {
    _, _ = io.Copy(io.Discard, conn)
}

// TestGameClientCloseLifecycle checks the client side close: the
// first Close answers nil, the second reports the already closed
// connection, and a send after the close fails instead of hanging.
func TestGameClientCloseLifecycle(t *testing.T) {
    server := startFakeGameServerFlow(t, (*fakeGameServer).quietFlow)

    conn, err := net.Dial("tcp", server.Addr())
    require.NoError(t, err)

    client, err := NewGameClient(conn)
    require.NoError(t, err)
    client.SetLogger(log.New(io.Discard, "", 0))

    require.NoError(t, client.Close())
    require.Error(t, client.Close(), "the second close reports")
    require.Error(t, client.Say("anyone there", 0, ""),
        "a send after the close must fail")
}

// TestGameClientReportsConnectionLoss closes the server side of a live
// session: Run must report the lost connection and take the tracker
// offline.
func TestGameClientReportsConnectionLoss(t *testing.T) {
    server := startFakeGameServerFlow(t, (*fakeGameServer).closeFlow)

    conn, err := net.Dial("tcp", server.Addr())
    require.NoError(t, err)

    client, err := NewGameClient(conn)
    require.NoError(t, err)
    client.SetLogger(log.New(io.Discard, "", 0))

    tracker := state.NewBot("dropped")
    client.SetTracker(tracker)
    tracker.SetCharacter("dropped", floodSelfID, 18,
        45000, 50000, -3500, 50, 30)

    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    err = client.Run(ctx, "dropped")
    require.Error(t, err)
    require.Contains(t, err.Error(), "game connection lost")
    require.Equal(t, state.StatusOffline, tracker.Snapshot().Status)
}

// TestEnsureCharacterReportsCreationFail drives the character creation
// into the server refusal: EnsureCharacter must surface the reason.
func TestEnsureCharacterReportsCreationFail(t *testing.T) {
    server := startFakeGameServerFlow(t, (*fakeGameServer).createFailFlow)

    conn, err := net.Dial("tcp", server.Addr())
    require.NoError(t, err)
    defer conn.Close()

    client, err := NewGameClient(conn)
    require.NoError(t, err)
    client.SetLogger(log.New(io.Discard, "", 0))

    _, err = client.EnsureCharacter(CharacterParams{
        Name:      "unittest1",
        Race:      1,
        Female:    0,
        ClassID:   18,
        HairStyle: 0,
        HairColor: 0,
        Face:      0,
    }, fromgameserver.NewCharSelectInfoPacket())
    require.Error(t, err)
    require.Contains(t, err.Error(), "name already exists")
}

// createFailFlow answers the character creation with a refusal.
func (s *fakeGameServer) createFailFlow(
    conn net.Conn, cipher *crypt.GameCrypt,
) {
    payload := s.readEncrypted(conn, cipher)
    require.Equal(s.t, byte(0x0B), payload[0])
    name := readUtf16String(payload[1:])
    require.Equal(s.t, "unittest1", name)

    reason := []byte{0x26, 0x02, 0x00, 0x00, 0x00} // name already exists
    s.writeEncrypted(conn, cipher, reason)
}

// closeFlow closes the connection right after the handshake.
func (s *fakeGameServer) closeFlow(
    conn net.Conn, _ *crypt.GameCrypt,
) {
    _ = conn.Close()
}
