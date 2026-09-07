// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package togameserver

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/melg8/swarm/internal/swarm/packets/packet"
)

func TestRequestBuyItemToBytes(t *testing.T) {
	t.Run("two items", func(t *testing.T) {
		writer := packet.NewWriter()
		request := NewRequestBuyItemPacket()
		request.ListID = 3014700
		request.Items = []BuyItemEntry{
			{ItemID: 1, Count: 1},
			{ItemID: 41, Count: 2},
		}
		err := request.ToBytes(writer)
		require.NoError(t, err)
		require.Equal(t, []byte{
			0x1F,                   // opcode
			0x2C, 0x00, 0x2E, 0x00, // buylist id 3014700
			0x02, 0x00, 0x00, 0x00, // item count
			0x01, 0x00, 0x00, 0x00, // item id 1 (short sword)
			0x01, 0x00, 0x00, 0x00, // count
			0x29, 0x00, 0x00, 0x00, // item id 41 (cloth cap)
			0x02, 0x00, 0x00, 0x00, // count
		}, writer.Bytes())
	})
	t.Run("empty request", func(t *testing.T) {
		writer := packet.NewWriter()
		request := NewRequestBuyItemPacket()
		request.ListID = 3014800
		err := request.ToBytes(writer)
		require.NoError(t, err)
		require.Equal(t, []byte{
			0x1F,
			0x90, 0x00, 0x2E, 0x00,
			0x00, 0x00, 0x00, 0x00,
		}, writer.Bytes())
	})
}
