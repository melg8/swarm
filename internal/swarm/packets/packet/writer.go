// SPDX-FileCopyrightText: 2024 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package packet

import (
	"bytes"
	"unsafe"
)

type Writer struct {
	*bytes.Buffer
}

func NewWriter() *Writer {
	return &Writer{Buffer: bytes.NewBuffer([]byte{})}
}

func NewWriterTo(data []byte) *Writer {
	return &Writer{Buffer: bytes.NewBuffer(data)}
}

func (b *Writer) WriteInt64(value int64) error {
	buf := (*[8]byte)(unsafe.Pointer(&value))
	_, err := b.Write(buf[:])

	return err
}

func (b *Writer) WriteInt32(value int32) error {
	buf := (*[4]byte)(unsafe.Pointer(&value))
	_, err := b.Write(buf[:])

	return err
}

func (b *Writer) WriteInt16(value int16) error {
	buf := (*[2]byte)(unsafe.Pointer(&value))
	_, err := b.Write(buf[:])

	return err
}

func (b *Writer) WriteInt8(value int8) error {
	return b.WriteByte(byte(value))
}

func (b *Writer) WriteBytes(bytes []byte) error {
	_, err := b.Write(bytes)

	return err
}

func (b *Writer) WriteStringAsUtf16(value string) error {
	bytes := make([]byte, 0, len(value)*2+2)
	for _, r := range value {
		bytes = append(bytes, byte(r), byte(0))
	}

	bytes = append(bytes, 0, 0)

	return b.WriteBytes(bytes)
}
