// SPDX-FileCopyrightText: 2024 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package toauthserver

import (
	"testing"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

func BenchmarkRequestGGAuth_ToBytes(b *testing.B) {
	req := &RequestGGAuth{
		SessionID: 1,
		Data1:     2,
		Data2:     3,
		Data3:     4,
		Data4:     5,
	}

	b.ResetTimer()
	for range b.N {
		packetWriter := packet.NewWriter()
		err := req.ToBytes(packetWriter)
		if err != nil {
			b.Fatal(err)
		}
	}
}
