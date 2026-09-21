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

// builder pairs a packet builder with the wire name the failure test
// reports.
type builder struct {
    name  string
    build func() interface {
        ToBytes(writer *packet.Writer) error
    }
}

// allBuilders enumerates every request builder of the package (the
// struct literals cover the ones without a constructor).
func allBuilders() []builder {
    return []builder{
        {"ProtocolVersion", func() interface{ ToBytes(writer *packet.Writer) error } {
            return &ProtocolVersion{}
        }},
        {"AuthLogin", func() interface{ ToBytes(writer *packet.Writer) error } {
            return &AuthLogin{Login: "unittest1", PlayOkID1: 1,
                PlayOkID2: 2, LoginOkID1: 3, LoginOkID2: 4}
        }},
        {"CharacterCreate", func() interface{ ToBytes(writer *packet.Writer) error } {
            return &CharacterCreate{Name: "name"}
        }},
        {"CharacterSelect", func() interface{ ToBytes(writer *packet.Writer) error } {
            return &CharacterSelect{}
        }},
        {"EnterWorld", func() interface{ ToBytes(writer *packet.Writer) error } {
            return &EnterWorld{}
        }},
        {"RequestNetPing", func() interface{ ToBytes(writer *packet.Writer) error } {
            return &RequestNetPing{}
        }},
        {"Logout", func() interface{ ToBytes(writer *packet.Writer) error } {
            return &Logout{}
        }},
        {"AttackRequest", func() interface{ ToBytes(writer *packet.Writer) error } {
            return NewAttackRequestPacket()
        }},
        {"ActionRequest", func() interface{ ToBytes(writer *packet.Writer) error } {
            return NewActionRequestPacket()
        }},
        {"RequestDestroyItem", func() interface{ ToBytes(writer *packet.Writer) error } {
            return NewRequestDestroyItem()
        }},
        {"Appearing", func() interface{ ToBytes(writer *packet.Writer) error } {
            return NewAppearingPacket()
        }},
        {"RequestUseItem", func() interface{ ToBytes(writer *packet.Writer) error } {
            return NewRequestUseItem()
        }},
        {"RequestDropItem", func() interface{ ToBytes(writer *packet.Writer) error } {
            return NewRequestDropItem()
        }},
        {"MoveToLocation", func() interface{ ToBytes(writer *packet.Writer) error } {
            return NewMoveToLocationRequestPacket()
        }},
        {"RequestAcquireSkill", func() interface{ ToBytes(writer *packet.Writer) error } {
            return NewRequestAcquireSkillPacket()
        }},
        {"RequestActionUse", func() interface{ ToBytes(writer *packet.Writer) error } {
            return NewRequestActionUsePacket()
        }},
        {"RequestBuyItem", func() interface{ ToBytes(writer *packet.Writer) error } {
            return NewRequestBuyItemPacket()
        }},
        {"RequestBypassToServer", func() interface{ ToBytes(writer *packet.Writer) error } {
            return &RequestBypassToServer{Command: "player_help"}
        }},
        {"RequestMagicSkillUse", func() interface{ ToBytes(writer *packet.Writer) error } {
            return NewRequestMagicSkillUsePacket()
        }},
        {"RequestRestartPoint", func() interface{ ToBytes(writer *packet.Writer) error } {
            return NewRequestRestartPointPacket()
        }},
        {"RequestSellItem", func() interface{ ToBytes(writer *packet.Writer) error } {
            return NewRequestSellItemPacket()
        }},
        {"Say", func() interface{ ToBytes(writer *packet.Writer) error } {
            return NewSay("hello", SayChannelGeneral)
        }},
        {"ValidatePosition", func() interface{ ToBytes(writer *packet.Writer) error } {
            return NewValidatePositionPacket()
        }},
        {"ChangeMoveType", func() interface{ ToBytes(writer *packet.Writer) error } {
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
