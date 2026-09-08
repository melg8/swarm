// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package proxy

// Transformer rewrites or drops relayed packets between the game client
// and the real game server. The proxy decrypts every packet and
// re-encrypts it on the direction specific cipher chain, so the
// transformation is free to change the payload size, drop packets or
// inject new ones: each direction owns its chain and the chains advance
// by what the proxy actually sends, never by what the other side sent.
//
// This is the seam for the future server packet spoofing used for
// debugging (the task brief: "swarm может подменять часть пакетов от
// сервера в целях отладки"). The current implementation is the
// transparent passthrough of the MITM mode: packets transit unchanged.
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

// ServerToClient passes the payload through unchanged.
func (PassthroughTransformer) ServerToClient(payload []byte) ([]byte, bool) {
	return payload, true
}

// ClientToServer passes the payload through unchanged.
func (PassthroughTransformer) ClientToServer(payload []byte) ([]byte, bool) {
	return payload, true
}

// compile time interface check.
var _ Transformer = PassthroughTransformer{}
