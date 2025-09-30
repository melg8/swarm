// SPDX-FileCopyrightText: 2024 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package crypt

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

func TestBlowfishEmptyData(t *testing.T) {
	data := []byte{}
	err := DefaultAuthKey().DecryptInplace(data)
	if err == nil {
		t.Fatal("Decryption should have failed on empty data")
	}
	if err.Error() != "encrypted data is empty" {
		t.Fatal("Error message should be 'encrypted data is empty', got: ",
			err.Error())
	}
}

func TestBlowfishDataNotMultipleOf8(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7}
	err := DefaultAuthKey().DecryptInplace(data)
	if err == nil {
		t.Fatal("Decryption should have failed on data not multiple of 8")
	}
	expectedPrefix := "encrypted data len must be multiple of 8"
	if !strings.HasPrefix(err.Error(), expectedPrefix) {
		t.Fatalf("Error message should start with '%s', got: %s", expectedPrefix, err.Error())
	}
}

func TestBlowfishEmptyKey(t *testing.T) {
	cipher, err := NewBlowfishCipher([]byte{})
	if err == nil {
		t.Fatal("Creating cipher should have failed with empty key")
	}
	if cipher != nil {
		t.Fatal("Cipher should be nil when creation fails")
	}
}

func TestBlowfishNilKey(t *testing.T) {
	cipher, err := NewBlowfishCipher(nil)
	if err == nil {
		t.Fatal("Creating cipher should have failed with nil key")
	}
	if cipher != nil {
		t.Fatal("Cipher should be nil when creation fails")
	}
}

func TestBlowfish(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	err := DefaultAuthKey().DecryptInplace(data)
	if err != nil {
		t.Fatal("Decryption failed: ", err)
	}
	if len(data) != 8 {
		t.Fatal("Data should be 8 bytes, got: ", len(data))
	}
}

func TestBlowfishEncryptEmptyData(t *testing.T) {
	data := []byte{}
	err := DefaultAuthKey().EncryptInplace(data)
	if err == nil {
		t.Fatal("Encryption should have failed on empty data")
	}
	if err.Error() != "data is empty" {
		t.Fatal("Error message should be 'data is empty', got: ",
			err.Error())
	}
}

func TestBlowfishEncryptDataNotMultipleOf8(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7}
	err := DefaultAuthKey().EncryptInplace(data)
	if err == nil {
		t.Fatal("Encryption should have failed on data not multiple of 8")
	}
	expectedPrefix := "data length must be a multiple of 8"
	if !strings.HasPrefix(err.Error(), expectedPrefix) {
		t.Fatalf("Error message should start with '%s', got: %s", expectedPrefix, err.Error())
	}
}

func TestBlowfishEncryptEmptyKey(t *testing.T) {
	cipher, err := NewBlowfishCipher([]byte{})
	if err == nil {
		t.Fatal("Creating cipher should have failed with empty key")
	}
	if cipher != nil {
		t.Fatal("Cipher should be nil when creation fails")
	}
}

func TestBlowfishEncryptNilKey(t *testing.T) {
	cipher, err := NewBlowfishCipher(nil)
	if err == nil {
		t.Fatal("Creating cipher should have failed with nil key")
	}
	if cipher != nil {
		t.Fatal("Cipher should be nil when creation fails")
	}
}

func TestBlowfishEncryptDecryptCycle(t *testing.T) {
	originalData := []byte{1, 2, 3, 4, 5, 6, 7, 8, 1, 2, 3, 4, 5, 6, 7, 8}
	data := make([]byte, len(originalData))
	copy(data, originalData)
	key := DefaultAuthKey()

	// Test encryption
	err := key.EncryptInplace(data)
	if err != nil {
		t.Fatal("Encryption failed: ", err)
	}
	if len(data) != 16 {
		t.Fatal("Data should be 16 bytes, got: ", len(data))
	}

	// Verify data was actually changed by encryption
	different := false
	for i := range originalData {
		if originalData[i] != data[i] {
			different = true

			break
		}
	}
	if !different {
		t.Fatal("Encrypted data is identical to original data")
	}

	// Test decryption of the encrypted data
	err = key.DecryptInplace(data)
	if err != nil {
		t.Fatal("Decryption failed: ", err)
	}
	if len(data) != 16 {
		t.Fatal("Data should be 16 bytes, got: ", len(data))
	}

	// Verify the decrypted data matches the original
	for i := range originalData {
		if originalData[i] != data[i] {
			t.Fatalf("Decrypted data differs from original at position %d: expected %d, got %d",
				i, originalData[i], data[i])
		}
	}
}

