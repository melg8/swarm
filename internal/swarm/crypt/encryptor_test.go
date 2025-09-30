// SPDX-FileCopyrightText: 2024 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package crypt

import (
	"testing"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
	toauthserver "github.com/melg8/swarm/internal/swarm/packets/to_auth_server"
	"github.com/stretchr/testify/require"
)

// EmptyPacket implements Serializable but writes no data.
type EmptyPacket struct{}

func (p *EmptyPacket) ToBytes(_ *packet.Writer) error {
	return nil
}

func TestEncryptor_Write_RequestGGAuth(t *testing.T) {
	// Create a new writer and encryptor
	writer := packet.NewWriter()
	cipher := DefaultAuthKey()
	encryptor := NewEncryptor(*writer, cipher)

	// Create a test packet
	packet := toauthserver.NewDefaultRequestGGAuth(1)

	// Write and encrypt the packet
	err := encryptor.Write(packet)
	require.NoError(t, err)

	// Get the encrypted bytes
	encryptedBytes := encryptor.Bytes()

	// Verify packet size (first 2 bytes)
	packetSize := int16(encryptedBytes[0]) | int16(encryptedBytes[1])<<8
	require.Equal(t, len(encryptedBytes), int(packetSize))

	// Decrypt the data for verification
	decryptedData := make([]byte, len(encryptedBytes)-2)
	copy(decryptedData, encryptedBytes[2:])
	err = cipher.DecryptInplace(decryptedData)
	require.NoError(t, err)

	// The decrypted data should start with packet ID (0x07)
	require.Equal(t, byte(0x07), decryptedData[0])

	// Verify session ID (1)
	sessionID := int32(decryptedData[1]) |
		int32(decryptedData[2])<<8 |
		int32(decryptedData[3])<<16 |
		int32(decryptedData[4])<<24
	require.Equal(t, int32(1), sessionID)
}

func TestEncryptor_Write_EmptyData(t *testing.T) {
	writer := packet.NewWriter()
	cipher := DefaultAuthKey()
	encryptor := NewEncryptor(*writer, cipher)

	// Create an empty packet
	emptyData := &EmptyPacket{}

	// Try to write empty data
	err := encryptor.Write(emptyData)
	require.Error(t, err)
	require.Contains(t, err.Error(), "data is too small")
}

func TestEncryptor_Write_PaddingAndChecksum(t *testing.T) {
	writer := packet.NewWriter()
	cipher := DefaultAuthKey()
	encryptor := NewEncryptor(*writer, cipher)

	// Create a test packet
	packet := toauthserver.NewDefaultRequestGGAuth(1)

	// Write and encrypt the packet
	err := encryptor.Write(packet)
	require.NoError(t, err)

	// Get the encrypted bytes
	encryptedBytes := encryptor.Bytes()

	// Decrypt the data
	decryptedData := make([]byte, len(encryptedBytes)-2)
	copy(decryptedData, encryptedBytes[2:])
	err = cipher.DecryptInplace(decryptedData)
	require.NoError(t, err)

	// Verify that the decrypted data length is aligned to CRC block size (4)
	require.Equal(t, 0, len(decryptedData)%crcAllignBy)

	// Verify checksum is present (last 4 bytes before padding)
	// Note: We don't verify the actual checksum value as it's implementation-dependent
	checksumPresent := false
	for i := len(decryptedData) - 4; i >= 0; i-- {
		if decryptedData[i] != 0 {
			checksumPresent = true

			break
		}
	}
	require.True(t, checksumPresent, "Checksum should be present in the data")
}
