// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package crypt

import (
	"testing"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
	toauthserver "github.com/melg8/swarm/internal/swarm/packets/to_auth_server"
)

func BenchmarkEncryptor_Write(b *testing.B) {
	writer := packet.NewWriter()
	writer.Buffer.Grow(100)
	cipher := DefaultAuthKey()
	encryptor := NewEncryptor(*writer, cipher)
	packet := toauthserver.NewDefaultRequestGGAuth(1)

	b.ResetTimer()

	for range b.N {
		writer.Reset()
		err := encryptor.Write(packet)
		if err != nil {
			b.Fatal(err)
		}

		if len(encryptor.Bytes()) != 34 {
			b.Fatal("Encrypted data should be 34 bytes, got: ", len(encryptor.Bytes()))
		}
	}
}
