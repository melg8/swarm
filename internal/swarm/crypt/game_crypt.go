// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package crypt

import (
	"encoding/binary"
)

// GameCryptKeySize is the size of the game protocol encryption key.
const GameCryptKeySize = 8

// DefaultGameCryptKey is the static key used by the Mobius C1 game server.
// The first 4 bytes hold the rolling offset (little endian int), the last
// 4 bytes are fixed.
func DefaultGameCryptKey() [GameCryptKeySize]byte {
	return [GameCryptKeySize]byte{
		0x94, 0x35, 0x00, 0x00, 0xa1, 0x6c, 0x54, 0x87,
	}
}

// GameCrypt implements the stateful XOR cipher of the Mobius game protocol.
// It mirrors the server side Encryption implementation: 8 byte key with a
// rolling offset stored in bytes [0..3], running XOR chain over payload
// bytes and per packet offset advance by the payload size.
type GameCrypt struct {
	inKey   [GameCryptKeySize]byte
	outKey  [GameCryptKeySize]byte
	enabled bool
}

// NewGameCrypt creates a game cipher from the session key. The cipher
// starts disabled: the first Enable call activates it.
func NewGameCrypt(key [GameCryptKeySize]byte) *GameCrypt {
	return &GameCrypt{
		inKey:   key,
		outKey:  key,
		enabled: false,
	}
}

// Enable activates encryption and decryption.
func (gc *GameCrypt) Enable() {
	gc.enabled = true
}

// Enabled reports whether the cipher processes data.
func (gc *GameCrypt) Enabled() bool {
	return gc.enabled
}

// Encrypt transforms outbound payload bytes in place.
//
// The cipher is a running XOR chain: out[i] = data[i] ^ key[i&7] ^ out[i-1].
// The byte loop processes one byte at a time, which on the 100 bot fleet
// path means ~100 bytes per packet times ~100 packets per second per bot
// = 1M byte iterations per second just for the game cipher. The optimized
// form processes 8 byte chunks through a SWAR (SIMD Within A Register)
// prefix XOR scan: the key repeats every 8 bytes, so a full chunk XORs
// with one uint64 key load, then a three step shift-and-XOR prefix scan
// (8, 16, 32 bit shifts) produces the running XOR of all 8 bytes in one
// register, and the chain value from the previous chunk broadcasts into
// every byte through a multiply by 0x0101010101010101. The remainder tail
// (1 to 7 bytes) falls back to the byte loop.
func (gc *GameCrypt) Encrypt(data []byte) {
	if !gc.enabled || len(data) == 0 {
		return
	}

	key := binary.LittleEndian.Uint64(gc.outKey[:])
	prev := byte(0)
	n := len(data)
	chunks := n / 8

	// Process 8 byte chunks with the SWAR prefix XOR scan.
	for c := range chunks {
		i := c * 8
		x := binary.LittleEndian.Uint64(data[i:]) ^ key

		// Prefix XOR scan: after these three steps, byte i of x
		// holds the XOR of the original x[0..i]. Verified against
		// the byte loop on the full packet corpus.
		x ^= x << 8
		x ^= x << 16
		x ^= x << 32

		// The chain value from the previous chunk XORs into every
		// byte (it flows through the chain), so broadcast it and
		// XOR once.
		x ^= uint64(prev) * bitsBroadcast

		binary.LittleEndian.PutUint64(data[i:], x)

		// The new chain value is the last output byte.
		prev = byte(x >> 56)
	}

	// Process the remainder tail byte by byte.
	for i := chunks * 8; i < n; i++ {
		prev = data[i] ^ gc.outKey[i&7] ^ prev
		data[i] = prev
	}

	gc.advanceOffset(&gc.outKey, n)
}

// Decrypt transforms inbound payload bytes in place.
//
// The decrypt chain uses the ENCRYPTED (input) bytes as the chain value,
// not the output: out[i] = enc[i] ^ key[i&7] ^ enc[i-1]. This is simpler
// than encrypt: XOR the chunk with the key, then XOR each byte with the
// previous ENCRYPTED byte (which is just the original input shifted by 8
// bits within the register). The chain value from the previous chunk goes
// into byte 0 through an OR with the shifted register.
func (gc *GameCrypt) Decrypt(data []byte) {
	if !gc.enabled || len(data) == 0 {
		return
	}

	key := binary.LittleEndian.Uint64(gc.inKey[:])
	last := byte(0)
	n := len(data)
	chunks := n / 8

	// Process 8 byte chunks.
	for c := range chunks {
		i := c * 8
		enc := binary.LittleEndian.Uint64(data[i:])
		x := enc ^ key

		// Each output byte XORs with the previous ENCRYPTED byte.
		// Shifting enc left by 8 bits puts enc[i-1] at byte i's
		// position; byte 0 gets 0, which we replace with the chain
		// value from the previous chunk.
		shifted := enc<<8 | uint64(last)
		out := x ^ shifted

		binary.LittleEndian.PutUint64(data[i:], out)

		// The chain value for the next chunk is the last encrypted
		// byte (the input, not the output).
		last = byte(enc >> 56)
	}

	// Process the remainder tail byte by byte.
	for i := chunks * 8; i < n; i++ {
		enc := data[i]
		data[i] = enc ^ gc.inKey[i&7] ^ last
		last = enc
	}

	gc.advanceOffset(&gc.inKey, n)
}

// bitsBroadcast is the multiplier that replicates a single byte into
// all 8 bytes of a uint64: 0x0101010101010101. Used by the encrypt
// SWAR path to broadcast the chain value into every byte of the
// prefix XOR scan result.
const bitsBroadcast uint64 = 0x0101010101010101

// advanceOffset adds size to the little endian int stored at key[0..3].
func (gc *GameCrypt) advanceOffset(key *[GameCryptKeySize]byte, size int) {
	offset := binary.LittleEndian.Uint32(key[0:4])
	offset += uint32(size)
	binary.LittleEndian.PutUint32(key[0:4], offset)
}
