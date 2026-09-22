// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package botlog_test

import (
    "strings"
    "testing"

    "github.com/melg8/swarm/internal/swarm/acceptance/botlog"
    "github.com/melg8/swarm/internal/swarm/packets/packet"
    togameserver "github.com/melg8/swarm/internal/swarm/packets/to_game_server"
)

// The send wire format round of issue #9: the client request
// decoders run against the production serializers of the
// to_game_server package - every line is pinned against the real
// wire bytes the session writes, not a hand built twin of them.

// byteSerializer is the ToBytes contract every send packet carries.
type byteSerializer interface {
    ToBytes(writer *packet.Writer) error
}

// sendLinesOf opens a botlog in a temp dir, feeds the packet and
// returns the rendered body.
func sendLinesOf(t *testing.T, payload []byte) string {
    t.Helper()
    log, err := botlog.Open(t.TempDir(), botlog.Header{TestID: "t"})
    if err != nil {
        t.Fatalf("open: %v", err)
    }
    log.Send(payload)
    log.Close()

    return readLog(t, log.Path())
}

// sendBytesOf serializes one send packet through its production
// serializer.
func sendBytesOf(t *testing.T, pkt byteSerializer) []byte {
    t.Helper()
    writer := packet.NewWriter()
    if err := pkt.ToBytes(writer); err != nil {
        t.Fatalf("serialize: %v", err)
    }

    return writer.Bytes()
}

func TestSendDecodeSessionFamily(t *testing.T) {
    t.Run("protocol version", func(t *testing.T) {
        body := sendLinesOf(t, sendBytesOf(t,
            &togameserver.ProtocolVersion{Version: 746}))
        if !strings.Contains(body, "protocol version 746") {
            t.Errorf("the version line misses the number: %s", body)
        }
    })

    t.Run("enter world", func(t *testing.T) {
        body := sendLinesOf(t, sendBytesOf(t, &togameserver.EnterWorld{}))
        if !strings.Contains(body, "enter world") {
            t.Errorf("the enter world line is wrong: %s", body)
        }
    })

    t.Run("logout", func(t *testing.T) {
        body := sendLinesOf(t, sendBytesOf(t, &togameserver.Logout{}))
        if !strings.Contains(body, "logout") {
            t.Errorf("the logout line is wrong: %s", body)
        }
    })

    t.Run("appearing", func(t *testing.T) {
        body := sendLinesOf(t,
            sendBytesOf(t, togameserver.NewAppearingPacket()))
        if !strings.Contains(body, "appearing") {
            t.Errorf("the appearing line is wrong: %s", body)
        }
    })
}

func TestSendDecodeActionClick(t *testing.T) {
    t.Run("plain click", func(t *testing.T) {
        body := sendLinesOf(t, sendBytesOf(t,
            &togameserver.ActionRequestPacket{
                ObjectID: 20538,
                X:        43896,
                Y:        54088,
                Z:        -3640,
                Shift:    0,
            }))
        want := "click object=20538 at=(43896,54088,-3640)"
        if !strings.Contains(body, want) {
            t.Errorf("the click line misses the target: %s", body)
        }
        if strings.Contains(body, "shift") {
            t.Errorf("the plain click carries no shift marker: %s", body)
        }
    })

    t.Run("shift click", func(t *testing.T) {
        body := sendLinesOf(t, sendBytesOf(t,
            &togameserver.ActionRequestPacket{
                ObjectID: 20538,
                X:        43896,
                Y:        54088,
                Z:        -3640,
                Shift:    1,
            }))
        if !strings.Contains(body, " shift") {
            t.Errorf("the shift click misses the marker: %s", body)
        }
    })
}

