// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package proxy

import (
	fromgameserver "github.com/melg8/swarm/internal/swarm/packets/from_game_server"
	"github.com/melg8/swarm/internal/swarm/packets/packet"
	"github.com/melg8/swarm/internal/swarm/state"
)

// charListOpcode marks the recorded char list packets of the bot.
const charListOpcode = 0x1F

// buildCharacterList serializes the one character CharSelectionInfo the
// emulated game server answers with: the character actually played by
// the attached bot session.
func (gc *gameConn) buildCharacterList() ([]byte, error) {
	snapshot := gc.session.tracker.Snapshot()
	info := gc.characterInfo(snapshot.Character, gc.session.id)

	list := &fromgameserver.CharSelectInfoPacket{
		Count:      1,
		Characters: []fromgameserver.CharacterInfo{info},
	}
	writer := packet.NewWriter()
	if err := list.ToBytes(writer); err != nil {
		return nil, err
	}

	return writer.Bytes(), nil
}

// characterInfo builds the char list entry of the played character. The
// appearance fields (sex, race, class, hair, face) come from the last
// recorded real char list of the session that contains the played
// character, so the client renders the same look the real server would;
// the vitals and the position come from the live tracker (they are
// fresher than anything recorded at login time).
func (gc *gameConn) characterInfo(
	character state.CharacterSnapshot, account string,
) fromgameserver.CharacterInfo {
	info := fromgameserver.CharacterInfo{
		Name:        character.Name,
		ObjectID:    character.ObjectID,
		Account:     account,
		SessionID:   0,
		ClanID:      0,
		Sex:         0,
		Race:        character.Race,
		BaseClassID: character.ClassID,
		X:           character.X,
		Y:           character.Y,
		Z:           character.Z,
		CurrentHP:   character.CurHP,
		CurrentMP:   character.CurMP,
		Level:       character.Level,
		HairStyle:   0,
		HairColor:   0,
		Face:        0,
		MaxHP:       character.MaxHP,
		MaxMP:       character.MaxMP,
		DeleteTimer: 0,
	}

	if recorded := gc.recordedCharacter(character.Name); recorded != nil {
		info.Sex = recorded.Sex
		info.Race = recorded.Race
		info.BaseClassID = recorded.BaseClassID
		info.HairStyle = recorded.HairStyle
		info.HairColor = recorded.HairColor
		info.Face = recorded.Face
		if info.Name == "" {
			info.Name = recorded.Name
		}
	}

	return info
}

// recordedCharacter returns the entry of the played character from the
// newest recorded real char list of the bot session, or nil.
func (gc *gameConn) recordedCharacter(
	name string,
) *fromgameserver.CharacterInfo {
	if gc.session == nil {
		return nil
	}

	var match *fromgameserver.CharacterInfo
	list := fromgameserver.NewCharSelectInfoPacket()
	gc.session.recorder.Walk(func(_ int64, payload []byte) bool {
		if len(payload) == 0 || payload[0] != charListOpcode {
			return true
		}
		err := fromgameserver.ParseCharSelectInfoPacket(list, payload)
		if err != nil {
			return true
		}
		for i := range list.Characters {
			character := list.Characters[i]
			if character.Name == name {
				copied := character
				match = &copied

				break
			}
		}

		return true
	})

	return match
}
