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

// questDialogWait bounds the wait for one dialog page of a walked
// conversation: the server answers a talk click or a bypass within
// a few hundred milliseconds (the ScriptLink -> notifyEvent ->
// showResult chain is synchronous), so five seconds covers a slow
// tick and a brief stall. Past it the walker fails the step instead
// of hanging the loop. The var (not a const) is a test seam: the
// unit tests shorten it to keep the timeout case under a second.
var questDialogWait = 5 * time.Second

// questDialogPoll paces the new-page wait: the walker reads the
// last dialog every tick until the page from the npc changes or
// the wait lapses (the same period as the gatekeeper dialog poll).
const questDialogPoll = 250 * time.Millisecond

// dialogClickPause spaces the two talk clicks: the server flood
// protector accepts one player action per second, so the second
// (interacting) click must not ride the first (selecting) one. The
// var (not a const) is a test seam.
var dialogClickPause = selectPeriod

// dialogBypassPace is the minimum pause between two bypass sends of
// one conversation: the server bypass flood protector
// (FloodProtectorServerBypassInterval = 3 of the deployed
// FloodProtector.ini) silently drops a bypass that arrives sooner -
// the live Q00406 accept run of 2026-09-12 lost the second page
// link to it (the page never answered, the step timed out). The
// margin above the 3 s interval covers the protector's rounding.
// The var (not a const) is a test seam.
var dialogBypassPace = 3200 * time.Millisecond

// DialogStep is one link choice of a dialog route: the walker picks
// the link whose visible text contains LinkText and sends the
// bypass command that link carries. The text of the quest and class
// master pages is stable data (the datapack html), so a distinctive
// substring - "Challenge the test", "Change profession to an
// Elven Knight" - identifies the step's link.
type DialogStep struct {
	LinkText string
}

// DriveDialog walks one NPC conversation end to end: the entry (the
// paced two-click talk of talkToNpc), then for every route step the
// new page wait (awaitNewDialogPage), the link match by text
// (findDialogLink), the IsDialogCommand validation of the tracker
// and the bypass send. The answer page of the last bypass is
// awaited and applied as well, so the tracker holds the final page
// (its links, its origin) when the call returns - the quest journal
// push of the same answer lands in the tracker through the QuestList
// apply path.
//
// The caller keeps the character within the interaction distance of
// the npc for the whole conversation (250 units - the server gates
// every bypass and quest event on it, see
// Player.processScriptEvent); the walker's clicks refresh the
// server's last-folk memory at the entry only.
//
// A step whose page never arrives, whose link text is not offered,
// or whose command fails the validation returns an error naming the
// step - the caller decides whether to retry the conversation from
// the entry (the server state of the quest makes the re-entry
// idempotent: re-talking re-sends the page of the current cond).
func (l *Loop) DriveDialog(npcObjID int32, steps []DialogStep) error {
	if npcObjID == 0 {
		return errors.New("dialog: no npc to talk to")
	}
	if len(steps) == 0 {
		return errors.New("dialog: the route has no steps")
	}
	if err := l.talkToNpc(npcObjID); err != nil {
		return fmt.Errorf("dialog: the talk entry failed: %w", err)
	}
	lastPage := ""
	lastBypass := time.Time{}
	for i := range steps {
		html, err := l.awaitNewDialogPage(npcObjID, lastPage)
		if err != nil {
			return fmt.Errorf("dialog: step %d (%q): %w",
				i+1, steps[i].LinkText, err)
		}
		lastPage = html
		links := l.applyDialogPage(npcObjID, html)
		link, ok := findDialogLink(links, steps[i].LinkText)
		if !ok {
			return fmt.Errorf(
				"dialog: step %d: the page offers no %q link",
				i+1, steps[i].LinkText)
		}
		if !l.tracker.IsDialogCommand(link.Command) {
			return fmt.Errorf(
				"dialog: step %d: the %q link command %q failed the open page validation",
				i+1, steps[i].LinkText, link.Command)
		}
		// The bypass flood protector pace: a bypass riding the
		// previous one inside the 3 s window is dropped silently.
		if !lastBypass.IsZero() {
			if wait := dialogBypassPace - time.Since(lastBypass); wait > 0 {
				time.Sleep(wait)
			}
		}
		lastBypass = time.Now()
		l.logf("dialog: step %d sends %q", i+1, link.Command)
		if err := l.game.SendBypass(link.Command); err != nil {
			return fmt.Errorf(
				"dialog: step %d: the bypass send failed: %w",
				i+1, err)
		}
	}
	html, err := l.awaitNewDialogPage(npcObjID, lastPage)
	if err != nil {
		return fmt.Errorf("dialog: the final page: %w", err)
	}
	l.applyDialogPage(npcObjID, html)

	return nil
}

// talkToNpc opens the dialog of an npc the character already stands
// within the interaction distance of: the plain client talk is two
// Action clicks (NpcClick.onAction - the first click of a target
// the player does not hold yet only selects it, the second click of
// the same object id walks the interact branch and the server
// answers with the npc's html page). The two clicks pace at
// dialogClickPause (the player action flood protector accepts one
// action per second); every click refreshes the server's last-folk
// memory the quest events resolve their npc through. A click on an
// npc that is already the target interacts right away, so the entry
// is idempotent - the second click re-opens the same page.
func (l *Loop) talkToNpc(npcObjID int32) error {
	if err := l.game.ClickObject(npcObjID); err != nil {
		return err
	}
	time.Sleep(dialogClickPause)

	return l.game.ClickObject(npcObjID)
}

// awaitNewDialogPage waits for a dialog page of the npc whose
// content differs from the last seen page: the connection layer
// stores only the last html, and the pages of one conversation all
// arrive from the same npc, so the content change is the arrival
// signal of the next page. Every quest page transition changes the
// content (the links of a page carry the next page's file name);
// the one blind spot - the server re-sending a byte identical page
// after a bypass - reads as "no answer yet" and lapses into the
// timeout, which the caller treats as a failed conversation.
func (l *Loop) awaitNewDialogPage(
	npcObjID int32, lastPage string,
) (string, error) {
	deadline := time.Now().Add(questDialogWait)
	for {
		id, html := l.game.LastHTMLDialog()
		if id == npcObjID && html != "" && html != lastPage {
			return html, nil
		}
		if time.Now().After(deadline) {
			return "", errors.New(
				"the next page never arrived")
		}
		time.Sleep(questDialogPoll)
	}
}

// applyDialogPage parses the bypass links of a dialog page and
// feeds the tracker's open dialog section: the hunt-side wiring of
// the dialog state of T-013 (the connection layer stores the raw
// html of the last NpcHtmlMessage, the walker is the consumer that
// applies it). The links land in the tracker exactly once per page,
// before any bypass of that page is sent, so the IsDialogCommand
// validation runs against the page the command came from. The
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
	// ItemID stays 0: the html seam of the GameAPI carries no item
	// id (the quest flow keeps the npc scope; an item bound dialog is
	// a different consumer's work).
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

// findDialogLink returns the first link whose visible text contains
// the wanted text: the match is case-insensitive containment, so a
// route step names a distinctive substring of the link label ("the
// test" of "Challenge the test") without pinning the full datapack
// sentence. A page can offer the same text twice; the first link in
// page order wins, matching the top-to-bottom reading of a dialog.
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
