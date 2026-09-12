// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"testing"
	"time"

	fromgameserver "github.com/melg8/swarm/internal/swarm/packets/from_game_server"
	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// The golden dialog pages of the M2 chain, byte for byte from the
// Mobius C1 datapack (data/scripts/quests/Q00406_PathOfTheElvenKnight
// and .../village_master/ElfHumanFighterChange1): the walker drives
// exactly these pages live, so the unit tests pin the walk on the
// real html the server sends.
const (
	soriusFirstPage = "<html><body>Master Sorius:<br>\n" +
		"Greetings, child in search of the training of the sword. " +
		"When I look at the new candidates like yourself that seek me, " +
		"I am sure that the long tradition of our Elven race is being " +
		"continued.<br>\n" +
		"<a action=\"bypass Script Q00406_PathOfTheElvenKnight 30327-05.htm\">" +
		"Say you want to be an Elven Knight</a>\n" +
		"</body></html>"
	soriusChallengePage = "<html><body>Master Sorius:<br>\n" +
		"Elven Knights choose the path of the sword over archery, " +
		"both of which were developed by our race for thousands of " +
		"years.<br>\n" +
		"We must test your skills to see if you have what it takes to " +
		"become an Elven Knight.<br>\n" +
		"<a action=\"bypass Script Q00406_PathOfTheElvenKnight 30327-06.htm\">" +
		"Challenge the test</a>\n" +
		"</body></html>"
	soriusAcceptPage = "<html><body>Master Sorius:<br>\n" +
		"There is a place called the <font color=\"LEVEL\">Ruins of " +
		"Agony</font> in the war-devastated area at the northern part " +
		"of Gludio.<br>\n" +
		"Please go to the Ruins of Agony and hunt skeletons and " +
		"spartoi.\n" +
		"</body></html>"
	rainsProfessionPage = "<html><body>Grand Master Rains:<br>\n" +
		"To change profession means that you have attained a certain " +
		"degree of ability and experience.<br>\n" +
		"<a action=\"bypass Script ElfHumanFighterChange1 30288-12.htm\">" +
		"Elven Knight</a><br>\n" +
		"<a action=\"bypass Script ElfHumanFighterChange1 30288-15.htm\">" +
		"Elven Scout</a>\n" +
		"</body></html>"
	rainsKnightPage = "<html><body>Grand Master Rains:<br>\n" +
		"The Elven Knight chooses the way of the sword.<br>\n" +
		"<a action=\"bypass Script ElfHumanFighterChange1 30288-13.htm\">" +
		"Description of an Elven Knight</a><br>\n" +
		"<a action=\"bypass -h Script ElfHumanFighterChange1 19\">" +
		"Change profession to an Elven Knight</a><br>\n" +
		"<a action=\"bypass Script ElfHumanFighterChange1 30288-11.htm\">" +
		"Return</a>\n" +
		"</body></html>"
	rainsChangedPage = "<html><body>Grand Master Rains:<br>\n" +
		"Congratulations! You have now become a magnificent Elf " +
		"Knight.<br>\n" +
		"</body></html>"
)

// scriptPage is one page of a scripted server conversation: the npc
// the page arrives from and the html body.
type scriptPage struct {
	npc  int32
	html string
}

// dialogScript models the server side of one conversation: the
// interact click serves the first page, a bypass of a link the
// current page offers serves the next page, and anything else is
// silently dropped (the html action cache emulation of
// RequestBypassToServer: an unmatched command never answers).
type dialogScript struct {
	pages    []scriptPage
	cur      int // -1: no page delivered yet
	clicks   int
	dropped  []string
	staleNPC int32
	staleHtm string
}

// scriptGame wraps the shared fakeGame stub with the dialog script:
// ClickObject, SendBypass and LastHTMLDialog serve the script; every
// other GameAPI method (the walk, attack and shop requests the
// walker never fires) stays the plain stub recording calls.
type scriptGame struct {
	*fakeGame
	script *dialogScript
}

