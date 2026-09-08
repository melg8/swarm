// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package proxy

import (
	"encoding/binary"
	"fmt"
	"io"
)

// wireHeaderSize is the size of the Mobius packet size header.
const wireHeaderSize = 2

// readWirePacket reads one framed packet payload from the connection
// into the reusable buffer. Mirrors the framing of the connection
// package: [size: 2 little endian, includes itself][payload].
func readWirePacket(conn io.Reader, buf []byte) ([]byte, error) {
	var header [wireHeaderSize]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return nil, fmt.Errorf("failed to read packet header: %w", err)
	}

	size := int(binary.LittleEndian.Uint16(header[:]))
	if size < wireHeaderSize {
		return nil, fmt.Errorf("invalid packet size %d", size)
	}

	payloadLen := size - wireHeaderSize
	if payloadLen == 0 {
		return buf[:0], nil
	}
	if cap(buf) < payloadLen {
		buf = make([]byte, payloadLen)
	}
	buf = buf[:payloadLen]
	if _, err := io.ReadFull(conn, buf); err != nil {
		return nil, fmt.Errorf("failed to read packet payload: %w", err)
	}

	return buf, nil
}

// writeWirePacket prepends the size header and writes the payload.
func writeWirePacket(conn io.Writer, payload []byte) error {
	size := len(payload) + wireHeaderSize
	if size > 65535 {
		return fmt.Errorf("packet size %d exceeds the uint16 wire limit", size)
	}

	var header [wireHeaderSize]byte
	binary.LittleEndian.PutUint16(header[:], uint16(size))
	if _, err := conn.Write(header[:]); err != nil {
		return fmt.Errorf("failed to write packet header: %w", err)
	}
	if _, err := conn.Write(payload); err != nil {
		return fmt.Errorf("failed to write packet payload: %w", err)
	}

	return nil
}
