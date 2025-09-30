// SPDX-FileCopyrightText: 2024 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package crypt

import (
	"errors"
	"fmt"

	//nolint:staticcheck // Required for compatibility with legacy game protocol
	"golang.org/x/crypto/blowfish"
)

type BlowfishCipher struct {
	cipher *blowfish.Cipher
}

func NewBlowfishCipher(key []byte) (*BlowfishCipher, error) {
	cipher, err := blowfish.NewCipher(key)
	if err != nil {
		return nil, err
	}

	return &BlowfishCipher{cipher: cipher}, nil
}

// 5F-3B-35-2E-5D-39-34-2D-33-31-3D-3D-2D-25-78-54-21-5E-5B-24-00.
func DefaultAuthKey() *BlowfishCipher {
	key := []byte{
		0x5F, 0x3B, 0x35, 0x2E,
		0x5D, 0x39, 0x34, 0x2D,
		0x33, 0x31, 0x3D, 0x3D,
		0x2D, 0x25, 0x78, 0x54,
		0x21, 0x5E, 0x5B, 0x24,
		0x00,
	}

	cipher, err := NewBlowfishCipher(key)
	if err != nil {
		panic(err)
	}

	return cipher
}

// flip4BytesEndianInplace swaps the endianness of 4-byte blocks in place.
// This is required because the game's blowfish implementation uses a different
// endianness than the golang.org/x/crypto/blowfish package. Open data should be
// flipped before encryption and encrypted data should be flipped again after
// decryption for results to match.
func flip4BytesEndianInplace(data []byte) {
	for i := 0; i < len(data); i += 4 {
		data[i], data[i+3] = data[i+3], data[i]
		data[i+1], data[i+2] = data[i+2], data[i+1]
	}
}

func (b *BlowfishCipher) Decrypt(dst, data []byte) error {
	lenData := len(data)
	if lenData == 0 {
		return errors.New("encrypted data is empty")
	}

	blockSize := b.cipher.BlockSize()
	if lenData%blockSize != 0 {
		return fmt.Errorf("encrypted data len must be multiple of %d, got %d",
			blockSize, lenData)
	}

	count := lenData / blockSize

	flip4BytesEndianInplace(data)
	for i := range count {
		start := i * blockSize
		end := start + blockSize
		b.cipher.Decrypt(dst[start:end], data[start:end])
	}
	flip4BytesEndianInplace(dst)

	if &dst[0] != &data[0] {
		flip4BytesEndianInplace(data)
	}

	return nil
}

func (b *BlowfishCipher) DecryptInplace(data []byte) error {
	return b.Decrypt(data, data)
}

func (b *BlowfishCipher) Encrypt(dst, data []byte) error {
	lenData := len(data)
	if lenData == 0 {
		return errors.New("data is empty")
	}

	blockSize := b.cipher.BlockSize()
	if lenData%blockSize != 0 {
		return fmt.Errorf("data length must be a multiple of %d, got %d", blockSize, lenData)
	}

	flip4BytesEndianInplace(data)
	blocksCount := lenData / blockSize
	for i := range blocksCount {
		start := i * blockSize
		end := start + blockSize
		b.cipher.Encrypt(dst[start:end], data[start:end])
	}
	flip4BytesEndianInplace(dst)

	if &dst[0] != &data[0] {
		flip4BytesEndianInplace(data)
	}

	return nil
}

func (b *BlowfishCipher) EncryptInplace(data []byte) error {
	return b.Encrypt(data, data)
}