// 00:08:18 :[Send request auth gg data: 00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00
// 00:08:18 :[key for encrypting auth gg data: 5F-3B-35-2E-5D-39-34-2D-33-31-3D-3D-2D-25-78-54-21-5E-5B-24-00
// 00:08:18 :[Send encrypted auth gg data: 2A-00-41-61-6F-5C-4B-BD-6F-A5-41-61-6F-5C-4B-BD-6F-A5-41-61-6F-5C-4B-BD-6F-A5-41-61-6F-5C-4B-BD-6F-A5

func TestBlowfishEncryption(t *testing.T) {
	data := make([]byte, 40)
	expected, _ := hex.DecodeString("41616F5C4BBD6FA541616F5C4BBD6FA541616F5C4BBD6FA541616F5C4BBD6FA541616F5C4BBD6FA5")

	cipher := DefaultAuthKey()

	err := cipher.EncryptInplace(data)
	if err != nil {
		t.Fatalf("Encryption failed: %v", err)
	}

	if !bytes.Equal(data, expected) {
		t.Errorf("Encrypted data does not match expected.\nGot:      %x\nExpected: %x", data, expected)
	}
}

// For test data see: https://www.schneier.com/wp-content/uploads/2015/12/vectors-2.txt
// But implementation of java/c# blowfish versions have different endianness.
// So each 4 bytes are reversed compared to the original data.
// Same issue was observed here:
// https://stackoverflow.com/questions/44274221/blowfish-results-are-different-between-openssl-and-golang.
// Other reference implementations can be found here:
// https://www.schneier.com/academic/blowfish/download/

// 01:05:42 :[Send encrypted auth gg data: 2A-00-45-97-F9-4E-78-DD-98-61-45-97-F9-4E-78-DD-98-61-45-97-F9-4E-78-DD-98-61-45-97-F9-4E-78-DD-98-61-45-97-F9-4E-78-DD-98-61
// 01:05:42 :[key for encrypting auth gg data: 00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00
// 01:05:42 :[Send request auth gg data: 00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00-00
func TestBlowfishEncryptionWithZeroKey(t *testing.T) {
	data := make([]byte, 40)
	key := make([]byte, 21)
	expected, _ := hex.DecodeString("4597F94E78DD98614597F94E78DD98614597F94E78DD98614597F94E78DD98614597F94E78DD9861")

	cipher, err := NewBlowfishCipher(key)
	if err != nil {
		t.Fatalf("Failed to create cipher: %v", err)
	}

	err = cipher.EncryptInplace(data)
	if err != nil {
		t.Fatalf("Encryption failed: %v", err)
	}

	if !bytes.Equal(data, expected) {
		t.Errorf("Encrypted data does not match expected.\nGot:      %x\nExpected: %x", data, expected)
	}
}

// 01:47:01 :[Send encrypted auth gg data: 2A-00-11-73-83-1B-FF-E8-87-DC-EE-76-D9-58-CA-E9-CA-64-56-A5-4D-5F-59-EA-DF-4F-C1-17-A5-E9-2C-9F-34-CD-41-61-6F-5C-4B-BD-6F-A5
// 01:47:01 :[key for encrypting auth gg data: 5F-3B-35-2E-5D-39-34-2D-33-31-3D-3D-2D-25-78-54-21-5E-5B-24-00
// 01:47:01 :[Send request auth gg data: 07-37-44-52-D9-23-01-00-00-67-45-00-00-AB-89-00-00-EF-CD-00-00-00-00-00-DE-37-44-52-00-00-00-00-00-00-00-00-00-00-00-00

func TestBlowfishEncryptionWithCustomKey(t *testing.T) {
	data, _ := hex.DecodeString("07374452D92301000067450000AB890000EFCD0000000000DE374452000000000000000000000000")
	key, _ := hex.DecodeString("5F3B352E5D39342D33313D3D2D257854215E5B2400")
	expected, _ := hex.DecodeString("1173831BFFE887DCEE76D958CAE9CA6456A54D5F59EADF4FC117A5E92C9F34CD41616F5C4BBD6FA5")

	cipher, err := NewBlowfishCipher(key)
	if err != nil {
		t.Fatalf("Failed to create cipher: %v", err)
	}

	err = cipher.EncryptInplace(data)
	if err != nil {
		t.Fatalf("Encryption failed: %v", err)
	}

	if !bytes.Equal(data, expected) {
		t.Errorf("Encrypted data does not match expected.\nGot:      %x\nExpected: %x", data, expected)
	}
}
