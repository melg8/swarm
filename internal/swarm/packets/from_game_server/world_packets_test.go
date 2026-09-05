// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package fromgameserver

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseNpcInfoPacket(t *testing.T) {
	t.Run("full packet", func(t *testing.T) {
		data := []byte{0x22}
		data = putInt32(data, 268473919)
		data = putInt32(data, 1001277)
		data = putInt32(data, 1) // attackable
		data = putInt32(data, 45000)
		data = putInt32(data, 50000)
		data = putInt32(data, -3500)
		data = putInt32(data, 16384)
		// speed lead: unknown int, mAtkSpd, pAtkSpd.
		data = append(data, make([]byte, npcInfoSpeedLead)...)
		data = putInt32(data, 165) // run speed
		data = putInt32(data, 55)  // walk speed
		data = append(data, make([]byte, npcInfoSpeedTrail)...)
		data = putFloat64(data, 1.15) // move multiplier
		data = append(data, make([]byte, npcInfoAtkSpeedTail)...)
		data = putFloat64(data, 10) // collision radius
		data = append(data, make([]byte, npcInfoBodyTail)...)
		data = append(data, 1, 1, 0, 1, 0) // flags: running, dead
		data = append(data, utf16("Keltir")...)
		data = append(data, utf16("")...)

		p := NewNpcInfoPacket()
		err := ParseNpcInfoPacket(p, data)
		require.NoError(t, err)
		require.Equal(t, int32(268473919), p.ObjectID)
		require.Equal(t, int32(1001277), p.TemplateID)
		require.True(t, p.Attackable)
		require.Equal(t, int32(45000), p.X)
		require.Equal(t, int32(50000), p.Y)
		require.Equal(t, int32(-3500), p.Z)
		require.Equal(t, int32(16384), p.Heading)
		require.Equal(t, int32(165), p.RunSpeed)
		require.Equal(t, int32(55), p.WalkSpeed)
		require.InDelta(t, 1.15, p.MoveSpeedMult, 0.0001)
		require.True(t, p.Running)
		require.False(t, p.InCombat)
		require.True(t, p.Dead)
		require.Equal(t, "Keltir", p.Name)
	})

	t.Run("wrong packet id", func(t *testing.T) {
		p := NewNpcInfoPacket()
		err := ParseNpcInfoPacket(p, []byte{0x23})
		require.Error(t, err)
	})

	t.Run("truncated", func(t *testing.T) {
		data := []byte{0x22}
		data = putInt32(data, 1)
		data = putInt32(data, 2)

		p := NewNpcInfoPacket()
		err := ParseNpcInfoPacket(p, data)
		require.Error(t, err)
	})
}

func TestParseAttackPacket(t *testing.T) {
	t.Run("full packet", func(t *testing.T) {
		data := []byte{0x06}
		data = putInt32(data, 268473919) // attacker
		data = putInt32(data, 268473100) // target
		data = putInt32(data, 42)        // damage
		data = append(data, 0x01)        // flags
		data = putInt32(data, 45000)
		data = putInt32(data, 50000)
		data = putInt32(data, -3500)
		data = putInt16(data, 1) // one more hit
		data = putInt32(data, 268473100)
		data = putInt32(data, 7)
		data = append(data, 0x00)
		data = putInt32(data, 45100)
		data = putInt32(data, 50100)
		data = putInt32(data, -3400)

		p := NewAttackPacket()
		err := ParseAttackPacket(p, data)
		require.NoError(t, err)
		require.Equal(t, int32(268473919), p.AttackerID)
		require.Equal(t, int32(45000), p.X)
		require.Equal(t, int32(50000), p.Y)
		require.Equal(t, int32(-3500), p.Z)
		require.Equal(t, 2, p.HitCount)
		require.Equal(t, int32(268473100), p.Hits[0].TargetID)
		require.Equal(t, int32(42), p.Hits[0].Damage)
		require.Equal(t, int32(7), p.Hits[1].Damage)
		require.Equal(t, int32(45100), p.TargetX)
		require.Equal(t, int32(50100), p.TargetY)
	})

	t.Run("wrong packet id", func(t *testing.T) {
		p := NewAttackPacket()
		err := ParseAttackPacket(p, []byte{0x07})
		require.Error(t, err)
	})

	t.Run("implausible hit count", func(t *testing.T) {
		data := []byte{0x06}
		data = putInt32(data, 1)
		data = putInt32(data, 2)
		data = putInt32(data, 3)
		data = append(data, 0x00)
		data = putInt32(data, 0)
		data = putInt32(data, 0)
		data = putInt32(data, 0)
		data = putInt16(data, -5)
		data = putInt32(data, 0)
		data = putInt32(data, 0)
		data = putInt32(data, 0)

		p := NewAttackPacket()
		err := ParseAttackPacket(p, data)
		require.Error(t, err)
	})
}

