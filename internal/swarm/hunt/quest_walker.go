// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
    "errors"
    "fmt"
    "strings"
    "time"

    fromgameserver "github.com/melg8/swarm/internal/swarm/packets/from_game_server"
    "github.com/melg8/swarm/internal/swarm/state"
)

// questDialogWait bounds the wait for one dialog page of a walked conversation:
// the server answers a talk click or a bypass within a few hundred milliseconds
// (the ScriptLink -> notifyEvent -> showResult chain is synchronous), so five
// seconds covers a slow tick and a brief stall. Past it the walker fails the
// step instead of hanging the loop. The var is a test seam.
var questDialogWait = 5 * time.Second

// pace blocks the caller for one protocol pacing wait: the flood protector
// pause between the two talk clicks, the dialog and walk poll periods. The
// production build sleeps, the tests swap it for a no-op (see disablePace in
// loop_test.go); the package tests never run in parallel, so the swap is race
// free.
var pace = time.Sleep

// questDialogPoll paces the new-page wait (the gatekeeper dialog poll period).
const questDialogPoll = 250 * time.Millisecond

// dialogClickPause spaces the two talk clicks: the server flood protector
// accepts one player action per second, so the interacting click must not ride
// the selecting one. The var is a test seam.
var dialogClickPause = selectPeriod

// dialogBypassPace is the minimum pause between two bypass sends of one
// conversation: the deployed FloodProtectorServerBypassInterval (three 100 ms
// game ticks) drops a bypass that arrives inside the window without any answer,
// and the page wait alone rides under it. The var is a test seam.
var dialogBypassPace = 3200 * time.Millisecond

// bypassPacer spaces the bypass sends of one conversation: the server drops a
// command that arrives inside the flood protector window of the previous one,
// and a zero pacer state sends at once (the first bypass has no predecessor).
type bypassPacer struct {
    last time.Time
}

// wait blocks until the next bypass may go out and arms the window
// from the current moment.
func (p *bypassPacer) wait() {
    if !p.last.IsZero() {
        if wait := dialogBypassPace - time.Since(p.last); wait > 0 {
            pace(wait)
        }
    }
    p.last = time.Now()
}

// DialogStep is one link choice of a dialog route: the walker picks the link
// whose visible text contains LinkText and sends the bypass command that link
// carries. AnswerIsEffect marks the last step of a route whose bypass answers
// with its effect instead of a dialog page (the Newbie Guide support magic
// casts every level-eligible buff and sends no NpcHtmlMessage), and the caller
// owns the wait for the effect itself.
type DialogStep struct {
    LinkText string
    // AnswerIsEffect skips the final page wait: the answer is the effect, not
    // an html page, and only the last step carries the flag.
    AnswerIsEffect bool
}

// DriveDialog walks one NPC conversation end to end: the entry (the paced
// two-click talk of talkToNpc), then per route step the new page wait, the link
// match by text, the IsDialogCommand validation and the bypass send, with the
// final answer page applied before the return. Every wait is gated on the
// arrival generation of the dialog store, so a stale page of the same npc never
// passes and the first bypass follows the fresh entry page (docs/hunting.md,
// the quest dialog walker). A failed step returns an error naming it.
func (l *Loop) DriveDialog(npcObjID int32, steps []DialogStep) error {
    if npcObjID == 0 {
        return errors.New("dialog: no npc to talk to")
    }
    if len(steps) == 0 {
        return errors.New("dialog: the route has no steps")
    }
    // The arrival generation the talk starts from: every page of this
    // conversation must arrive past it.
    _, _, talkGen := l.game.LastHTMLDialogArrival()
    if err := l.talkToNpc(npcObjID); err != nil {
        return fmt.Errorf("dialog: the talk entry failed: %w", err)
    }
    lastGen := talkGen
    pacer := bypassPacer{last: time.Time{}}
    for i := range steps {
        html, gen, err := l.awaitNewDialogPage(npcObjID, lastGen)
        if err != nil {
            return fmt.Errorf("dialog: step %d (%q): %w",
                i+1, steps[i].LinkText, err)
        }
        lastGen = gen
        links := l.applyDialogPage(npcObjID, html)
        link, ok := findDialogLink(links, steps[i].LinkText)
        if !ok {
            return fmt.Errorf(
                "dialog: step %d: the page offers no %q link",
                i+1, steps[i].LinkText)
        }
        if !l.tracker.IsDialogCommand(link.Command) {
            return fmt.Errorf(
                "dialog: step %d: the %q link command %q failed "+
                    "the open page validation",
                i+1, steps[i].LinkText, link.Command)
        }
        // The bypass flood protector pace: a bypass riding the
        // previous one inside the 300 ms window is dropped silently.
        pacer.wait()
        l.logf("dialog: step %d sends %q", i+1, link.Command)
        if err := l.game.SendBypass(link.Command); err != nil {
            return fmt.Errorf(
                "dialog: step %d: the bypass send failed: %w",
                i+1, err)
        }
        // The effect answer of the last step: the bypass answered with its
        // effect, no page follows and the caller owns the effect wait.
        if steps[i].AnswerIsEffect && i == len(steps)-1 {
            return nil
        }
    }
    html, _, err := l.awaitNewDialogPage(npcObjID, lastGen)
    if err != nil {
        return fmt.Errorf("dialog: the final page: %w", err)
    }
    l.applyDialogPage(npcObjID, html)

    return nil
}

