// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromauthserver

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"math"
	"strings"
	"testing"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

func OnlyPartialPacket(size int) []byte {
	result := make([]byte, size)

	return result
}

func OnlySessionIDPacket() []byte {
	packet := "ca0e617cffff"
	data, err := hex.DecodeString(packet)
	if err != nil {
		panic(err)
	}

	return data
}

func InitPacketData() []byte {
	packet := "ca0e617c21c6000040dbaa8d4c6f4b4ab33a538ff2977f24b1a08a2799b8696b2834efb8dfdf75807dfd14ef3051489fedf04712ba576139898c0a5de47c431ce407f5450092d747ff1e2e8294c0f00365f1b6a5d005767f9bda4ec694d43c7cc956ed4b2edc982d657c42a8793f3b1a6c4be631b97fd791a8a5adc9f9f4e8660dcba865b679b0124e95dd29fc9cc37720b6ad97f7e0bd07"
	data, err := hex.DecodeString(packet)
	if err != nil {
		panic(err)
	}

	return data
}

func ExpectedRsaPublicKey() []byte {
	RsaPublicKey := "40dbaa8d4c6f4b4ab33a538ff2977f24b1a08a2799b8696b2834efb8dfdf75807dfd14ef3051489fedf04712ba576139898c0a5de47c431ce407f5450092d747ff1e2e8294c0f00365f1b6a5d005767f9bda4ec694d43c7cc956ed4b2edc982d657c42a8793f3b1a6c4be631b97fd791a8a5adc9f9f4e8660dcba865b679b012"
	data, err := hex.DecodeString(RsaPublicKey)
	if err != nil {
		panic(err)
	}

	return data
}

func convertBytesToInt32BigEndian(bytes []byte) int32 {
	//nolint:gosec
	return int32(binary.BigEndian.Uint32(bytes))
}

func ExpectedGameGuard3() int32 {
	return convertBytesToInt32BigEndian([]byte{0x97, 0xad, 0xb6, 0x20})
}

func TestInitPacketEncodingAndDecoding(t *testing.T) { //nolint:cyclop
	packetBin := InitPacketData()
	initPacket := &InitPacket{}
	err := ParseInitPacket(initPacket, packetBin)
	if err != nil {
		t.Fatal(err)
	}
	if initPacket.SessionID != int32(0x7c610eca) {
		t.Error("wrong session id")
	}

	if initPacket.ProtocolVersion != int32(0x0000c621) {
		t.Error("wrong protocol version")
	}

	expectedRsaPublicKey := ExpectedRsaPublicKey()
	if !bytes.Equal(initPacket.RsaPublicKey, expectedRsaPublicKey) {
		t.Error("wrong rsa public key")
	}

	if initPacket.GameGuard1 != int32(0x29dd954e) {
		t.Error("wrong game guard part 1")
	}

	if initPacket.GameGuard2 != int32(0x77c39cfc) {
		t.Error("wrong game guard part 2")
	}

	if initPacket.GameGuard3 != ExpectedGameGuard3() {
		t.Error("wrong game guard part 3")
	}

	if initPacket.GameGuard4 != int32(0x07bde0f7) {
		t.Error("wrong game guard part 4")
	}

	if initPacket.BlowfishKey != nil {
		t.Error("blowfish key should be nil")
	}

	packetWriter := packet.NewWriter()
	err = initPacket.ToBytes(packetWriter)
	if err != nil {
		t.Fatal(err)
	}
	if packetWriter.Len() != len(packetBin) {
		t.Error("wrong packet writer length")
	}

	if !bytes.Equal(packetWriter.Bytes(), packetBin) {
		t.Error("wrong encoded packet")
	}
}

func TestInitPacketDecodingErrorOnEmptyPacket(t *testing.T) {
	emptyPacket := []byte{}
	initPacket := &InitPacket{}
	err := ParseInitPacket(initPacket, emptyPacket)
	if err == nil {
		t.Error("expected error on empty packet")
	}
}

func TestInitPacketDecodingErrorOnOnlyPartialPacket(t *testing.T) {
	partialPacket := OnlySessionIDPacket()
	initPacket := &InitPacket{}
	err := ParseInitPacket(initPacket, partialPacket)
	if err == nil {
		t.Error("expected error on partial packet")
	}
}

func TestInitPacketDecodingOkayWithOptionalBlowfishKeyPresent(t *testing.T) {
	packetBin := InitPacketData()
	initPacket := &InitPacket{}
	err := ParseInitPacket(initPacket, packetBin)
	if err != nil {
		t.Fatal(err)
	}
	blowFishKey := make([]byte, 21)
	initPacket.BlowfishKey = blowFishKey

	packetWriter := packet.NewWriter()
	err = initPacket.ToBytes(packetWriter)
	if err != nil {
		t.Fatal(err)
	}
	if packetWriter.Len() != len(packetBin)+21 {
		t.Error("incorrect length after blowfish key addition")
	}

	stringRepresentation := initPacket.ToString()
	if strings.Contains(stringRepresentation, "BlowfishKey: nil") {
		t.Error("expected BlowfishKey to be non-nil")
	}
}

