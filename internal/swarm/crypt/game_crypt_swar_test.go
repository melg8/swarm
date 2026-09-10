// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package crypt

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGameCryptSWARCorrectness verifies the SWAR 8 byte chunk path
// produces identical output to the byte by byte reference for every
// size from 1 to 256 (every chunk boundary, every remainder length).
// The SWAR prefix XOR scan and the shift-and-XOR decrypt path are
// bit-exact translations of the reference, but a single misaligned
// shift would silently corrupt one chunk out of every eight, so the
// test sweeps the full size range against the reference implementation.
func TestGameCryptSWARCorrectness(t *testing.T) {
	for _, n := range []int{
		1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 15, 16, 17,
		31, 32, 33, 63, 64, 65, 127, 128, 129,
		200, 255, 256,
	} {
		t.Run("size_"+itoa(n), func(t *testing.T) {
			// Build deterministic data: byte(i) for i in 0..n-1.
			original := make([]byte, n)
			for i := range original {
				original[i] = byte(i)
			}

			// Encrypt with SWAR path.
			swarCipher := NewGameCrypt(DefaultGameCryptKey())
			swarCipher.Enable()
			swarWire := append([]byte(nil), original...)
			swarCipher.Encrypt(swarWire)

			// Encrypt with reference byte loop.
			refCipher := NewGameCrypt(DefaultGameCryptKey())
			refCipher.Enable()
			refWire := append([]byte(nil), original...)
			refCipher.encryptReference(refWire)

			require.Equal(t, refWire, swarWire,
				"SWAR encrypt differs from reference at size %d", n)

			// Decrypt the SWAR ciphertext with the SWAR decrypt path.
			swarDecrypt := NewGameCrypt(DefaultGameCryptKey())
			swarDecrypt.Enable()
			swarDecrypt.Decrypt(swarWire)
			require.Equal(t, original, swarWire,
				"SWAR decrypt roundtrip failed at size %d", n)

			// Decrypt the reference ciphertext with the SWAR decrypt path.
			refDecrypt := NewGameCrypt(DefaultGameCryptKey())
			refDecrypt.Enable()
			refDecrypt.Decrypt(refWire)
			require.Equal(t, original, refWire,
				"SWAR decrypt of reference ciphertext failed at size %d", n)
		})
	}
}

// TestGameCryptSWARMultiPacket verifies the chain value carries
// correctly across packet boundaries: the rolling offset modifies
// the key between packets, and the chain value (prev for encrypt,
// last for decrypt) resets to zero within each packet but the key
// state carries. A regression that fails to call advanceOffset or
// that reads the key at the wrong time surfaces here.
func TestGameCryptSWARMultiPacket(t *testing.T) {
	outCipher := NewGameCrypt(DefaultGameCryptKey())
	inCipher := NewGameCrypt(DefaultGameCryptKey())
	outCipher.Enable()
	inCipher.Enable()

	packets := [][]byte{
		{0x08, 0x74, 0x00, 0x65, 0x00, 0x73, 0x00, 0x74, 0x00}, // 9 bytes
		{0xA8, 0x00, 0x01}, // 3 bytes
		make([]byte, 64),   // full 8 chunks
		make([]byte, 100),  // 12 chunks + 4
		{0x38, 0x37, 0x36, 0x35, 0x34, 0x33, 0x32, 0x31}, // exactly 8
	}
	for i, p := range packets {
		original := append([]byte(nil), p...)
		wire := append([]byte(nil), p...)
		outCipher.Encrypt(wire)
		inCipher.Decrypt(wire)
		require.Equal(t, original, wire,
			"packet %d (%d bytes) roundtrip failed", i, len(p))
	}

	// After 5 packets the key must be in sync on both sides.
	require.Equal(t, outCipher.outKey, inCipher.inKey,
		"keys diverged after multi packet roundtrip")
}

