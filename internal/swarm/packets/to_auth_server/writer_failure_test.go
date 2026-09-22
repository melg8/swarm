// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package toauthserver

import (
    "errors"
    "testing"

    "github.com/melg8/swarm/internal/swarm/packets/packet"
    "github.com/stretchr/testify/require"
)

// The error propagation of the auth packet builders (the same walk
// the game server builders run, see the to_game_server package):
// every guarded write of every builder returns the armed failure in
// its turn - the counted failure arm (writer.FailWritesAfter) steps
// through the whole write sequence, one branch per pass.

var errInjected = errors.New("injected write failure")

// packetBuilder is the minimal contract the failure walk needs: any
// builder that serializes itself through the packet writer.
type packetBuilder interface {
    ToBytes(writer *packet.Writer) error
}

// TestEveryAuthBuilderPropagatesTheArmedWriteFailure walks the four
// auth builders: at every arm position the injected error must
// surface from ToBytes, and the walk ends on the first clean pass
// (the budget outlived the write count).
func TestEveryAuthBuilderPropagatesTheArmedWriteFailure(t *testing.T) {
    builders := []struct {
        name  string
        build func() packetBuilder
    }{
        {"RequestAuthLogin", func() packetBuilder {
            return &RequestAuthLogin{Account: "unittest1", Password: "pw"}
        }},
        {"RequestGGAuth", func() packetBuilder {
            return NewDefaultRequestGGAuth(0x11223344)
        }},
        {"RequestServerList", func() packetBuilder {
            return NewRequestServerList(1, 2)
        }},
        {"RequestServerLogin", func() packetBuilder {
            return &RequestServerLogin{LoginOkID1: 1, LoginOkID2: 2,
                ServerID: 1}
        }},
    }
    for _, b := range builders {
        t.Run(b.name, func(t *testing.T) {
            for budget := range 16 {
                writer := packet.NewWriter()
                writer.FailWritesAfter(budget, errInjected)
                err := b.build().ToBytes(writer)
                if err == nil {
                    break
                }
                require.ErrorIs(t, err, errInjected,
                    "the builder must propagate the armed write failure")
            }
        })
    }
}