// talkToNpc opens the dialog of an npc the character already stands inside the
// interaction distance of: the plain client talk is two paced Action clicks -
// the first click of a new target only selects it, the second walks the
// interact branch (outside the distance the server pulls the character over),
// and a click on the npc that is already the target interacts right away.
func (l *Loop) talkToNpc(npcObjID int32) error {
    if err := l.game.ClickObject(npcObjID); err != nil {
        return err
    }
    pace(dialogClickPause)

    return l.game.ClickObject(npcObjID)
}

// awaitNewDialogPage waits for a dialog page of the npc that arrived past the
// given generation: the connection layer stores only the last html, so the
// arrival generation is the arrival signal - a page the store already held when
// the talk began never passes, while a genuine byte identical re-send does (the
// generation advanced). The accepted generation returns for the next anchor.
func (l *Loop) awaitNewDialogPage(
    npcObjID int32, lastGen uint64,
) (string, uint64, error) {
    deadline := l.now().Add(questDialogWait)
    for {
        id, html, gen := l.game.LastHTMLDialogArrival()
        if id == npcObjID && html != "" && gen > lastGen {
            return html, gen, nil
        }
        if l.now().After(deadline) {
            return "", 0, errors.New(
                "the next page never arrived")
        }
        pace(questDialogPoll)
    }
}

// applyDialogPage parses the bypass links of a dialog page and feeds the
// tracker open dialog section before any bypass of that page is sent, so the
// IsDialogCommand validation runs against the page the command came from. The
// returned links are the parsed copy the caller matches steps by.
func (l *Loop) applyDialogPage(
    npcObjID int32, html string,
) []fromgameserver.HTMLLink {
    links := fromgameserver.ParseHTMLLinks(html)
    views := make([]state.DialogLinkView, 0, len(links))
    for i := range links {
        views = append(views, state.DialogLinkView{
            Command: links[i].Command,
            Text:    links[i].Text,
        })
    }
    // ItemID stays 0: the html seam of the GameAPI carries no item id.
    l.tracker.ApplyDialog(state.DialogPageView{
        NpcObjID: npcObjID,
        ItemID:   0,
        Links:    views,
    })

    return links
}

// noDialogLink is the zero return of findDialogLink's miss (a var,
// not a literal, keeps the exhaustruct gate of the link struct).
var noDialogLink fromgameserver.HTMLLink

// findDialogLink returns the first link whose visible text contains the wanted
// text: the match is case-insensitive containment, so a route step names a
// distinctive substring of the link label without pinning the full sentence.
// The first link in page order wins, matching the top-to-bottom reading.
func findDialogLink(
    links []fromgameserver.HTMLLink, text string,
) (fromgameserver.HTMLLink, bool) {
    want := strings.ToLower(text)
    for i := range links {
        if strings.Contains(strings.ToLower(links[i].Text), want) {
            return links[i], true
        }
    }

    return noDialogLink, false
}