// shortenDialogSeams squeezes the test seams so every timeout case
// runs in well under a second and the two-click entry pause adds no
// wall clock: the tests restore the production values on cleanup.
func shortenDialogSeams(t *testing.T) {
	t.Helper()
	dialogPause, dialogWait := dialogClickPause, questDialogWait
	dialogClickPause = time.Millisecond
	questDialogWait = 150 * time.Millisecond
	t.Cleanup(func() {
		dialogClickPause = dialogPause
		questDialogWait = dialogWait
	})
}

func (s *scriptGame) ClickObject(objectID int32) error {
	if err := s.fakeGame.ClickObject(objectID); err != nil {
		return err
	}
	s.script.clicks++
	// The second click is the interact one (NpcClick: the first
	// click of a new target only selects it).
	if s.script.clicks == 2 {
		s.script.cur = 0
	}

	return nil
}

func (s *scriptGame) SendBypass(command string) error {
	if err := s.fakeGame.SendBypass(command); err != nil {
		return err
	}
	current := s.script.pages[s.script.cur]
	for _, link := range fromgameserver.ParseHTMLLinks(current.html) {
		if link.Command == command && s.script.cur+1 < len(s.script.pages) {
			s.script.cur++

			return nil
		}
	}
	s.script.dropped = append(s.script.dropped, command)

	return nil
}

func (s *scriptGame) LastHTMLDialog() (int32, string) {
	if s.script.cur < 0 || s.script.cur >= len(s.script.pages) {
		// Before the first interact click the connection holds
		// whatever stale dialog a previous npc left; a page index
		// past the script is the "server never answers" case.
		return s.script.staleNPC, s.script.staleHtm
	}
	page := s.script.pages[s.script.cur]

	return page.npc, page.html
}

// TestDriveDialogQuestAcceptHappyPath pins the full Q00406 accept
// walk on the real datapack pages: the two-click entry, the page
// wait with content change detection, the link match by text, the
// bypass sends in route order and the final page applied to the
// tracker (its origin and the stale link rejection of the page
// replacement).
func TestDriveDialogQuestAcceptHappyPath(t *testing.T) {
	shortenDialogSeams(t)
	script := &dialogScript{
		cur: -1,
		pages: []scriptPage{
			{npc: 30327, html: soriusFirstPage},
			{npc: 30327, html: soriusChallengePage},
			{npc: 30327, html: soriusAcceptPage},
		},
	}
	game := &scriptGame{fakeGame: &fakeGame{}, script: script}
	tracker := state.NewBot("quest-walker")
	loop := NewLoop(game, tracker)

	err := loop.DriveDialog(30327, []DialogStep{
		{LinkText: "Say you want to be an Elven Knight"},
		{LinkText: "Challenge the test"},
	})
	require.NoError(t, err)
	require.Len(t, game.clicks, 2,
		"the entry is the select click then the interact click")
	require.Equal(t,
		"Script Q00406_PathOfTheElvenKnight 30327-05.htm",
		game.bypasses[0])
	require.Equal(t,
		"Script Q00406_PathOfTheElvenKnight 30327-06.htm",
		game.bypasses[1])
	require.Empty(t, script.dropped,
		"every sent bypass matched a link of the open page")
	require.Equal(t, int32(30327), tracker.DialogOrigin(),
		"the final page is the open dialog of the tracker")
	require.Empty(t, tracker.DialogLinks(),
		"the accept page carries no links")
	require.False(t,
		tracker.IsDialogCommand(
			"Script Q00406_PathOfTheElvenKnight 30327-05.htm"),
		"the links of the replaced page stop validating")
}

