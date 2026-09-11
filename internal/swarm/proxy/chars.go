// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package proxy

import (
	"errors"

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
	return buildCharacterListFor(gc.currentSession())
}

// buildCharacterListFor serializes the one character CharSelectionInfo
// of the given bot session (the session is resolved under the connection
// lock by the caller - the char list is built from the relay and the
// timer goroutines while the read loop may swap the session).
func buildCharacterListFor(session *botSession) ([]byte, error) {
	if session == nil {
		return nil, errors.New("no bot session attached")
	}
	snapshot := session.tracker.Snapshot()
	info := characterInfoFor(session, snapshot.Character, session.id)

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

// characterInfoFor builds the char list entry of the played character
// of the given session. The appearance fields (sex, race, class, hair,
// face) come from the last recorded real char list of the session that
// contains the played character, so the client renders the same look
// the real server would; the vitals, the position and the paperdoll
// come from the live tracker (they are fresher than anything recorded
// at login time: the C1 client renders the selection screen model from
// the paperdoll item ids, so the zeroed table showed a naked character).
func characterInfoFor(
	session *botSession, character state.CharacterSnapshot, account string,
) fromgameserver.CharacterInfo {
	//nolint:exhaustruct_v5 // the paperdoll is filled below
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

	if recorded := recordedCharacter(session, character.Name); recorded != nil {
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
	fillLivePaperdoll(session, &info)

	return info
}

// fillLivePaperdoll resolves the equipped gear of the played character
// of the given session from its live tracker: the paperdoll object ids
// of the slots come from the last UserInfo broadcast the server sent
// (it refreshes the block on every equip and unequip), and the item ids
// are looked up in the tracked inventory. The two tables share the
// Mobius wire slot order, so the mapping is index to index (the right
// hand duplicate repeats in both).
func fillLivePaperdoll(
	session *botSession, info *fromgameserver.CharacterInfo,
) {
	if session == nil {
		return
	}

	paperdoll := session.tracker.PaperdollSlotObjectIDs()
	inventory := session.tracker.InventoryItems()
	itemIDs := make(map[int32]int32, len(inventory))
	for _, item := range inventory {
		itemIDs[item.ObjectID] = item.ItemID
	}
	slots := min(len(info.PaperdollObjectIDs), len(paperdoll))
	for i := range slots {
		objectID := paperdoll[i]
		info.PaperdollObjectIDs[i] = objectID
		info.PaperdollItemIDs[i] = itemIDs[objectID]
	}
}

// recordedCharacter returns the entry of the played character from the
// newest recorded real char list of the given bot session, or nil.
func recordedCharacter(
	session *botSession, name string,
) *fromgameserver.CharacterInfo {
	if session == nil {
		return nil
	}

	var match *fromgameserver.CharacterInfo
	list := fromgameserver.NewCharSelectInfoPacket()
	session.recorder.Walk(func(_ int64, payload []byte) bool {
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