func TestParseAutoAttackPackets(t *testing.T) {
	t.Run("start", func(t *testing.T) {
		p := NewAutoAttackStartPacket()
		err := ParseAutoAttackStartPacket(
			p, append([]byte{0x3B}, putInt32(nil, 77)...))
		require.NoError(t, err)
		require.Equal(t, int32(77), p.ObjectID)
	})

	t.Run("stop", func(t *testing.T) {
		p := NewAutoAttackStopPacket()
		err := ParseAutoAttackStopPacket(
			p, append([]byte{0x3C}, putInt32(nil, 77)...))
		require.NoError(t, err)
		require.Equal(t, int32(77), p.ObjectID)
	})

	t.Run("wrong packet id", func(t *testing.T) {
		p := NewAutoAttackStopPacket()
		err := ParseAutoAttackStopPacket(p, []byte{0x3B, 0, 0, 0, 0})
		require.Error(t, err)
	})
}

func TestParseMoveToPawnPacket(t *testing.T) {
	t.Run("full packet", func(t *testing.T) {
		data := []byte{0x75}
		data = putInt32(data, 268473919) // attacker
		data = putInt32(data, 268473100) // target
		data = putInt32(data, 40)        // distance
		data = putInt32(data, 45000)
		data = putInt32(data, 50000)
		data = putInt32(data, -3500)
		data = putInt32(data, 45200)
		data = putInt32(data, 50200)
		data = putInt32(data, -3400)

		p := NewMoveToPawnPacket()
		err := ParseMoveToPawnPacket(p, data)
		require.NoError(t, err)
		require.Equal(t, int32(268473919), p.ObjectID)
		require.Equal(t, int32(268473100), p.TargetID)
		require.Equal(t, int32(40), p.Distance)
		require.Equal(t, int32(45000), p.X)
		require.Equal(t, int32(50200), p.TargetY)
	})

	t.Run("truncated", func(t *testing.T) {
		p := NewMoveToPawnPacket()
		err := ParseMoveToPawnPacket(p, []byte{0x75, 1, 2, 3})
		require.Error(t, err)
	})
}

