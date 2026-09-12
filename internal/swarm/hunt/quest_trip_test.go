// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"context"
	"testing"

	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// The quest trip engine tests: a mini chain of the Q00406 shape
// (the Sorius accept, one kill stage, the exit talk) driven through
// the scripted dialog stub of the walker tests, with the server
// state moves the script pins: the accept bypass starts the quest
// (the journal cond 1), the 20th piece drop kills the mob AND sets
// the cond (the giveItems setCond of the script), the exit talk
// drops the quest from the journal.

// questChainGame wraps the scripted dialog stub with the quest
// server state moves of the mini chain.
type questChainGame struct {
	*scriptGame
	bot      *state.Bot
	attacks  int
	bypasses int
}

// ClickObject serves the entry pages of the two conversations: the
// script stub only serves the first interact click, the second
// conversation needs its own page reset.
func (q *questChainGame) ClickObject(objectID int32) error {
	if err := q.scriptGame.ClickObject(objectID); err != nil {
		return err
	}
	if q.script.clicks == 4 {
		q.script.cur = 4
	}

	return nil
}

func (q *questChainGame) SendBypass(command string) error {
	if err := q.scriptGame.SendBypass(command); err != nil {
		return err
	}
	q.bypasses++
	switch command {
	case "Script Q00406_PathOfTheElvenKnight 30327-06.htm":
		// The accept challenge link: startQuest, the journal
		// flips to cond 1 (the QuestList push).
		q.bot.ApplyQuestList(
			[]state.QuestEntryView{{QuestID: 406, State: 1}}, nil)
	case "Script":
		if q.bypasses == 4 {
			// The Quest entry link of the exit talk: the
			// 30327-09 page opens and the script exits the
			// quest (the journal drops it).
			q.bot.ApplyQuestList(nil, nil)
		}
	}

	return nil
}

func (q *questChainGame) AttackTarget(objectID int32) error {
	if err := q.scriptGame.AttackTarget(objectID); err != nil {
		return err
	}
	q.attacks++
	if q.attacks == 3 {
		// The 20th piece lands: the mob dies, the giveItems
		// setCond pushes the journal to cond 2 with the full
		// counters.
		q.bot.ApplyStatusUpdate(objectID, []state.Attribute{
			{ID: state.AttrCurHP, Value: 0},
		})
		q.bot.ApplyQuestList(
			[]state.QuestEntryView{{QuestID: 406, State: 2}},
			[]state.QuestItemView{{ItemID: 1205, Count: 20}},
		)
	}

	return nil
}

// testQuestChain is the mini chain of the engine test: the accept
// at Sorius, the topaz piece hunt of one mob species at the bot
// feet, the exit talk that drops the quest.
func testQuestChain() QuestChain {
	sorius := QuestNpc{
		TemplateID: 7327, Name: "Sorius",
		X: 45100, Y: 50050, Z: -3500,
	}

	return QuestChain{
		QuestID: 406, Script: "Q00406_PathOfTheElvenKnight",
		Title: "Path to an Elven Knight", MinLevel: 19, ClassID: 18,
		Start:     sorius,
		ProofItem: 1204,
		Accept: []DialogStep{
			{LinkText: "Say you want to be an Elven Knight"},
			{LinkText: "Challenge the test"},
		},
		Stages: []QuestStage{
			questKillStage(1, QuestKill{
				Mobs: []int32{35}, ItemIDs: []int32{1205},
				DropPercent: 70, Target: 20,
				GroundX: 45000, GroundY: 50000,
			}),
			questTalkStage(2, sorius),
		},
	}
}

// TestDriveQuestChainHappyPath pins the full trip of the mini chain:
// the accept conversation (the static page entry, the accept links,
// the journal flip), the kill stage (the ground walk, the engage
// until the counters fill, the journal cond advance of the 20th
// drop) and the exit talk that drops the quest from the journal.
func TestDriveQuestChainHappyPath(t *testing.T) {
	shortenDialogSeams(t)
	bot := newTestBot()
	// The stations of the chain live at the bot feet: the walks
	// close inside the arrival radius without movement.
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 7, TemplateID: 1007327, Attackable: false,
		X: 45100, Y: 50050, Z: -3500, Name: "Sorius",
	})
	bot.ApplyNpcInfo(state.NpcInfo{
		ObjectID: 9, TemplateID: 1000035, Attackable: true,
		X: 45100, Y: 50050, Z: -3500, Name: "Tracker Skeleton",
	})
	script := &dialogScript{
		cur: -1,
		pages: []scriptPage{
			{npc: 7, html: soriusTrainerPage},
			{npc: 7, html: soriusQuestChoosePage},
			{npc: 7, html: soriusChallengePage},
			{npc: 7, html: soriusAcceptPage},
			{npc: 7, html: soriusTrainerPage},
			{npc: 7, html: soriusExitPage},
		},
	}
	game := &questChainGame{
		scriptGame: &scriptGame{fakeGame: &fakeGame{}, script: script},
		bot:        bot,
	}
	loop := NewLoop(game, bot)

	err := loop.DriveQuestChain(context.Background(), testQuestChain())
	require.NoError(t, err)
	// The accept conversation ran its two-link route after the
	// static page entry, the kill stage engaged the mob until the
	// counters filled, the exit talk closed the chain.
	require.GreaterOrEqual(t, game.attacks, 3,
		"the kill stage must engage the quest mob")
	require.Len(t, game.clicks, 4,
		"the two conversations are the two-click entries")
	_, ok := bot.QuestCond(406)
	require.False(t, ok, "the journal must have dropped the quest")
}

// TestDriveQuestChainCondOutsideLadder pins the ladder guard: a
// journal cond no stage covers returns the error naming it instead
// of looping the engine.
func TestDriveQuestChainCondOutsideLadder(t *testing.T) {
	shortenDialogSeams(t)
	bot := newTestBot()
	bot.ApplyQuestList(
		[]state.QuestEntryView{{QuestID: 406, State: 9}}, nil)
	script := &dialogScript{cur: -1}
	game := &questChainGame{
		scriptGame: &scriptGame{fakeGame: &fakeGame{}, script: script},
		bot:        bot,
	}
	loop := NewLoop(game, bot)

	err := loop.DriveQuestChain(context.Background(), testQuestChain())
	require.ErrorContains(t, err, "cond 9")
}

// soriusTrainerPage is the STATIC trainer page every Sorius talk
// opens with: the bare Quest link whose bypass runs the
// showQuestWindow of the single quest station.
const soriusTrainerPage = "<html><body>Master Sorius:<br>\n" +
	"Greetings, child in search of the training of the sword.<br>\n" +
	"<a action=\"bypass Script\">Quest</a>\n" +
	"</body></html>"

// soriusQuestChoosePage is the CREATED quest page of the station:
// the accept link of the route.
const soriusQuestChoosePage = "<html><body>Master Sorius:<br>\n" +
	"Elven Knights choose the path of the sword over archery.<br>\n" +
	"<a action=\"bypass Script Q00406_PathOfTheElvenKnight " +
	"30327-05.htm\">Say you want to be an Elven Knight</a>\n" +
	"</body></html>"

// soriusExitPage is the 30327-09 shape the exit talk answers with:
// a page without links (the talk itself ends the quest).
const soriusExitPage = "<html><body>Master Sorius:<br>\n" +
	"You have done well. Take this token of your worth.<br>\n" +
	"</body></html>"
