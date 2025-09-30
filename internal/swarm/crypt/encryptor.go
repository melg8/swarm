// SPDX-FileCopyrightText: 2024 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package crypt

import (
	"errors"
	"math"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

type Serializable interface {
	ToBytes(w *packet.Writer) error
}

const (
	messagePrefixSize = 2
	crcAllignBy       = 4
	crcSize           = 4
	cipherAllignBy    = 8
)

type Encryptor struct {
	writer packet.Writer
	cipher *BlowfishCipher
}

func NewEncryptor(writer packet.Writer, cipher *BlowfishCipher) *Encryptor {
	return &Encryptor{
		writer: writer,
		cipher: cipher,
	}
}

func (e *Encryptor) writePadding(size int) error {
	for range size {
		if err := e.writer.WriteInt8(0); err != nil {
			return err
		}
	}

	return nil
}

func paddingSizeFor(size, align int) int {
	return (align - (size % align)) % align
}

func (e *Encryptor) writePaddingAndChecksum() error {
	currentMessageSize := e.writer.Len() - messagePrefixSize

	sizeWithCrc := currentMessageSize + crcSize
	paddingSize := paddingSizeFor(sizeWithCrc, crcAllignBy)

	if err := e.writePadding(paddingSize); err != nil {
		return err
	}

	crc, err := Checksum(e.writer.Bytes()[messagePrefixSize:])
	if err != nil {
		return err
	}
	crcBytes := make([]byte, 4)
	crcBytes[3] = byte(crc)
	crcBytes[2] = byte(crc >> 8)
	crcBytes[1] = byte(crc >> 16)
	crcBytes[0] = byte(crc >> 24)

	return e.writer.WriteBytes(crcBytes)
}

func (e *Encryptor) allignAndEncrypt() error {
	if e.writer.Len() < 2 {
		return errors.New("not enough data to encrypt")
	}

	paddingSize := paddingSizeFor(e.writer.Len()-2, cipherAllignBy)
	if err := e.writePadding(paddingSize); err != nil {
		return err
	}

	if err := e.cipher.EncryptInplace(e.writer.Bytes()[2:]); err != nil {
		return err
	}

	return nil
}

func (e *Encryptor) writePacketSize() error {
	packetSizeBytes := e.writer.Bytes()[0:2]
	size := e.writer.Len()
	if size > math.MaxUint16 {
		panic("packet size too big")
	}
	packetSize := int16(size) //nolint:gosec
	packetSizeBytes[0] = byte(packetSize)
	packetSizeBytes[1] = byte(packetSize >> 8)

	return nil
}

func (e *Encryptor) Write(data Serializable) error {
	// Reserve 2 bytes for future size value
	if err := e.writer.WriteInt16(0); err != nil {
		return err
	}

	if err := data.ToBytes(&e.writer); err != nil {
		return err
	}

	if err := e.writePaddingAndChecksum(); err != nil {
		return err
	}

	if err := e.allignAndEncrypt(); err != nil {
		return err
	}

	if err := e.writePacketSize(); err != nil {
		return err
	}

	return nil
}

func (e *Encryptor) Bytes() []byte {
	return e.writer.Bytes()
}