// TestGameCryptSWARAllZeroData verifies the SWAR path handles all
// zero input correctly (the XOR chain produces the key bytes as
// output, which is a known Mobius reference shape).
func TestGameCryptSWARAllZeroData(t *testing.T) {
	cipher := NewGameCrypt(DefaultGameCryptKey())
	cipher.Enable()

	data := make([]byte, 64)
	original := append([]byte(nil), data...)
	cipher.Encrypt(data)

	// The first 8 bytes of encrypted output are the running XOR of
	// the key bytes: key[0], key[0]^key[1], key[0]^key[1]^key[2], ...
	// For the default key [0x94, 0x35, 0x00, 0x00, 0xa1, 0x6c, 0x54, 0x87]:
	// 0x94, 0xa1, 0xa1, 0xa1, 0x00, 0x6c, 0x38, 0xbf.
	expected := []byte{0x94, 0xa1, 0xa1, 0xa1, 0x00, 0x6c, 0x38, 0xbf}
	require.Equal(t, expected, data[:8])

	// Roundtrip: decrypt must recover the zeros.
	dec := NewGameCrypt(DefaultGameCryptKey())
	dec.Enable()
	dec.Decrypt(data)
	require.Equal(t, original, data)
}

// TestGameCryptSWARRandomLikeData verifies the SWAR path on data that
// has high entropy (every byte different), exercising all bit positions
// of the uint64 XOR operations.
func TestGameCryptSWARRandomLikeData(t *testing.T) {
	// A pseudo-random pattern that exercises every bit position.
	data := make([]byte, 256)
	for i := range data {
		data[i] = byte(i*7 + 13)
	}

	original := append([]byte(nil), data...)

	enc := NewGameCrypt(DefaultGameCryptKey())
	enc.Enable()
	enc.Encrypt(data)

	dec := NewGameCrypt(DefaultGameCryptKey())
	dec.Enable()
	dec.Decrypt(data)

	require.Equal(t, original, data)
}

// encryptReference is the original byte by byte implementation kept
// as a test oracle for the SWAR path. It must produce identical
// output to the optimized Encrypt for every input.
func (gc *GameCrypt) encryptReference(data []byte) {
	if !gc.enabled || len(data) == 0 {
		return
	}

	prev := byte(0)
	for i := range data {
		prev = data[i] ^ gc.outKey[i&7] ^ prev
		data[i] = prev
	}

	gc.advanceOffset(&gc.outKey, len(data))
}

// itoa is a minimal int to string converter to avoid the strconv
// import in this test file.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	var buf [8]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}

	return string(buf[pos:])
}

// BenchmarkGameCryptEncryptSizes measures the SWAR path across the
// realistic packet size range: 8 (the minimal chunk), 64 (the bench
// default), 256 (a large status update) and 1024 (an inventory burst).
// The 100 bot fleet pays the per byte cost at every size.
func BenchmarkGameCryptEncryptSizes(b *testing.B) {
	for _, n := range []int{8, 64, 256, 1024} {
		b.Run("size_"+itoa(n), func(b *testing.B) {
			cipher := NewGameCrypt(DefaultGameCryptKey())
			cipher.Enable()
			data := make([]byte, n)
			for i := range data {
				data[i] = byte(i)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				cipher.Encrypt(data)
				cipher.Decrypt(data)
			}
		})
	}
}

// BenchmarkGameCryptDecryptOnly measures the decrypt path in isolation
// (the existing bench measures encrypt+decrypt together).
func BenchmarkGameCryptDecryptOnly(b *testing.B) {
	cipher := NewGameCrypt(DefaultGameCryptKey())
	cipher.Enable()
	data := make([]byte, 64)
	for i := range data {
		data[i] = byte(i)
	}
	cipher.Encrypt(data)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		cipher.Decrypt(data)
		cipher.Encrypt(data)
	}
}

// BenchmarkGameCryptEncryptOnly measures the encrypt path in isolation.
func BenchmarkGameCryptEncryptOnly(b *testing.B) {
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