func TestParseUserInfoPacket(t *testing.T) {
	t.Run("full packet", func(t *testing.T) {
		data := []byte{0x04}
		data = putInt32(data, 45000)
		data = putInt32(data, 50000)
		data = putInt32(data, -3500)
		data = putInt32(data, 0) // vehicle id
		data = putInt32(data, 268473919)
		data = append(data, utf16("test1")...)
		data = putInt32(data, 1)  // race
		data = putInt32(data, 0)  // female
		data = putInt32(data, 18) // base class
		data = putInt32(data, 5)  // level
		data = putInt32(data, 2000)
		data = putInt32(data, 36) // str
		data = putInt32(data, 35) // dex
		data = putInt32(data, 36) // con
		data = putInt32(data, 23) // int
		data = putInt32(data, 14) // wit
		data = putInt32(data, 25) // men
		data = putInt32(data, 122)
		data = putInt32(data, 100)
		data = putInt32(data, 40)
		data = putInt32(data, 39)
		data = putInt32(data, 10)  // sp
		data = putInt32(data, 640) // cur load
		data = putInt32(data, 6400000)
		// weapon flag, 15 paperdoll object ids, 15 paperdoll display
		// ids and 12 combat stat ints.
		data = append(data, make([]byte, userInfoStatsSkip)...)
		data = putInt32(data, 165) // run speed
		data = putInt32(data, 80)  // walk speed
		// swim and fly speeds.
		data = append(data, make([]byte, userInfoSpeedTrail)...)
		data = putFloat64(data, 1.1)

		p := NewUserInfoPacket()
		err := ParseUserInfoPacket(p, data)
		require.NoError(t, err)
		require.Equal(t, int32(268473919), p.ObjectID)
		require.Equal(t, "test1", p.Name)
		require.Equal(t, int32(1), p.Race)
		require.Equal(t, int32(18), p.ClassID)
		require.Equal(t, int32(5), p.Level)
		require.Equal(t, int32(45000), p.X)
		require.Equal(t, int32(50000), p.Y)
		require.Equal(t, int32(-3500), p.Z)
		require.Equal(t, int32(36), p.STR)
		require.Equal(t, int32(122), p.MaxHP)
		require.Equal(t, int32(100), p.CurHP)
		require.Equal(t, int32(40), p.MaxMP)
		require.Equal(t, int32(39), p.CurMP)
		require.Equal(t, int32(10), p.Sp)
		require.Equal(t, int32(640), p.CurrentLoad)
		require.Equal(t, int32(6400000), p.MaxLoad)
		require.Equal(t, int32(165), p.RunSpeed)
		require.Equal(t, int32(80), p.WalkSpeed)
		require.InDelta(t, 1.1, p.MoveSpeedMult, 0.0001)
	})

	t.Run("truncated", func(t *testing.T) {
		data := []byte{0x04}
		data = putInt32(data, 45000)

		p := NewUserInfoPacket()
		err := ParseUserInfoPacket(p, data)
		require.Error(t, err)
	})
}

func TestParseCharInfoPacket(t *testing.T) {
	t.Run("full packet", func(t *testing.T) {
		data := []byte{0x03}
		data = putInt32(data, 45000)
		data = putInt32(data, 50000)
		data = putInt32(data, -3500)
		data = putInt32(data, 0) // vehicle id
		data = putInt32(data, 268473919)
		data = append(data, utf16("Player2")...)
		data = putInt32(data, 0) // race
		data = putInt32(data, 0) // female
		data = putInt32(data, 10)
		// paperdoll ints, attack speed pair, pvp and karma.
		data = append(data, make([]byte, charInfoSpeedLead)...)
		data = putInt32(data, 150) // run speed
		data = putInt32(data, 75)  // walk speed
		// swim and fly speeds.
		data = append(data, make([]byte, charInfoSpeedTrail)...)
		data = putFloat64(data, 1.0)
		// attack speed multiplier.
		data = append(data, make([]byte, charInfoAtkSpeedTail)...)
		// collision radius, collision height and hair fields.
		data = putFloat64(data, 9)
		data = append(data, make([]byte, charInfoBodyTail)...)
		data = append(data, utf16("Title")...)
		// clan and ally ids plus the relation int.
		data = append(data, make([]byte, charInfoClanTail)...)
		// standing, running, in combat, alike dead, invisible, mount,
		// private store.
		data = append(data, 1, 1, 0, 0, 0, 0, 0)

		p := NewCharInfoPacket()
		err := ParseCharInfoPacket(p, data)
		require.NoError(t, err)
		require.Equal(t, int32(268473919), p.ObjectID)
		require.Equal(t, "Player2", p.Name)
		require.Equal(t, int32(45000), p.X)
		require.Equal(t, int32(50000), p.Y)
		require.Equal(t, int32(10), p.ClassID)
		require.Equal(t, "Title", p.Title)
		require.Equal(t, int32(150), p.RunSpeed)
		require.Equal(t, int32(75), p.WalkSpeed)
		require.InDelta(t, 1.0, p.MoveSpeedMult, 0.0001)
		require.True(t, p.Running)
		require.False(t, p.InCombat)
		require.False(t, p.Dead)
	})

	t.Run("truncated after title keeps flags default", func(t *testing.T) {
		data := []byte{0x03}
		data = putInt32(data, 45000)
		data = putInt32(data, 50000)
		data = putInt32(data, -3500)
		data = putInt32(data, 0)
		data = putInt32(data, 7)
		data = append(data, utf16("Player2")...)
		data = putInt32(data, 0)
		data = putInt32(data, 0)
		data = putInt32(data, 10)
		data = append(data, make([]byte, charInfoSpeedLead)...)
		data = putInt32(data, 150)
		data = putInt32(data, 75)
		data = append(data, make([]byte, charInfoSpeedTrail)...)
		data = putFloat64(data, 1.0)
		data = append(data, make([]byte, charInfoAtkSpeedTail)...)
		data = putFloat64(data, 9)
		data = append(data, make([]byte, charInfoBodyTail)...)
		data = append(data, utf16("Title")...)

		p := NewCharInfoPacket()
		err := ParseCharInfoPacket(p, data)
		require.NoError(t, err)
		require.False(t, p.Running)
	})
}

