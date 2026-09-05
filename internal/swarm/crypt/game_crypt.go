// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package crypt

import "encoding/binary"

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
func (gc *GameCrypt) Encrypt(data []byte) {
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

// Decrypt transforms inbound payload bytes in place.
func (gc *GameCrypt) Decrypt(data []byte) {
	if !gc.enabled || len(data) == 0 {
		return
	}

	last := byte(0)
	for i := range data {
		enc := data[i]
		data[i] = enc ^ gc.inKey[i&7] ^ last
		last = enc
	}
	gc.advanceOffset(&gc.inKey, len(data))
}

// advanceOffset adds size to the little endian int stored at key[0..3].
func (gc *GameCrypt) advanceOffset(key *[GameCryptKeySize]byte, size int) {
	offset := binary.LittleEndian.Uint32(key[0:4])
	offset += uint32(size) //nolint:gosec
	binary.LittleEndian.PutUint32(key[0:4], offset)
}
