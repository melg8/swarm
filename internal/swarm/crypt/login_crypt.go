// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package crypt

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const (
	loginChecksumSize = 4
	loginAlignBy      = 8
	loginHeaderSize   = 2
	loginMaxPacket    = 65535
)

// LoginCrypt implements packet framing of the Mobius login protocol.
// Wire layout is [size:2 little endian][blowfish(content+filler+crc32le)]
// where the payload is aligned to 8 bytes and the little endian XOR checksum
// occupies the last 4 bytes of the payload.
type LoginCrypt struct {
	cipher *BlowfishCipher
}

// NewLoginCrypt creates a LoginCrypt from an initialized blowfish cipher.
func NewLoginCrypt(cipher *BlowfishCipher) *LoginCrypt {
	return &LoginCrypt{cipher: cipher}
}

// MobiusAuthKey returns blowfish cipher with the Mobius static login key.
func MobiusAuthKey() *BlowfishCipher {
	key := []byte{
		0x5B, 0x3B, 0x27, 0x2E,
		0x5D, 0x39, 0x34, 0x2D,
		0x33, 0x31, 0x3D, 0x3D,
		0x2D, 0x25, 0x26, 0x40,
		0x21, 0x5E, 0x2B, 0x5D,
		0x00,
	}

	cipher, err := NewBlowfishCipher(key)
	if err != nil {
		panic(err)
	}

	return cipher
}

// ChecksumLE computes the Mobius login checksum: XOR of little endian
// 4 byte words over all bytes except the trailing checksum word.
func ChecksumLE(data []byte) (uint32, error) {
	if len(data) < loginChecksumSize {
		return 0, errors.New("data is too small")
	}
	if len(data)%4 != 0 {
		return 0, errors.New("data is not multiple of 4")
	}

	checksum := uint32(0)
	for i := 0; i < len(data)-loginChecksumSize; i += 4 {
		checksum ^= binary.LittleEndian.Uint32(data[i : i+4])
	}

	return checksum, nil
}

// fillerSize returns the number of zero bytes between content and checksum
// so that content+filler+checksum is aligned to 8 bytes.
func fillerSize(contentSize int) int {
	return (loginAlignBy -
		(contentSize+loginChecksumSize)%loginAlignBy) % loginAlignBy
}

// Seal produces a full wire packet from plaintext content bytes. It appends
// zero filler and the little endian checksum, blowfish encrypts the payload
// and prepends the 2 byte little endian packet size.
func (lc *LoginCrypt) Seal(dst []byte, content []byte) ([]byte, error) {
	filler := fillerSize(len(content))
	size := len(content) + filler + loginChecksumSize + loginHeaderSize
	if size > loginMaxPacket {
		return nil, fmt.Errorf("packet size %d exceeds %d", size, loginMaxPacket)
	}

	out := dst[:0]
	if cap(out) < size {
		out = make([]byte, 0, size)
	}

	var header [loginHeaderSize]byte
	binary.LittleEndian.PutUint16(header[:], uint16(size)) //nolint:gosec
	out = append(out, header[:]...)

	payloadStart := len(out)
	out = append(out, content...)
	for range filler {
		out = append(out, 0)
	}
	// Checksum placeholder: the checksum covers all payload words except
	// the trailing one, so the four zero bytes must be part of the input.
	out = append(out, 0, 0, 0, 0)

	checksum, err := ChecksumLE(out[payloadStart:])
	if err != nil {
		return nil, fmt.Errorf("failed to compute login checksum: %w", err)
	}
	binary.LittleEndian.PutUint32(out[len(out)-loginChecksumSize:], checksum)

	if err := lc.cipher.EncryptInplace(out[payloadStart:]); err != nil {
		return nil, fmt.Errorf("failed to encrypt login packet: %w", err)
	}

	return out, nil
}

// Open decrypts a login payload, verifies its little endian checksum and
// returns the content without the trailing checksum bytes.
func (lc *LoginCrypt) Open(payload []byte) ([]byte, error) {
	if len(payload) < loginChecksumSize {
		return nil, errors.New("login payload is too small")
	}
	if len(payload)%loginAlignBy != 0 {
		return nil, fmt.Errorf("login payload len %d is not multiple of %d",
			len(payload), loginAlignBy)
	}

	if err := lc.cipher.DecryptInplace(payload); err != nil {
		return nil, fmt.Errorf("failed to decrypt login packet: %w", err)
	}

	expected, err := ChecksumLE(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to verify login checksum: %w", err)
	}
	actual := binary.LittleEndian.Uint32(payload[len(payload)-loginChecksumSize:])
	if expected != actual {
		return nil, fmt.Errorf("login packet checksum mismatch: got %08x, want %08x",
			expected, actual)
	}

	return payload[:len(payload)-loginChecksumSize], nil
}

// SealPacket serializes a packet and seals it into dst.
func (lc *LoginCrypt) SealPacket(dst []byte, p Serializable) ([]byte, error) {
	writer := packet.NewWriter()
	if err := p.ToBytes(writer); err != nil {
		return nil, fmt.Errorf("failed to serialize login packet: %w", err)
	}

	return lc.Seal(dst, writer.Bytes())
}