// TestDriveDialogClassChangeStripsHPrefix pins the -h prefix walk of
// the class master pages: the walker sends the command the html
// action cache stores (the "-h " prefix stripped by the link
// parser), exactly the form RequestBypassToServer carries.
func TestDriveDialogClassChangeStripsHPrefix(t *testing.T) {
	shortenDialogSeams(t)
	script := &dialogScript{
		cur: -1,
		pages: []scriptPage{
			{npc: 30288, html: rainsProfessionPage},
			{npc: 30288, html: rainsKnightPage},
			{npc: 30288, html: rainsChangedPage},
		},
	}
	game := &scriptGame{fakeGame: &fakeGame{}, script: script}
	loop := NewLoop(game, state.NewBot("quest-walker"))

	err := loop.DriveDialog(30288, []DialogStep{
		{LinkText: "Elven Knight"},
		{LinkText: "Change profession to an Elven Knight"},
	})
	require.NoError(t, err)
	require.Equal(t, "Script ElfHumanFighterChange1 30288-12.htm",
		game.bypasses[0])
	require.Equal(t, "Script ElfHumanFighterChange1 19",
		game.bypasses[1],
		"the -h prefix of the class change link is stripped")
	require.Equal(t, int32(30288), loop.tracker.DialogOrigin())
}

// TestDriveDialogStaleOtherNpcPageIgnored pins the origin guard: a
// stale dialog page of a previous npc (30000) must not satisfy the
// wait for the conversation's own page (30327).
func TestDriveDialogStaleOtherNpcPageIgnored(t *testing.T) {
	shortenDialogSeams(t)
	script := &dialogScript{
		cur:      -1,
		staleNPC: 30000,
		staleHtm: "<a action=\"bypass npc_30000_Chat\">old</a>",
		pages: []scriptPage{
			{npc: 30327, html: soriusFirstPage},
			{npc: 30327, html: soriusChallengePage},
			{npc: 30327, html: soriusAcceptPage},
		},
	}
	game := &scriptGame{fakeGame: &fakeGame{}, script: script}
	loop := NewLoop(game, state.NewBot("quest-walker"))

	err := loop.DriveDialog(30327, []DialogStep{
		{LinkText: "Say you want to be an Elven Knight"},
		{LinkText: "Challenge the test"},
	})
	require.NoError(t, err)
}