func TestInitPacketToString(t *testing.T) {
	packetBin := InitPacketData()
	initPacket := &InitPacket{}
	err := ParseInitPacket(initPacket, packetBin)
	if err != nil {
		t.Fatal(err)
	}

	str := initPacket.ToString()

	// Check that key components are present in the string representation
	expectedParts := []string{
		"InitPacket:",
		"SessionID:",
		"ProtocolVersion:",
		"RsaPublicKey:",
		"GameGuard1:",
		"GameGuard2:",
		"GameGuard3:",
		"GameGuard4:",
	}

	for _, part := range expectedParts {
		if !strings.Contains(str, part) {
			t.Errorf("ToString() output missing expected part: %s", part)
		}
	}
}

func TestInitPacketEncodingErrors(t *testing.T) {
	// Create a packet with an invalid RSA key length
	invalidPacket := &InitPacket{
		SessionID:       0x7c610eca,
		ProtocolVersion: 0x0000c621,
		RsaPublicKey:    make([]byte, 64), // Invalid length, should be 128
		GameGuard1:      0x29dd954e,
		GameGuard2:      0x77c39cfc,
		GameGuard3:      ExpectedGameGuard3(),
		GameGuard4:      0x07bde0f7,
		BlowfishKey:     nil,
	}

	packetWriter := packet.NewWriter()
	err := invalidPacket.ToBytes(packetWriter)
	if err == nil {
		t.Error("Expected error for invalid RSA key length, got nil")
	}
}

func TestInitPacketWithBlowfishKeyEncoding(t *testing.T) {
	blowfishKey := []byte("test-blowfish-key-123")
	initPacket := &InitPacket{
		SessionID:       0x7c610eca,
		ProtocolVersion: 0x0000c621,
		RsaPublicKey:    ExpectedRsaPublicKey(),
		GameGuard1:      0x29dd954e,
		GameGuard2:      0x77c39cfc,
		GameGuard3:      ExpectedGameGuard3(),
		GameGuard4:      0x07bde0f7,
		BlowfishKey:     blowfishKey,
	}

	packetWriter := packet.NewWriter()
	err := initPacket.ToBytes(packetWriter)
	if err != nil {
		t.Fatal(err)
	}

	decoded := &InitPacket{}
	err = ParseInitPacket(decoded, packetWriter.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	if decoded.BlowfishKey == nil {
		t.Error("Expected BlowfishKey to be present in decoded packet")
	}

	if !bytes.Equal(decoded.BlowfishKey, blowfishKey) {
		t.Error("Decoded BlowfishKey does not match original")
	}
}

func TestNewInitPacketFromBytesFieldErrors(t *testing.T) {
	testCases := []struct {
		name     string
		size     int
		expected string
	}{
		{
			name:     "missing protocol version",
			size:     4, // Only session ID
			expected: "EOF",
		},
		{
			name:     "missing RSA key",
			size:     8, // Session ID + protocol version
			expected: "EOF",
		},
		{
			name:     "missing GameGuard1",
			size:     136, // Up to RSA key
			expected: "EOF",
		},
		{
			name:     "missing GameGuard2",
			size:     140, // Up to GameGuard1
			expected: "EOF",
		},
		{
			name:     "missing GameGuard3",
			size:     144, // Up to GameGuard2
			expected: "EOF",
		},
		{
			name:     "missing GameGuard4",
			size:     148, // Up to GameGuard3
			expected: "EOF",
		},
	}

	fullPacket := InitPacketData()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			partialData := fullPacket[:tc.size]
			initPacket := &InitPacket{}
			err := ParseInitPacket(initPacket, partialData)
			if err == nil || err.Error() != tc.expected {
				t.Errorf("Expected error %q, got %v", tc.expected, err)
			}
		})
	}
}