func TestSendDecodeSessionHandoverRequests(t *testing.T) {
    t.Run("auth login", func(t *testing.T) {
        body := sendLinesOf(t, sendBytesOf(t,
            &togameserver.AuthLogin{Login: "temp1"}))
        if !strings.Contains(body, "auth login account=temp1") {
            t.Errorf("the auth line misses the account: %s", body)
        }
    })

    t.Run("character create", func(t *testing.T) {
        body := sendLinesOf(t, sendBytesOf(t,
            &togameserver.CharacterCreate{
                Name:      "Yumi",
                Race:      1,
                Female:    1,
                ClassID:   18,
                INT:       23,
                STR:       36,
                CON:       36,
                MEN:       25,
                DEX:       35,
                WIT:       14,
                HairStyle: 1,
                HairColor: 2,
                Face:      1,
            }))
        if !strings.Contains(body, "character create name=Yumi") {
            t.Errorf("the create line misses the name: %s", body)
        }
    })

    t.Run("character select", func(t *testing.T) {
        body := sendLinesOf(t, sendBytesOf(t,
            &togameserver.CharacterSelect{CharSlot: 0}))
        if !strings.Contains(body, "character select slot=0") {
            t.Errorf("the select line misses the slot: %s", body)
        }
    })
}

func TestSendDecodeItemRequests(t *testing.T) {
    t.Run("drop item", func(t *testing.T) {
        body := sendLinesOf(t, sendBytesOf(t,
            &togameserver.RequestDropItem{
                ObjectID: 3001,
                Count:    100,
                X:        45200,
                Y:        50200,
                Z:        -3480,
            }))
        want := "drop item object=3001 count=100 at=(45200,50200,-3480)"
        if !strings.Contains(body, want) {
            t.Errorf("the drop line misses the stack: %s", body)
        }
    })

    t.Run("use item", func(t *testing.T) {
        body := sendLinesOf(t, sendBytesOf(t,
            &togameserver.RequestUseItem{ObjectID: 3002}))
        if !strings.Contains(body, "use item object=3002") {
            t.Errorf("the use line misses the object: %s", body)
        }
    })

    t.Run("destroy item", func(t *testing.T) {
        body := sendLinesOf(t, sendBytesOf(t,
            &togameserver.RequestDestroyItem{ObjectID: 3003, Count: 5}))
        if !strings.Contains(body, "destroy item object=3003 count=5") {
            t.Errorf("the destroy line misses the stack: %s", body)
        }
    })

    t.Run("run stance", func(t *testing.T) {
        body := sendLinesOf(t, sendBytesOf(t,
            &togameserver.ChangeMoveTypePacket{TypeRun: 1}))
        if !strings.Contains(body, "change move type run") {
            t.Errorf("the stance line misses the run word: %s", body)
        }
    })

    t.Run("walk stance", func(t *testing.T) {
        body := sendLinesOf(t, sendBytesOf(t,
            &togameserver.ChangeMoveTypePacket{TypeRun: 0}))
        if !strings.Contains(body, "change move type walk") {
            t.Errorf("the stance line misses the walk word: %s", body)
        }
    })
}

func TestSendDecodeShopBatches(t *testing.T) {
    t.Run("sell batch", func(t *testing.T) {
        body := sendLinesOf(t, sendBytesOf(t,
            &togameserver.RequestSellItemPacket{
                ListID: togameserver.SellListIDCustom,
                Items: []togameserver.SellItemEntry{
                    {ObjectID: 3001, ItemID: 57, Count: 500},
                    {ObjectID: 3004, ItemID: 17, Count: 40},
                },
            }))
        if !strings.Contains(body, "sell item list=0 entries=2") {
            t.Errorf("the sell line misses the count: %s", body)
        }
        want := "  sell Adena (item 57, object 3001) x500"
        if !strings.Contains(body, want) {
            t.Errorf("the sell batch misses the adena: %s", body)
        }
        want = "  sell Wooden Arrow (item 17, object 3004) x40"
        if !strings.Contains(body, want) {
            t.Errorf("the sell batch misses the arrows: %s", body)
        }
    })

    t.Run("buy batch", func(t *testing.T) {
        body := sendLinesOf(t, sendBytesOf(t,
            &togameserver.RequestBuyItemPacket{
                ListID: 15,
                Items: []togameserver.BuyItemEntry{
                    {ItemID: 17, Count: 50},
                    {ItemID: 1060, Count: 1},
                },
            }))
        if !strings.Contains(body, "buy item list=15 entries=2") {
            t.Errorf("the buy line misses the count: %s", body)
        }
        if !strings.Contains(body, "  buy Wooden Arrow (item 17) x50") {
            t.Errorf("the buy batch misses the arrows: %s", body)
        }
    })
}