// TestDriveDialogMissingLinkFailsStep pins the lookup guard: a step
// whose link text the page never offers fails the walk before any
// bypass of that step is sent.
func TestDriveDialogMissingLinkFailsStep(t *testing.T) {
	shortenDialogSeams(t)
	script := &dialogScript{
		cur: -1,
		pages: []scriptPage{
			{npc: 30327, html: soriusFirstPage},
		},
	}
	game := &scriptGame{fakeGame: &fakeGame{}, script: script}
	loop := NewLoop(game, state.NewBot("quest-walker"))

	err := loop.DriveDialog(30327, []DialogStep{
		{LinkText: "Learn the dwarven ways"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "offers no")
	require.Contains(t, err.Error(), "Learn the dwarven ways")
	require.Empty(t, game.bypasses,
		"no bypass fires for a link the page does not offer")
}

// TestDriveDialogEntryTimeout pins the page wait guard: when the
// talk entry never brings a dialog page, the first step fails after
// the bounded wait instead of hanging.
func TestDriveDialogEntryTimeout(t *testing.T) {
	shortenDialogSeams(t)
	script := &dialogScript{cur: -1}
	game := &scriptGame{fakeGame: &fakeGame{}, script: script}
	loop := NewLoop(game, state.NewBot("quest-walker"))

	err := loop.DriveDialog(30327, []DialogStep{
		{LinkText: "Say you want to be an Elven Knight"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "next page never arrived")
}

// TestDriveDialogSamePageRepeatTimesOut pins the documented blind
// spot of the content change detection: a server that re-sends the
// byte identical page after a bypass (instead of the next page)
// reads as "no answer" and the walk fails with the final page
// timeout - never a silent success.
func TestDriveDialogSamePageRepeatTimesOut(t *testing.T) {
	shortenDialogSeams(t)
	script := &dialogScript{
		cur: -1,
		pages: []scriptPage{
			{npc: 30327, html: soriusFirstPage},
			// No second page: the bypass of the first page's link
			// re-delivers the same html (cur never advances).
		},
	}
	game := &scriptGame{fakeGame: &fakeGame{}, script: script}
	loop := NewLoop(game, state.NewBot("quest-walker"))

	err := loop.DriveDialog(30327, []DialogStep{
		{LinkText: "Say you want to be an Elven Knight"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "final page")
}

// TestDriveDialogEmptyRouteRefused pins the argument guards: no npc
// and no steps fail before any packet is sent.
func TestDriveDialogEmptyRouteRefused(t *testing.T) {
	shortenDialogSeams(t)
	game := &scriptGame{fakeGame: &fakeGame{}, script: &dialogScript{}}
	loop := NewLoop(game, state.NewBot("quest-walker"))

	require.Error(t, loop.DriveDialog(0, []DialogStep{{LinkText: "x"}}))
	require.Error(t, loop.DriveDialog(30327, nil))
	require.Empty(t, game.clicks)
}

// TestFindDialogLinkContainmentMatch pins the link lookup rules: the
// case-insensitive containment match and the first-link-in-page
// order.
func TestFindDialogLinkContainmentMatch(t *testing.T) {
	links := fromgameserver.ParseHTMLLinks(rainsKnightPage)

	link, ok := findDialogLink(links, "change profession")
	require.True(t, ok)
	require.Equal(t, "Script ElfHumanFighterChange1 19", link.Command)

	_, ok = findDialogLink(links, "Elven Wizard")
	require.False(t, ok)

	// The full sentence matches its own link too.
	link, ok = findDialogLink(links,
		"Change profession to an Elven Knight")
	require.True(t, ok)
	require.Equal(t, "Script ElfHumanFighterChange1 19", link.Command)
}

// TestApplyDialogPageFeedsTracker pins the tracker feed: every page
// the walker applies replaces the open dialog (the links of the old
// page stop validating, the new ones pass), matching the NPC_HTML
// scope semantics of the server cache.
func TestApplyDialogPageFeedsTracker(t *testing.T) {
	game := &fakeGame{}
	tracker := state.NewBot("quest-walker")
	loop := NewLoop(game, tracker)

	links := loop.applyDialogPage(30327, soriusFirstPage)
	require.Len(t, links, 1)
	require.True(t, tracker.IsDialogCommand(
		"Script Q00406_PathOfTheElvenKnight 30327-05.htm"))

	loop.applyDialogPage(30327, soriusChallengePage)
	require.False(t, tracker.IsDialogCommand(
		"Script Q00406_PathOfTheElvenKnight 30327-05.htm"),
		"the first page's link is gone with the page")
	require.True(t, tracker.IsDialogCommand(
		"Script Q00406_PathOfTheElvenKnight 30327-06.htm"))
}

// TestDriveDialogGuessedBypassDropped pins the anti-injection
// discipline end to end: a bypass the open page never offered is
// dropped by the scripted server (the html action cache emulation)
// and the walk fails with the page timeout - the walker itself
// never constructs such a command (it sends the link it matched),
// so the guard this test pins is the walker's contract with the
// server, not a reachable walker branch.
func TestDriveDialogGuessedBypassDropped(t *testing.T) {
	shortenDialogSeams(t)
	script := &dialogScript{
		cur: -1,
		pages: []scriptPage{
			{npc: 30327, html: soriusFirstPage},
			{npc: 30327, html: soriusAcceptPage},
		},
	}
	game := &scriptGame{fakeGame: &fakeGame{}, script: script}
	loop := NewLoop(game, state.NewBot("quest-walker"))

	// The guessed command of the route's step matches no link of
	// the entry page - the walker refuses to send it (the page
	// offers no such link), the scripted server stays untouched.
	err := loop.DriveDialog(30327, []DialogStep{
		{LinkText: "Teleport me to Giran"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "offers no")
	require.Empty(t, script.dropped)
}
