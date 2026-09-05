// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package crypt

import "testing"

func BenchmarkGameCryptEncrypt(b *testing.B) {
	cipher := NewGameCrypt(DefaultGameCryptKey())
	cipher.Enable()
	data := make([]byte, 64)
	for i := range data {
		data[i] = byte(i)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		cipher.Encrypt(data)
		cipher.Decrypt(data)
	}
}

func BenchmarkGameCryptDecrypt(b *testing.B) {
	cipher := NewGameCrypt(DefaultGameCryptKey())
	cipher.Enable()
	data := make([]byte, 64)
	for i := range data {
		data[i] = byte(i)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		cipher.Decrypt(data)
		cipher.Encrypt(data)
	}
}