func TestParseMoveToLocationPacket(t *testing.T) {
	t.Run("full packet", func(t *testing.T) {
		data := []byte{0x01}
		data = putInt32(data, 7)
		data = putInt32(data, 200)
		data = putInt32(data, 300)
		data = putInt32(data, 400)
		data = putInt32(data, 100)
		data = putInt32(data, 110)
		data = putInt32(data, 120)

		p := NewMoveToLocationPacket()
		err := ParseMoveToLocationPacket(p, data)
		require.NoError(t, err)
		require.Equal(t, int32(7), p.ObjectID)
		// The destination comes before the current position.
		require.Equal(t, int32(200), p.DestX)
		require.Equal(t, int32(300), p.DestY)
		require.Equal(t, int32(100), p.X)
		require.Equal(t, int32(110), p.Y)
	})

	t.Run("truncated", func(t *testing.T) {
		p := NewMoveToLocationPacket()
		err := ParseMoveToLocationPacket(p, []byte{0x01, 0x00})
		require.Error(t, err)
	})
}

func TestParseStopMovePacket(t *testing.T) {
	data := []byte{0x59}
	data = putInt32(data, 7)
	data = putInt32(data, 200)
	data = putInt32(data, 300)
	data = putInt32(data, 400)
	data = putInt32(data, 8192)

	p := NewStopMovePacket()
	err := ParseStopMovePacket(p, data)
	require.NoError(t, err)
	require.Equal(t, int32(7), p.ObjectID)
	require.Equal(t, int32(200), p.X)
	require.Equal(t, int32(8192), p.Heading)
}

func TestParseValidateLocationPacket(t *testing.T) {
	data := []byte{0x76}
	data = putInt32(data, 268473919)
	data = putInt32(data, 45000)
	data = putInt32(data, 50000)
	data = putInt32(data, -3500)
	data = putInt32(data, 49152)

	p := NewValidateLocationPacket()
	err := ParseValidateLocationPacket(p, data)
	require.NoError(t, err)
	require.Equal(t, int32(268473919), p.ObjectID)
	require.Equal(t, int32(45000), p.X)
	require.Equal(t, int32(49152), p.Heading)
}

func TestParseDeleteObjectPacket(t *testing.T) {
	data := []byte{0x1E}
	data = putInt32(data, 7)

	p := NewDeleteObjectPacket()
	err := ParseDeleteObjectPacket(p, data)
	require.NoError(t, err)
	require.Equal(t, int32(7), p.ObjectID)
}