func TestSendDecodeBypassAndSkillRequests(t *testing.T) {
    t.Run("bypass command", func(t *testing.T) {
        body := sendLinesOf(t, sendBytesOf(t,
            &togameserver.RequestBypassToServer{
                Command: "teleport request",
            }))
        if !strings.Contains(body, `bypass "teleport request"`) {
            t.Errorf("the bypass line misses the command: %s", body)
        }
    })

    t.Run("skill cast", func(t *testing.T) {
        body := sendLinesOf(t, sendBytesOf(t,
            &togameserver.RequestMagicSkillUsePacket{
                SkillID: 1177,
                Ctrl:    true,
                Shift:   false,
            }))
        want := "cast Wind Strike (skill 1177) ctrl"
        if !strings.Contains(body, want) {
            t.Errorf("the cast line misses the skill name: %s", body)
        }
    })

    t.Run("action use", func(t *testing.T) {
        body := sendLinesOf(t, sendBytesOf(t,
            &togameserver.RequestActionUsePacket{
                ActionID: 0,
                Ctrl:     false,
                Shift:    false,
            }))
        if !strings.Contains(body, "action use id=0") {
            t.Errorf("the action use line misses the id: %s", body)
        }
    })

    t.Run("acquire skill", func(t *testing.T) {
        body := sendLinesOf(t, sendBytesOf(t,
            &togameserver.RequestAcquireSkillPacket{SkillID: 3, Level: 3}))
        want := "learn Power Strike (skill 3 level 3)"
        if !strings.Contains(body, want) {
            t.Errorf("the learn line misses the skill: %s", body)
        }
    })

    t.Run("restart point village", func(t *testing.T) {
        body := sendLinesOf(t, sendBytesOf(t,
            &togameserver.RequestRestartPointPacket{PointType: 0}))
        if !strings.Contains(body, "restart point village") {
            t.Errorf("the restart line misses the village: %s", body)
        }
    })

    t.Run("restart point agathion", func(t *testing.T) {
        body := sendLinesOf(t, sendBytesOf(t,
            &togameserver.RequestRestartPointPacket{PointType: 1}))
        if !strings.Contains(body, "restart point agathion") {
            t.Errorf("the restart line misses the agathion: %s", body)
        }
    })
}

func TestSendDecodeValidatePosition(t *testing.T) {
    body := sendLinesOf(t, sendBytesOf(t,
        &togameserver.ValidatePositionPacket{
            X:       45321,
            Y:       50321,
            Z:       -3480,
            Heading: 16384,
            Vehicle: 0,
        }))
    want := "validate position at=(45321,50321,-3480) heading=16384"
    if !strings.Contains(body, want) {
        t.Errorf("the validate line misses the claim: %s", body)
    }
}

func TestSendDecodeUnknownAndTruncated(t *testing.T) {
    t.Run("unknown opcode", func(t *testing.T) {
        body := sendLinesOf(t, []byte{0x63, 1, 2, 3})
        if !strings.Contains(body, "unknown send packet, 4 bytes") {
            t.Errorf("the unknown line misses the size: %s", body)
        }
    })

    t.Run("truncated cast", func(t *testing.T) {
        body := sendLinesOf(t, []byte{0x2F})
        if !strings.Contains(body, "magic skill use (short payload)") {
            t.Errorf("the cast must fall back to the short line: %s", body)
        }
    })

    t.Run("truncated sell batch", func(t *testing.T) {
        body := sendLinesOf(t, []byte{0x1E})
        if !strings.Contains(body, "sell item (short payload)") {
            t.Errorf("the sell must fall back to the short line: %s", body)
        }
    })

    t.Run("truncated validate", func(t *testing.T) {
        body := sendLinesOf(t, []byte{0x48})
        if !strings.Contains(body, "validate position (short payload)") {
            t.Errorf("the validate must fall back to the short line: %s",
                body)
        }
    })
}
