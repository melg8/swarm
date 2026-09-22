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
    // The golden guide pages of the Newbie Guide support magic,
    // byte for byte from the Mobius C1 datapack (data/html/default/
    // 30599.htm with the %objectId% placeholders substituted and
    // data/html/default/SupportMagic.htm): the deployed entry page
    // carries no comment lines (the 401 byte page of the issue #35
    // state dump), the support page links the apply bypass whose
    // answer is the buff cast itself - no page follows it.
    guideEntryPage = "<html><body>Newbie Guide:<br>\n" +
        "Don't hesitate to tell me if you require assistance. " +
        "I can teach you about a number of useful things.<br>\n" +
        "<a action=\"bypass npc_30599_Chat 1\">Ask for advice.</a><br>\n" +
        "<a action=\"bypass npc_30599_Link default/SupportMagic.htm\">" +
        "Receive help from beneficial magic.</a><br>\n" +
        "<a action=\"bypass npc_30599_Chat 14\">" +
        "Ask about the novice characters.</a>\n" +
        "</body></html>"
    guideSupportPage = "<html><body>\n" +
        "You are eligible to receive the following supplemental " +
        "magic:<br>\n" +
        "Levels 8-24: Wind Walk<br1>\n" +
        "Levels 11-24: Shield<br1>\n" +
        "Levels 12-23: Bless the Body (Fighter), Bless the Soul " +
        "(Wizard)<br1>\n" +
        "Levels 13-22: Might (Fighter), Acumen (Wizard)<br1>\n" +
        "Levels 14-21: Regeneration (Fighter), Concentration " +
        "(Wizard)<br1>\n" +
        "Levels 15-20: Haste (Fighter), Empower (Wizard)<br1>\n" +
        "Levels 16-19: Life Cubic<br1>\n" +
        "<a action=\"bypass -h npc_30599_SupportMagic\">" +
        "Receive supplemental magic.</a>\n" +
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
// RequestBypassToServer: an unmatched command never answers - the
// same silent drop the Mobius server answers a bypass sent before
// the dialog opened with). The gen field is the arrival generation
// of the store: the stale pre-conversation page sits at generation
// 0, every served page advances it, so the walker's arrival gate
// tells the fresh pages of this conversation from the yesterday
// page of a previous one.
type dialogScript struct {
    pages    []scriptPage
    cur      int // -1: no page delivered yet
    gen      uint64
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
    dialogPace := dialogBypassPace
    dialogBypassPace = time.Millisecond
    t.Cleanup(func() { dialogBypassPace = dialogPace })
}

func (s *scriptGame) ClickObject(objectID int32) error {
    if err := s.fakeGame.ClickObject(objectID); err != nil {
        return err
    }
    s.script.clicks++
    // The second click is the interact one (NpcClick: the first
    // click of a new target only selects it): the server opens the
    // dialog and sends the entry page - a fresh arrival.
    if s.script.clicks == 2 {
        s.script.cur = 0
        s.script.gen++
    }

    return nil
}

