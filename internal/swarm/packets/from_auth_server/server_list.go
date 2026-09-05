// SPDX-FileCopyrightText: 2024 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromauthserver

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

const serverListPacketID = 0x04

// ServerListEntry describes a single game server in the server list.
type ServerListEntry struct {
	ServerID       int8
	IP             [4]byte
	Port           int32
	AgeLimit       int8
	Pvp            int8
	CurrentPlayers int16
	MaxPlayers     int16
	Status         int8
	Bits           int32
	Brackets       int8
}

// ServerListPacket carries the list of available game servers.
// Wire format: [opcode 0x04][count: 1][lastServer: 1] then per server
// [serverID: 1][ip: 4][port: 4][age: 1][pvp: 1][current: 2][max: 2]
// [status: 1][bits: 4][brackets: 1].
type ServerListPacket struct {
	Count      int8
	LastServer int8
	Servers    []ServerListEntry
}

// NewServerListPacket creates an empty server list packet.
func NewServerListPacket() *ServerListPacket {
	return &ServerListPacket{Count: 0, LastServer: 0, Servers: nil}
}

// ParseServerListPacket reads the packet from decrypted content bytes.
func ParseServerListPacket(p *ServerListPacket, data []byte) error {
	reader := packet.NewReader(data)

	id, err := reader.ReadInt8()
	if err != nil {
		return err
	}
	if id != serverListPacketID {
		return errors.New("invalid server list packet id")
	}

	if p.Count, err = reader.ReadInt8(); err != nil {
		return err
	}
	if p.LastServer, err = reader.ReadInt8(); err != nil {
		return err
	}

	count := int(p.Count)
	if count < 0 || count > 255 {
		return fmt.Errorf("invalid server count %d", count)
	}
	if cap(p.Servers) < count {
		p.Servers = make([]ServerListEntry, 0, count)
	}
	p.Servers = p.Servers[:0]

	for range count {
		entry, err := readServerListEntry(reader)
		if err != nil {
			return err
		}

		p.Servers = append(p.Servers, entry)
	}

	return nil
}

// readServerListEntry reads a single game server entry from the reader.
func readServerListEntry(reader *packet.Reader) (ServerListEntry, error) {
	var entry ServerListEntry

	serverID, err := reader.ReadInt8()
	if err != nil {
		return entry, err
	}
	entry.ServerID = serverID
	ip, err := reader.ReadBytes(4)
	if err != nil {
		return entry, err
	}
	copy(entry.IP[:], ip)

	// The remaining fields form one fixed size tail block.
	tail, err := reader.ReadBytes(serverListTailSize)
	if err != nil {
		return entry, err
	}
	decodeServerListTail(&entry, tail)

	return entry, nil
}

// serverListTailSize is the size of the fixed tail of a server list entry:
// port, age limit, pvp, current and max players, status, bits, brackets.
const serverListTailSize = 4 + 1 + 1 + 2 + 2 + 1 + 4 + 1

// decodeServerListTail decodes the fixed tail fields of a server entry.
func decodeServerListTail(entry *ServerListEntry, tail []byte) {
	//nolint:gosec // port is a network value
	entry.Port = int32(binary.LittleEndian.Uint32(tail[0:4]))
	entry.AgeLimit = int8(tail[4])
	entry.Pvp = int8(tail[5])
	//nolint:gosec // player counts are limited by the server capacity
	entry.CurrentPlayers = int16(binary.LittleEndian.Uint16(tail[6:8]))
	//nolint:gosec // player counts are limited by the server capacity
	entry.MaxPlayers = int16(binary.LittleEndian.Uint16(tail[8:10]))
	entry.Status = int8(tail[10])
	//nolint:gosec // bits is a network value
	entry.Bits = int32(binary.LittleEndian.Uint32(tail[11:15]))
	entry.Brackets = int8(tail[15])
}

// FindServerByID returns the server entry with the given id.
func (p *ServerListPacket) FindServerByID(serverID int) *ServerListEntry {
	for i := range p.Servers {
		if int(p.Servers[i].ServerID) == serverID {
			return &p.Servers[i]
		}
	}

	return nil
}

// FirstAvailableServer returns the first server entry that is not down.
func (p *ServerListPacket) FirstAvailableServer() *ServerListEntry {
	for i := range p.Servers {
		if p.Servers[i].Status != 0 {
			return &p.Servers[i]
		}
	}

	return nil
}
