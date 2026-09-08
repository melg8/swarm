// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseChangeWaitTypePacket(t *testing.T) {
	build := func(moveType int32) []byte {
		data := []byte{changeWaitTypePacketID}
		data = putInt32(data, 268473919)
		data = putInt32(data, moveType)

		return data
	}

	t.Run("constructor defaults", func(t *testing.T) {
		p := NewChangeWaitTypePacket()
		require.Zero(t, p.ObjectID)
		require.False(t, p.Sitting)
	})

	t.Run("sitting transition", func(t *testing.T) {
		p := NewChangeWaitTypePacket()
		require.NoError(t, ParseChangeWaitTypePacket(p, build(waitTypeSitting)))
		require.Equal(t, int32(268473919), p.ObjectID)
		require.True(t, p.Sitting)
	})

	t.Run("standing transition", func(t *testing.T) {
		p := NewChangeWaitTypePacket()
		require.NoError(t, ParseChangeWaitTypePacket(p, build(waitTypeStanding)))
		require.False(t, p.Sitting)
	})

	t.Run("fake death transition counts as standing", func(t *testing.T) {
		p := NewChangeWaitTypePacket()
		require.NoError(t, ParseChangeWaitTypePacket(p, build(2)))
		require.False(t, p.Sitting)
	})

	t.Run("wrong packet id", func(t *testing.T) {
		p := NewChangeWaitTypePacket()
		require.Error(t, ParseChangeWaitTypePacket(p, []byte{0x40}))
	})

	t.Run("truncated object id", func(t *testing.T) {
		p := NewChangeWaitTypePacket()
		require.Error(t, ParseChangeWaitTypePacket(p, []byte{changeWaitTypePacketID, 1}))
	})

	t.Run("truncated move type", func(t *testing.T) {
		data := []byte{changeWaitTypePacketID}
		data = putInt32(data, 268473919)

		p := NewChangeWaitTypePacket()
		require.Error(t, ParseChangeWaitTypePacket(p, data))
	})
}

func TestCharCreateOkPacketValue(t *testing.T) {
	t.Run("constructor returns a usable packet", func(t *testing.T) {
		require.NotNil(t, NewCharCreateOkPacket())
	})

	t.Run("confirmation value", func(t *testing.T) {
		data := []byte{charCreateOkPacketID}
		data = putInt32(data, 1)

		require.NoError(t, ParseCharCreateOkPacket(data))
	})
}

func TestCharCreateFailReasonText(t *testing.T) {
	cases := []struct {
		reason int32
		text   string
	}{
		{0x00, "creation failed"},
		{0x01, "too many characters"},
		{0x02, "name already exists"},
		{0x03, "name exceeds 16 characters"},
		{0x04, "incorrect name"},
		{0x05, "creation not allowed on this server"},
		{0x06, "choose another server"},
		{0x63, "unknown reason 99"},
	}

	for _, tc := range cases {
		p := &CharCreateFailPacket{Reason: tc.reason}
		require.Equal(t, tc.text, p.ReasonText(), "reason 0x%02x", tc.reason)
	}
}

func TestParseKeyPacketTruncations(t *testing.T) {
	t.Run("missing result byte", func(t *testing.T) {
		p := NewKeyPacket()
		require.Error(t, ParseKeyPacket(p, []byte{keyPacketID}))
	})

	t.Run("missing tail", func(t *testing.T) {
		data := []byte{keyPacketID, 0x01}
		data = append(data, make([]byte, keyPacketKeySize)...)
		data = putInt32(data, 1)

		p := NewKeyPacket()
		require.Error(t, ParseKeyPacket(p, data))
	})
}