func TestBlowfishKeyEdgeCases(t *testing.T) {
	// Test with exactly 21 bytes
	exactKey := make([]byte, 21)
	for i := range exactKey {
		exactKey[i] = byte(i)
	}

	initPacket := &InitPacket{
		SessionID:       1,
		ProtocolVersion: 1,
		RsaPublicKey:    make([]byte, 128),
		GameGuard1:      1,
		GameGuard2:      2,
		GameGuard3:      3,
		GameGuard4:      4,
		BlowfishKey:     exactKey,
	}

	packetWriter := packet.NewWriter()
	err := initPacket.ToBytes(packetWriter)
	if err != nil {
		t.Fatal(err)
	}
	decoded := &InitPacket{}
	err = ParseInitPacket(decoded, packetWriter.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(decoded.BlowfishKey, exactKey) {
		t.Error("BlowfishKey not correctly encoded/decoded with exact size")
	}

	// Test with nil BlowfishKey
	packetWriter = packet.NewWriter()
	initPacket.BlowfishKey = nil
	err = initPacket.ToBytes(packetWriter)
	if err != nil {
		t.Fatal(err)
	}

	decoded = &InitPacket{}
	err = ParseInitPacket(decoded, packetWriter.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if decoded.BlowfishKey != nil {
		t.Error("Expected nil BlowfishKey in decoded packet")
	}
}

func TestNewInitPacketWriteErrors(t *testing.T) {
	// Test invalid RSA key sizes
	testCases := []struct {
		name        string
		rsaKeySize  int
		expectError bool
	}{
		{"empty key", 0, true},
		{"small key", 64, true},
		{"large key", 256, true},
		{"correct key", 128, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			initPacket := &InitPacket{
				SessionID:       1,
				ProtocolVersion: 1,
				RsaPublicKey:    make([]byte, tc.rsaKeySize),
				GameGuard1:      1,
				GameGuard2:      2,
				GameGuard3:      3,
				GameGuard4:      4,
				BlowfishKey:     nil,
			}

			packetWriter := packet.NewWriter()
			err := initPacket.ToBytes(packetWriter)
			if tc.expectError && err == nil {
				t.Errorf("Expected error for RSA key size %d, got nil", tc.rsaKeySize)
			} else if !tc.expectError && err != nil {
				t.Errorf("Unexpected error for RSA key size %d: %v", tc.rsaKeySize, err)
			}
		})
	}
}

func TestNewInitPacketWithZeroValues(t *testing.T) {
	initPacket := &InitPacket{
		SessionID:       0,
		ProtocolVersion: 0,
		RsaPublicKey:    make([]byte, 128),
		GameGuard1:      0,
		GameGuard2:      0,
		GameGuard3:      0,
		GameGuard4:      0,
		BlowfishKey:     nil,
	}

	packetWriter := packet.NewWriter()
	err := initPacket.ToBytes(packetWriter)
	if err != nil {
		t.Fatal(err)
	}
	decoded := &InitPacket{}
	err = ParseInitPacket(decoded, packetWriter.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	if decoded.SessionID != 0 || decoded.ProtocolVersion != 0 ||
		decoded.GameGuard1 != 0 || decoded.GameGuard2 != 0 ||
		decoded.GameGuard3 != 0 || decoded.GameGuard4 != 0 {
		t.Error("zero values were not preserved during encoding/decoding")
	}
}

func TestNewInitPacketWithMaxValues(t *testing.T) { //nolint:cyclop
	initPacket := &InitPacket{
		SessionID:       math.MaxInt32, // More readable than int32(^uint32(0) >> 1)
		ProtocolVersion: math.MaxInt32,
		RsaPublicKey:    make([]byte, 128),
		GameGuard1:      math.MaxInt32,
		GameGuard2:      math.MaxInt32,
		GameGuard3:      math.MaxInt32,
		GameGuard4:      math.MaxInt32,
		BlowfishKey:     nil,
	}

	// Fill RSA key with max values
	for i := range initPacket.RsaPublicKey {
		initPacket.RsaPublicKey[i] = 0xFF
	}

	packetWriter := packet.NewWriter()
	err := initPacket.ToBytes(packetWriter)
	if err != nil {
		t.Fatal(err)
	}
	decoded := &InitPacket{}
	err = ParseInitPacket(decoded, packetWriter.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	if decoded.SessionID != initPacket.SessionID ||
		decoded.ProtocolVersion != initPacket.ProtocolVersion ||
		decoded.GameGuard1 != initPacket.GameGuard1 ||
		decoded.GameGuard2 != initPacket.GameGuard2 ||
		decoded.GameGuard3 != initPacket.GameGuard3 ||
		decoded.GameGuard4 != initPacket.GameGuard4 ||
		!bytes.Equal(decoded.RsaPublicKey, initPacket.RsaPublicKey) {
		t.Error("max values were not preserved during encoding/decoding")
	}
}

func TestNewInitPacketWithNegativeValues(t *testing.T) {
	initPacket := &InitPacket{
		SessionID:       -1,
		ProtocolVersion: -1,
		RsaPublicKey:    make([]byte, 128),
		GameGuard1:      -1,
		GameGuard2:      -1,
		GameGuard3:      -1,
		GameGuard4:      -1,
		BlowfishKey:     nil,
	}

	packetWriter := packet.NewWriter()
	err := initPacket.ToBytes(packetWriter)
	if err != nil {
		t.Fatal(err)
	}

	decoded := &InitPacket{}
	err = ParseInitPacket(decoded, packetWriter.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	if decoded.SessionID != -1 || decoded.ProtocolVersion != -1 ||
		decoded.GameGuard1 != -1 || decoded.GameGuard2 != -1 ||
		decoded.GameGuard3 != -1 || decoded.GameGuard4 != -1 {
		t.Error("negative values were not preserved during encoding/decoding")
	}
}