func (s *scriptGame) SendBypass(command string) error {
    if err := s.fakeGame.SendBypass(command); err != nil {
        return err
    }
    // No open page: the server never opened the dialog, the html
    // action cache holds no such link - the bypass is dropped
    // without an answer (the race the arrival gate closes).
    if s.script.cur < 0 || s.script.cur >= len(s.script.pages) {
        s.script.dropped = append(s.script.dropped, command)

        return nil
    }
    current := s.script.pages[s.script.cur]
    for _, link := range fromgameserver.ParseHTMLLinks(current.html) {
        if link.Command == command && s.script.cur+1 < len(s.script.pages) {
            s.script.cur++
            s.script.gen++

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

func (s *scriptGame) LastHTMLDialogArrival() (int32, string, uint64) {
    if s.script.cur < 0 || s.script.cur >= len(s.script.pages) {
        // The stale page of a previous conversation: it arrived
        // before this talk began, its generation never advances.
        return s.script.staleNPC, s.script.staleHtm, 0
    }
    page := s.script.pages[s.script.cur]

    return page.npc, page.html, s.script.gen
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

// TestDriveDialogNoAnswerTimesOut pins the no-answer guard: a server
// that never answers the step's bypass (the command is dropped - the
// html action cache holds no such link) fails the walk with the
// final page timeout - never a silent success.
func TestDriveDialogNoAnswerTimesOut(t *testing.T) {
    shortenDialogSeams(t)
    script := &dialogScript{
        cur: -1,
        pages: []scriptPage{
            {npc: 30327, html: soriusFirstPage},
            // No second page: the bypass of the first page's link
            // is dropped, no page follows (cur never advances).
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

// TestDriveDialogSamePageResendAdvances pins the arrival gate on the
// byte identical re-send: a server that answers the step's bypass
// with the SAME page again (the dialog state returned to it) is a
// fresh arrival - the generation advanced - and the route proceeds
// through it. The old content comparison read the identical html as
// "no answer" and failed the walk; the arrival generation tells the
// two apart.
func TestDriveDialogSamePageResendAdvances(t *testing.T) {
    shortenDialogSeams(t)
    script := &dialogScript{
        cur: -1,
        pages: []scriptPage{
            {npc: 30327, html: soriusFirstPage},
            {npc: 30327, html: soriusFirstPage},
        },
    }
    game := &scriptGame{fakeGame: &fakeGame{}, script: script}
    loop := NewLoop(game, state.NewBot("quest-walker"))

    err := loop.DriveDialog(30327, []DialogStep{
        {LinkText: "Say you want to be an Elven Knight"},
    })
    require.NoError(t, err,
        "the byte identical answer page is a fresh arrival")
}

// TestDriveDialogStaleSameNpcPageWaitsForFreshEntry pins the issue
// #35 regression: a stale entry page of the SAME npc left in the
// dialog store by the PREVIOUS conversation (the recurring Newbie
// Guide stop) must never satisfy the entry wait - the first bypass
// goes out only after the fresh entry page arrived, so the server
// opened the dialog and rebuilt the html action cache the bypass
// validates against. The state dump of the issue pinned the race:
// the step 1 bypass logged BEFORE the fresh 401 byte entry page
// arrived, the bypass hit the stale server cache, was dropped
// silently and step 2 timed out.
func TestDriveDialogStaleSameNpcPageWaitsForFreshEntry(t *testing.T) {
    shortenDialogSeams(t)
    script := &dialogScript{
        cur: -1,
        // The yesterday page of the guide conversation: the byte
        // identical entry page a previous stop ended on (the
        // content comparison of the old walker could not tell it
        // from the fresh arrival).
        staleNPC: 30599,
        staleHtm: guideEntryPage,
        pages: []scriptPage{
            {npc: 30599, html: guideEntryPage},
            {npc: 30599, html: guideSupportPage},
        },
    }
    game := &scriptGame{fakeGame: &fakeGame{}, script: script}
    loop := NewLoop(game, state.NewBot("quest-walker"))

    err := loop.DriveDialog(30599, []DialogStep{
        {LinkText: "Receive help from beneficial magic."},
        {LinkText: "Receive supplemental magic.", AnswerIsEffect: true},
    })
    require.NoError(t, err,
        "the stale page never satisfies the wait, the fresh entry "+
            "page does")
    require.Len(t, game.bypasses, 2)
    require.Equal(t, "npc_30599_Link default/SupportMagic.htm",
        game.bypasses[0])
    require.Equal(t, "npc_30599_SupportMagic", game.bypasses[1])
    require.Equal(t, []string{"npc_30599_SupportMagic"}, script.dropped,
        "only the apply bypass is unanswered - its answer is the "+
            "buff effect; the entry Link bypass was answered with "+
            "the SupportMagic page (no race with the dialog open)")
}

// TestDriveDialogEffectAnswerSkipsFinalWait pins the AnswerIsEffect
// contract: the last step's bypass answers with its effect (the
// buffs of the guide gate), not with a dialog page, so the walker
// sends it and returns without the final page wait - while a route
// without the flag still awaits the final page and fails when
// nothing arrives (the quest default).
func TestDriveDialogEffectAnswerSkipsFinalWait(t *testing.T) {
    shortenDialogSeams(t)
    build := func() *scriptGame {
        script := &dialogScript{
            cur: -1,
            pages: []scriptPage{
                {npc: 30599, html: guideEntryPage},
                {npc: 30599, html: guideSupportPage},
                // No third page: the support magic bypass answers
                // with the buffs, never with a page.
            },
        }

        return &scriptGame{fakeGame: &fakeGame{}, script: script}
    }

    // The effect answer route: no final wait, no error.
    game := build()
    loop := NewLoop(game, state.NewBot("quest-walker"))
    err := loop.DriveDialog(30599, []DialogStep{
        {LinkText: "Receive help from beneficial magic."},
        {LinkText: "Receive supplemental magic.", AnswerIsEffect: true},
    })
    require.NoError(t, err,
        "the effect answer never waits for a page")
    require.Len(t, game.bypasses, 2)

    // The same script without the flag: the final page wait fails.
    game = build()
    loop = NewLoop(game, state.NewBot("quest-walker"))
    err = loop.DriveDialog(30599, []DialogStep{
        {LinkText: "Receive help from beneficial magic."},
        {LinkText: "Receive supplemental magic."},
    })
    require.Error(t, err,
        "the page answer default still waits for the final page")
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
