// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package crypt

import (
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

type Deserializable interface {
	FromBytes(r *packet.Reader) error
}

type Decryptor struct {
	reader *packet.Reader
	cipher *BlowfishCipher
}

func NewDecryptor(reader *packet.Reader, cipher *BlowfishCipher) *Decryptor {
	return &Decryptor{reader: reader, cipher: cipher}
}

func (d *Decryptor) Read(destination Deserializable) error {
	size, err := d.reader.ReadInt16()
	if err != nil {
		return fmt.Errorf("failed to read packet size: %w", err)
	}

	if size < messagePrefixSize {
		return fmt.Errorf("invalid packet size: %d", size)
	}

	encryptedData := make([]byte, size-messagePrefixSize)
	if _, err := d.reader.Read(encryptedData); err != nil {
		return fmt.Errorf("failed to read packet data: %w", err)
	}

	if err := d.cipher.DecryptInplace(encryptedData); err != nil {
		return fmt.Errorf("failed to decrypt packet data: %w", err)
	}
	unencryptedReader := packet.NewReader(encryptedData)
	return destination.FromBytes(unencryptedReader)
}