func TestParseDropItemPacket(t *testing.T) {
	data := []byte{0x16}
	data = putInt32(data, 100) // dropper
	data = putInt32(data, 200)
	data = putInt32(data, 5720)
	data = putInt32(data, 45000)
	data = putInt32(data, 50000)
	data = putInt32(data, -3500)
	data = putInt32(data, 1) // stackable
	data = putInt32(data, 25)
	data = putInt32(data, 0) // unknown

	p := NewDropItemPacket()
	err := ParseDropItemPacket(p, data)
	require.NoError(t, err)
	require.Equal(t, int32(200), p.ObjectID)
	require.Equal(t, int32(5720), p.TemplateID)
	require.True(t, p.Stackable)
	require.Equal(t, int32(25), p.Count)
	require.Equal(t, int32(45000), p.X)
}

func TestParseStatusUpdatePacket(t *testing.T) {
	t.Run("vitals", func(t *testing.T) {
		data := []byte{0x1A}
		data = putInt32(data, 268473919)
		data = putInt32(data, 4)
		data = putInt32(data, 0x09)
		data = putInt32(data, 70)
		data = putInt32(data, 0x0A)
		data = putInt32(data, 122)
		data = putInt32(data, 0x0B)
		data = putInt32(data, 25)
		data = putInt32(data, 0x0C)
		data = putInt32(data, 40)

		p := NewStatusUpdatePacket()
		err := ParseStatusUpdatePacket(p, data)
		require.NoError(t, err)
		require.Equal(t, int32(268473919), p.ObjectID)
		require.Equal(t, int32(4), p.Count)

		seen := map[int32]int32{}
		p.ForEach(func(id int32, value int32) {
			seen[id] = value
		})
		require.Equal(t, int32(70), seen[0x09])
		require.Equal(t, int32(122), seen[0x0A])
		require.Equal(t, int32(25), seen[0x0B])
		require.Equal(t, int32(40), seen[0x0C])
	})

	t.Run("too many attributes", func(t *testing.T) {
		data := []byte{0x1A}
		data = putInt32(data, 1)
		data = putInt32(data, 100)

		p := NewStatusUpdatePacket()
		err := ParseStatusUpdatePacket(p, data)
		require.Error(t, err)
	})

	t.Run("more pairs than stored", func(t *testing.T) {
		data := []byte{0x1A}
		data = putInt32(data, 1)
		data = putInt32(data, 12)
		for i := range 12 {
			data = putInt32(data, int32(i))    //nolint:gosec // loop
			data = putInt32(data, int32(i*10)) //nolint:gosec // loop
		}

		p := NewStatusUpdatePacket()
		err := ParseStatusUpdatePacket(p, data)
		require.NoError(t, err)
		require.Equal(t, int32(12), p.Count)
		require.Equal(t, int32(0), p.Attributes[0].ID)
		require.Equal(t, int32(7), p.Attributes[7].ID)
	})
}

func TestParseAttackPacketReuseResetsHits(t *testing.T) {
	build := func() []byte {
		data := []byte{0x06}
		data = putInt32(data, 268473919)
		data = putInt32(data, 268473100)
		data = putInt32(data, 42)
		data = append(data, 0x01)
		data = putInt32(data, 45000)
		data = putInt32(data, 50000)
		data = putInt32(data, -3500)
		data = putInt16(data, 0)
		data = putInt32(data, 45100)
		data = putInt32(data, 50100)
		data = putInt32(data, -3400)

		return data
	}

	p := NewAttackPacket()
	require.NoError(t, ParseAttackPacket(p, build()))
	require.Equal(t, 1, p.HitCount)
	// The same scratch packet parses the next attack: the hit count
	// must not accumulate.
	require.NoError(t, ParseAttackPacket(p, build()))
	require.Equal(t, 1, p.HitCount)
	require.NoError(t, ParseAttackPacket(p, build()))
	require.Equal(t, 1, p.HitCount)
}
