// SPDX-FileCopyrightText: 2024 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package connection

import (
	"encoding/binary"
	"fmt"
	"io"
)

const wireHeaderSize = 2

const maxUint16 = 65535

// wireEndian is the endianness of the size header of every Mobius protocol
// packet, both login and game server connections read and write it little
// endian.
var wireEndian = binary.LittleEndian

// readWirePacket reads a single framed packet from the connection. The
// returned payload reuses the given buffer when its capacity is enough.
func readWirePacket(conn io.Reader, buf []byte) ([]byte, error) {
	var header [wireHeaderSize]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return nil, fmt.Errorf("failed to read packet header: %w", err)
	}

	size := int(wireEndian.Uint16(header[:]))
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

// writeWirePacket prepends the size header and writes the full packet.
func writeWirePacket(conn io.Writer, payload []byte) error {
	size := len(payload) + wireHeaderSize
	if size > maxUint16 {
		return fmt.Errorf("packet size %d exceeds %d", size, maxUint16)
	}

	var header [wireHeaderSize]byte
	wireEndian.PutUint16(header[:], uint16(size)) //nolint:gosec

	if _, err := conn.Write(header[:]); err != nil {
		return fmt.Errorf("failed to write packet header: %w", err)
	}
	if _, err := conn.Write(payload); err != nil {
		return fmt.Errorf("failed to write packet payload: %w", err)
	}

	return nil
}
