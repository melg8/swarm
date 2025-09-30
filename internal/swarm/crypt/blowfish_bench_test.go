// SPDX-FileCopyrightText: 2024 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package crypt

import (
	"testing"
)

func dataForBlowfishBenchmark(size int) []byte {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}

	return data
}

func BenchmarkBlowfish(b *testing.B) {
	data := dataForBlowfishBenchmark(1000000)
	authKey := DefaultAuthKey()

	b.ResetTimer()

	for range b.N {
		err := authKey.DecryptInplace(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBlowfishEncrypt(b *testing.B) {
	data := dataForBlowfishBenchmark(1000000)
	authKey := DefaultAuthKey()

	b.ResetTimer()

	for range b.N {
		err := authKey.EncryptInplace(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}
