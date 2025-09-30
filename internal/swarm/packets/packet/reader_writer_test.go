// SPDX-FileCopyrightText: 2024 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package packet

import (
	"bytes"
	"fmt"
	"testing"
)

const testStringValue = "some string"

func writeValuesToWriter(writer *Writer) ([]byte, error) {
	int64Value := int64(1234567890123)
	err := writer.WriteInt64(int64Value)
	if err != nil {
		return nil, err
	}

	int32Value := int32(123456789)
	err = writer.WriteInt32(int32Value)
	if err != nil {
		return nil, err
	}

	int16Value := int16(12345)
	err = writer.WriteInt16(int16Value)
	if err != nil {
		return nil, err
	}

	int8Value := int8(123)
	err = writer.WriteInt8(int8Value)
	if err != nil {
		return nil, err
	}

	err = writer.WriteStringAsUtf16(testStringValue)
	if err != nil {
		return nil, err
	}

	keyData := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	err = writer.WriteBytes(keyData)
	if err != nil {
		return nil, err
	}

	return writer.Bytes(), nil
}

func readValuesFromReader(reader *Reader) error { //nolint:cyclop
	int64Value, err := reader.ReadInt64()
	if err != nil {
		return err
	}
	if int64Value != 1234567890123 {
		return fmt.Errorf("Got different int64 value: %d != 1234567890123", int64Value)
	}

	int32Value, err := reader.ReadInt32()
	if err != nil {
		return err
	}
	if int32Value != 123456789 {
		return fmt.Errorf("Got different int32 value: %d != 123456789", int32Value)
	}

	int16Value, err := reader.ReadInt16()
	if err != nil {
		return err
	}
	if int16Value != 12345 {
		return fmt.Errorf("Got different int16 value: %d != 12345", int16Value)
	}

	int8Value, err := reader.ReadInt8()
	if err != nil {
		return err
	}
	if int8Value != 123 {
		return fmt.Errorf("Got different int8 value: %d != 123", int8Value)
	}

	stringValue, err := reader.ReadStringFromUtf16Format()
	if err != nil {
		return err
	}
	if stringValue != testStringValue {
		return fmt.Errorf("Got different string value: %s != %s", stringValue, testStringValue)
	}

	keyData, err := reader.ReadBytes(16)
	if err != nil {
		return err
	}
	if !bytes.Equal(keyData, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}) {
		return fmt.Errorf("Got different key data: %s != %s", keyData, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15})
	}

	return nil
}

func TestPacketWriterAndReader(t *testing.T) {
	writer := NewWriter()
	data, err := writeValuesToWriter(writer)
	if err != nil {
		panic(err)
	}

	reader := NewReader(data)
	if err := readValuesFromReader(reader); err != nil {
		t.Error(err)
	}
}

func TestUtf16StringToHexAndASCII(t *testing.T) {
	writer := NewWriter()

	stringValue := "some string"
	err := writer.WriteStringAsUtf16(stringValue)
	if err != nil {
		panic(err)
	}

	data := writer.Bytes()

	reader := NewReader(data)

	gotStringValue, err := reader.ReadStringFromUtf16Format()
	if err != nil {
		panic(err)
	}
	if gotStringValue != stringValue {
		t.Errorf("Got different string value: %s != %s", gotStringValue, stringValue)
	}
}

func TestPacketReaderReadInt64Error(t *testing.T) {
	reader := NewReader([]byte{})
	_, err := reader.ReadInt64()
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
}

func TestPacketReaderReadInt64Error1(t *testing.T) {
	reader := NewReader([]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07})
	_, err := reader.ReadInt64()
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
}

func TestPacketReaderReadInt32Error(t *testing.T) {
	reader := NewReader([]byte{})
	_, err := reader.ReadInt32()
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
}

func TestPacketReaderReadInt32Error1(t *testing.T) {
	reader := NewReader([]byte{0x01, 0x02, 0x03})
	_, err := reader.ReadInt32()
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
}

func TestPacketReaderReadInt16Error(t *testing.T) {
	reader := NewReader([]byte{})
	_, err := reader.ReadInt16()
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
}

func TestPacketReaderReadInt8Error(t *testing.T) {
	reader := NewReader([]byte{})
	_, err := reader.ReadInt8()
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
}

func TestPacketReaderReadBytesError(t *testing.T) {
	reader := NewReader([]byte{})
	_, err := reader.ReadBytes(1)
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
}

func TestPacketReaderReadBytesNotEnoughBytesError(t *testing.T) {
	reader := NewReader([]byte{1, 2, 3})
	_, err := reader.ReadBytes(4)
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
}

func TestPacketReaderReadStringFromUtf16FormatError(t *testing.T) {
	reader := NewReader([]byte{})
	_, err := reader.ReadStringFromUtf16Format()
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
}

func TestPacketReaderReadStringFromUtf16FormatTooSmallError(t *testing.T) {
	reader := NewReader([]byte{0x22, 0x00, 0x33})
	_, err := reader.ReadStringFromUtf16Format()
	if err == nil {
		t.Errorf("Expected error, got nil")
	}
}
