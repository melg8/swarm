// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"testing"

	"github.com/melg8/swarm/internal/swarm/npcdata"
	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// The skill teacher data of the bot must match the Mobius C1
// SkillLearn.xml: the elven fighter (class 18) learns from Ellenia
// (display id 7155) and Cobendell (display id 7156), both standing on
// the teacher plaza of the elven village. The elven mystic (class 25)
// learns from Greenis (7157) and Esrandell (7158). The bot's nearest
// teacher selection picks the closest of the class teachers, which is
// correct: both teachers can teach the same class, the trip walks to
// whichever is nearer.

func TestElvenFighterTeachers(t *testing.T) {
	teachers := npcdata.TeachersOfClass(18)
	require.Len(t, teachers, 2,
		"the elven fighter has two teachers in the elven village")
	names := make(map[string]bool, len(teachers))
	for _, teacher := range teachers {
		names[teacher.Name] = true
	}
	require.True(t, names["Ellenia"],
		"Ellenia teaches the elven fighter")
	require.True(t, names["Cobendell"],
		"Cobendell teaches the elven fighter")
}

func TestElvenMysticTeachers(t *testing.T) {
	teachers := npcdata.TeachersOfClass(25)
	require.Len(t, teachers, 2,
		"the elven mystic has two teachers in the elven village")
	names := make(map[string]bool, len(teachers))
	for _, teacher := range teachers {
		names[teacher.Name] = true
	}
	require.True(t, names["Greenis"],
		"Greenis teaches the elven mystic")
	require.True(t, names["Esrandell"],
		"Esrandell teaches the elven mystic")
}

// TestNearestTeacherPicksClosest verifies the nearest teacher selection
// of the learning trip: the bot picks the teacher closest to the
// character. When the character stands near Cobendell (the western
// teacher of the plaza), the nearest teacher is Cobendell; when it
// stands near Ellenia (the eastern teacher), the nearest is Ellenia.
func TestNearestTeacherPicksClosest(t *testing.T) {
	bot := state.NewBot("acc1")
	bot.SetCharacter("test1", 100, 18, 44440, 52552, -2832, 13, 25023)
	loop := NewLoop(&fakeGame{}, bot)

	// The character at the dump stuck position (44440 52552) is
	// closer to Cobendell (44823 52414, dist ~407) than to Ellenia
	// (45725 52105, dist ~1361).
	teacher, ok := loop.nearestTeacher()
	require.True(t, ok)
	require.Equal(t, "Cobendell", teacher.Name,
		"the character near Cobendell must pick Cobendell")

	// Move the character closer to Ellenia.
	moveSelfTo(bot, 45600, 52100, -2792)
	teacher, ok = loop.nearestTeacher()
	require.True(t, ok)
	require.Equal(t, "Ellenia", teacher.Name,
		"the character near Ellenia must pick Ellenia")
}

// TestTownMerchantsAreNotTeachers verifies the town merchants (Herbiel,
// Creamees, Unoren, Ariel) are NOT listed as skill teachers: the
// learning trip must never walk to a merchant to learn a skill. The
// merchants sell items (including spellbooks), the teachers teach
// skills. The bot's data must keep these separate.
func TestTownMerchantsAreNotTeachers(t *testing.T) {
	merchantTemplateIDs := []int32{7147, 7148, 7149, 7150} // Unoren, Ariel, Creamees, Herbiel
	for _, classID := range []int32{18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30} {
		teachers := npcdata.TeachersOfClass(classID)
		for _, teacher := range teachers {
			for _, merchantID := range merchantTemplateIDs {
				require.NotEqual(t, merchantID, teacher.TemplateID,
					"merchant %d must not be a teacher of class %d",
					merchantID, classID)
			}
		}
	}
}
