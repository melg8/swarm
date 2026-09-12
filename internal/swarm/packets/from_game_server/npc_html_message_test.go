// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestParseNpcHTMLMessage pins the wire format of the server html
// dialog: the 0x1B opcode, the npc object id, the null-terminated
// UTF-16LE html body and the trailing item id. The object id uses
// 0x01020304 so the little-endian byte order is unambiguous.
func TestParseNpcHTMLMessage(t *testing.T) {
	html := []byte{'H', 0, 'i', 0, 0, 0}
	data := []byte{
		0x1B,
		0x04, 0x03, 0x02, 0x01, // npc obj id 0x01020304 little endian
	}
	data = append(data, html...)
	data = append(data, 0x00, 0x00, 0x00, 0x00) // item id 0

	msg := NewNpcHTMLMessage()
	err := ParseNpcHTMLMessage(msg, data)
	require.NoError(t, err)
	require.Equal(t, int32(0x01020304), msg.NpcObjID)
	require.Equal(t, "Hi", msg.HTML)
	require.Equal(t, int32(0), msg.ItemID)
}

// TestParseNpcHTMLMessageItemID pins the item html variant: a non-zero
// item id rides the tail (the HtmlActionScope NpcItemHtml).
func TestParseNpcHTMLMessageItemID(t *testing.T) {
	html := []byte{'x', 0, 0, 0}
	data := []byte{
		0x1B,
		0x01, 0x00, 0x00, 0x00, // npc obj id 1
	}
	data = append(data, html...)
	data = append(data, 0xE9, 0x03, 0x00, 0x00) // item id 1001

	msg := NewNpcHTMLMessage()
	err := ParseNpcHTMLMessage(msg, data)
	require.NoError(t, err)
	require.Equal(t, int32(1), msg.NpcObjID)
	require.Equal(t, "x", msg.HTML)
	require.Equal(t, int32(1001), msg.ItemID)
}

// TestParseNpcHTMLMessageWrongOpcode pins the opcode guard.
func TestParseNpcHTMLMessageWrongOpcode(t *testing.T) {
	data := []byte{0xFF, 0, 0, 0, 0, 0, 0}
	msg := NewNpcHTMLMessage()
	err := ParseNpcHTMLMessage(msg, data)
	require.Error(t, err)
}
