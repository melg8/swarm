// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
    "errors"
    "testing"

    "github.com/melg8/swarm/internal/swarm/packets/packet"
    "github.com/stretchr/testify/require"
)

// The error propagation of the packet builders: every guarded write
// of every builder returns the failure once the writer is armed
// (writer.FailWrites - a memory buffer write never fails on its own,
// so the failure injection is the only exercise those branches get).
// The armed error must surface instead of a silent partial packet,
// whatever write of the sequence trips first.

var errInjected = errors.New("injected write failure")

// packetBuilder is the minimal contract the failure walk needs: any
// builder that serializes itself through the packet writer.
type packetBuilder interface {
    ToBytes(writer *packet.Writer) error
}

// builder pairs a packet builder with the wire name the failure test
// reports.
type builder struct {
    name  string
    build func() packetBuilder
}

// allBuilders enumerates every request builder of the package (the
// struct literals cover the ones without a constructor).
func allBuilders() []builder {
    return []builder{
        {"ProtocolVersion", func() packetBuilder { return &ProtocolVersion{} }},
        {"AuthLogin", func() packetBuilder {
            return &AuthLogin{Login: "unittest1", PlayOkID1: 1,
                PlayOkID2: 2, LoginOkID1: 3, LoginOkID2: 4}
        }},
        {"CharacterCreate", func() packetBuilder {
            return &CharacterCreate{Name: "name"}
        }},
        {"CharacterSelect", func() packetBuilder { return &CharacterSelect{} }},
        {"EnterWorld", func() packetBuilder { return &EnterWorld{} }},
        {"RequestNetPing", func() packetBuilder { return &RequestNetPing{} }},
        {"Logout", func() packetBuilder { return &Logout{} }},
        {"AttackRequest", func() packetBuilder {
            return NewAttackRequestPacket()
        }},
        {"ActionRequest", func() packetBuilder {
            return NewActionRequestPacket()
        }},
        {"RequestDestroyItem", func() packetBuilder {
            return NewRequestDestroyItem()
        }},
        {"Appearing", func() packetBuilder { return NewAppearingPacket() }},
        {"RequestUseItem", func() packetBuilder { return NewRequestUseItem() }},
        {"RequestDropItem", func() packetBuilder {
            return NewRequestDropItem()
        }},
        {"MoveToLocation", func() packetBuilder {
            return NewMoveToLocationRequestPacket()
        }},
        {"RequestAcquireSkill", func() packetBuilder {
            return NewRequestAcquireSkillPacket()
        }},
        {"RequestActionUse", func() packetBuilder {
            return NewRequestActionUsePacket()
        }},
        {"RequestBuyItem", func() packetBuilder {
            return NewRequestBuyItemPacket()
        }},
        {"RequestBypassToServer", func() packetBuilder {
            return &RequestBypassToServer{Command: "player_help"}
        }},
        {"RequestMagicSkillUse", func() packetBuilder {
            return NewRequestMagicSkillUsePacket()
        }},
        {"RequestRestartPoint", func() packetBuilder {
            return NewRequestRestartPointPacket()
        }},
        {"RequestSellItem", func() packetBuilder {
            return NewRequestSellItemPacket()
        }},
        {"Say", func() packetBuilder {
            return NewSay("hello", SayChannelGeneral)
        }},
        {"ValidatePosition", func() packetBuilder {
            return NewValidatePositionPacket()
        }},
        {"ChangeMoveType", func() packetBuilder {
            return NewChangeMoveTypePacket()
        }},
    }
}

// TestEveryBuilderPropagatesTheArmedWriteFailure walks every builder
// with the counted failure (FailWritesAfter) stepping through the
// whole write sequence: arming the failure at write 1, 2, 3, ... -
// every guarded write of the builder must return the injected error
// in its turn, no builder may swallow it, panic through it or report
// success over a partial packet. The walk stops at the first clean
// pass (the budget outlived the write count - the sequence is fully
// walked).
func TestEveryBuilderPropagatesTheArmedWriteFailure(t *testing.T) {
    for _, b := range allBuilders() {
        t.Run(b.name, func(t *testing.T) {
            for budget := range maxBuilderWrites {
                writer := packet.NewWriter()
                writer.FailWritesAfter(budget, errInjected)
                err := b.build().ToBytes(writer)
                if err == nil {
                    require.Positive(t, budget,
                        "a builder that fails with a clean writer "+
                            "validates its input first - the walk "+
                            "still needs one armed pass")

                    break
                }
                require.ErrorIs(t, err, errInjected,
                    "the builder must propagate the armed write failure")
            }
        })
    }
}

// maxBuilderWrites bounds the write count of any single builder: the
// walk stops earlier on the first clean pass, the bound only keeps a
// broken builder from looping forever.
const maxBuilderWrites = 32

// TestSayBuilderFailingMidSequence arms the failure on a VALID say
// packet only after the first write passed: the say builder validates
// first, writes second - the injected failure must still surface
// from the write sequence, not from the validation.
func TestSayBuilderFailingMidSequence(t *testing.T) {
    request := NewSay("valid text", SayChannelGeneral)
    require.NoError(t, request.Validate())

    writer := packet.NewWriter()
    writer.FailWritesAfter(1, errInjected)
    require.ErrorIs(t, request.ToBytes(writer), errInjected)
}
