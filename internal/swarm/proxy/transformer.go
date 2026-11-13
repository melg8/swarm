// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package proxy

// Transformer rewrites or drops relayed packets between the game client and the
// real server. The proxy decrypts every packet and re-encrypts it on the
// direction specific cipher chain, so a transformation may change the payload
// size, drop or inject packets: each direction owns its chain, and a chain
// advances by what the proxy actually sends, never by what the other side sent.
// The current implementation is the transparent passthrough; the seam stays for
// the debugging server packet spoofing.
type Transformer interface {
    // ServerToClient transforms a server packet on its way to the
    // connected game client. Returning ok=false drops the packet.
    ServerToClient(payload []byte) (out []byte, ok bool)
    // ClientToServer transforms a client packet on its way to the real
    // game server. Returning ok=false drops the packet.
    ClientToServer(payload []byte) (out []byte, ok bool)
}

// PassthroughTransformer is the transparent default: every packet
// transits unchanged in both directions.
type PassthroughTransformer struct{}

func (PassthroughTransformer) ServerToClient(payload []byte) ([]byte, bool) {
    return payload, true
}

func (PassthroughTransformer) ClientToServer(payload []byte) ([]byte, bool) {
    return payload, true
}

// compile time interface check.
var _ Transformer = PassthroughTransformer{}
