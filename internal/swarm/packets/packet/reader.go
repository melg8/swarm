// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package packet

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"

	"golang.org/x/text/encoding/unicode"
)

type Reader struct {
	*bytes.Reader
}

func NewReader(buffer []byte) *Reader {
	return &Reader{bytes.NewReader(buffer)}
}

func (r *Reader) ReadBytes(number int) ([]byte, error) {
	buffer := make([]byte, number)
	n, err := r.Read(buffer)
	if err != nil {
		return nil, err
	}
	if n < number {
		return nil, errors.New("not enough bytes to read")
	}

	return buffer, nil
}

// Skip discards the next number bytes without allocating a buffer.
func (r *Reader) Skip(number int) error {
	for number > 0 {
		chunk := number
		var buf [64]byte
		if chunk > len(buf) {
			chunk = len(buf)
		}
		n, err := r.Read(buf[:chunk])
		if err != nil {
			return err
		}
		if n < chunk {
			return errors.New("not enough bytes to skip")
		}
		number -= chunk
	}

	return nil
}

func (r *Reader) ReadInt64() (int64, error) {
	var buf [8]byte
	n, err := r.Read(buf[:])
	if err != nil {
		return 0, err
	}
	if n != 8 {
		return 0, errors.New("not enough bytes to read int64")
	}
	result := int64(buf[7])<<56 |
		(int64(buf[6]) << 48) |
		(int64(buf[5]) << 40) |
		(int64(buf[4]) << 32) |
		(int64(buf[3]) << 24) |
		(int64(buf[2]) << 16) |
		(int64(buf[1]) << 8) |
		int64(buf[0])

	return result, nil
}

func (r *Reader) ReadInt32() (int32, error) {
	var buf [4]byte
	n, err := r.Read(buf[:])
	if err != nil {
		return 0, err
	}
	if n != 4 {
		return 0, errors.New("not enough bytes to read int32")
	}
	result := int32(buf[3])<<24 |
		(int32(buf[2]) << 16) |
		(int32(buf[1]) << 8) |
		int32(buf[0])

	return result, nil
}

func (r *Reader) ReadInt16() (int16, error) {
	var buf [2]byte
	n, err := r.Read(buf[:])
	if err != nil {
		return 0, err
	}
	if n != 2 {
		return 0, errors.New("not enough bytes to read int16")
	}

	return int16(buf[1])<<8 | int16(buf[0]), nil
}

func (r *Reader) ReadInt8() (int8, error) {
	var buf [1]byte
	n, err := r.Read(buf[:])
	if err != nil {
		return 0, err
	}
	if n != 1 {
		return 0, errors.New("not enough bytes to read int8")
	}

	return int8(buf[0]), nil
}

func (r *Reader) ReadFloat64() (float64, error) {
	var buf [8]byte
	n, err := r.Read(buf[:])
	if err != nil {
		return 0, err
	}
	if n != 8 {
		return 0, errors.New("not enough bytes to read float64")
	}

	return math.Float64frombits(binary.LittleEndian.Uint64(buf[:])), nil
}

func (r *Reader) ReadStringFromUtf16Format() (string, error) {
	// Typical names and titles fit into 16 utf16 units.
	data := make([]byte, 0, 32)

	for {
		firstByte, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		secondByte, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		if firstByte == 0 && secondByte == 0 {
			break
		}

		data = append(data, firstByte, secondByte)
	}
	decoder := unicode.UTF16(
		unicode.LittleEndian, unicode.UseBOM).NewDecoder()
	decodedString, err := decoder.String(string(data))
	if err != nil {
		return "", err
	}

	return decodedString, nil
}
