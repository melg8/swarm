// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/melg8/swarm/internal/swarm/connection"
	"github.com/melg8/swarm/internal/swarm/hunt"
	"github.com/melg8/swarm/internal/swarm/state"
)

// The class transfer scenario (T-016, the M2 acceptance vehicle):
// a DB-injected level 19 elven fighter at Gludio drives the Q00406
// chain of hunt/quest_chains.go through the dialog walker of
// hunt/quest_walker.go. The build is staged - this round ships the
// ACCEPT leg (the journal cond flip of the quest start); the kill
// stages (the quest trip phase of the hunt loop, the Gludio band
// combat) and the Rains class change join in the follow-up rounds,
// the full SelfClassID gate closes M2.
const (
	classTransferScenarioID = "class-transfer"
	classTransferAccount    = "temp9"
	classTransferPassword   = "temp9"
	// classTransferTimeout bounds the whole run: the accept leg is
	// the two session setups, the world entry and one dialog walk
	// (the live walker round trip measured under 2 s), so the bound
	// holds the injection retries and a slow login queue with a
	// wide margin.
	classTransferTimeout = 8 * time.Minute
	// classTransferLevel is the injected level: 19 passes the
	// Q00406 start gate (the 05 event checks level < 19).
	classTransferLevel = 19
)

// The Gludio injection geometry: the character wakes on the
// approach ring south of Master Sorius (GludioNPCs.xml: -13440
// 122643 -3103), 150 units off his cell - inside the server
// interaction distance without standing on the trainer hall
// interior cell itself (the roof rule of the teacher approach).
const (
	soriusApproachX = -13440
	soriusApproachY = 122493
	soriusApproachZ = -3103
)

// questNpcTemplateOffset mirrors the NpcInfo template id space of
// the world tracker: the hunt chain data carries the npcdata
// display ids, the tracker stores display id + 1000000.
const questNpcTemplateOffset = 1000000

// The condition ids of the staged checks.
const (
	checkSoriusFound   = "sorius-found"
	checkQuestAccepted = "quest-accepted"
)

// questNpcFindWait bounds the wait for the quest npc to appear in
// the world store (the spawn packets of the world entry flood in
// within the first seconds).
const questNpcFindWait = 30 * time.Second

// classTransferReset builds the start state of the class transfer
// scenario: the level 19 elven fighter with the milestone wallet
// shape (the later kill stages buy the band gear with it) standing
// on Sorius's approach ring.
func classTransferReset() characterReset {
	gain := elvenFighterVitals[classTransferLevel-1]

	return characterReset{
		Account: classTransferAccount,
		Char:    classTransferAccount,
		Level:   classTransferLevel,
		Exp: soakExperienceTable[classTransferLevel] -
			milestoneXPTail(classTransferLevel),
		SP:    milestoneSP,
		Adena: milestoneWallet,
		Items: nil,
		X:     soriusApproachX,
		Y:     soriusApproachY,
		Z:     soriusApproachZ,
		MaxHP: gain.HP,
		MaxMP: gain.MP,
		MaxCP: gain.CP,
	}
}

// classTransferChecks is the staged condition list of the scenario.
func classTransferChecks() []Check {
	return []Check{
		{
			ID: checkOnline, Label: "entered the world at Gludio",
			Done: false, Detail: "",
		},
		{
			ID: checkSoriusFound, Label: "found Master Sorius in the world",
			Done: false, Detail: "",
		},
		{
			ID:    checkQuestAccepted,
			Label: "the quest journal reports Q00406 cond 1",
			Done:  false, Detail: "",
		},
	}
}

// classTransferScenario runs the staged M2 acceptance: the
// injection, the world entry on the Sorius approach ring, the
// dialog walker drive of the chain accept route (the two page
// links of the CREATED state) and the journal flip gate. The pass
// line of the stage: QuestCond(406) == 1 within the budget.
func classTransferScenario(ctx context.Context, m *Manager, t *Test) error {
	test := t
	test.setChecks(classTransferChecks())
	test.appendLog("acceptance: the class transfer stage is the " +
		"Q00406 accept (the journal cond flip)")

	if err := m.ensureCharacter(classTransferAccount,
		classTransferPassword, classTransferAccount,
		test.appendLog); err != nil {
		writeClassTransferFailMetrics(test, err.Error())

		return fmt.Errorf("ensure character: %w", err)
	}
	time.Sleep(ensurePause)
	if err := m.injectReset(classTransferReset(), test); err != nil {
		writeClassTransferFailMetrics(test, err.Error())

		return fmt.Errorf("inject start state: %w", err)
	}
	time.Sleep(ensurePause)

	tracker := m.tracker(test)
	session, err := startClassTransferSession(ctx, m, tracker,
		test.appendLog)
	if err != nil {
		writeClassTransferFailMetrics(test, err.Error())

		return fmt.Errorf("start session: %w", err)
	}
	defer session.cancel()

	if err := waitOnline(ctx, tracker, test); err != nil {
		writeClassTransferFailMetrics(test, err.Error())
		session.cancel()
		<-session.done

		return fmt.Errorf("world entry: %w", err)
	}

	return classTransferDrive(ctx, test, tracker, session)
}

// classTransferDrive runs the staged quest leg of the scenario: the
// Sorius find, the dialog walker accept route and the journal flip
// gate, ending with the graceful teardown and the metrics row.
func classTransferDrive(
	ctx context.Context, test *Test, tracker *state.Bot,
	session *classTransferSession,
) error {
	chain := hunt.ElvenKnightChain()
	start := time.Now()
	sorius, err := awaitQuestNpc(ctx, tracker, chain.Start,
		questNpcFindWait, test)
	if err != nil {
		return classTransferAbort(test, tracker, session, start,
			"find Sorius: "+err.Error())
	}
	test.updateCheck(checkSoriusFound, true, fmt.Sprintf(
		"object %d at (%d, %d, %d)", sorius.ObjectID, sorius.X,
		sorius.Y, sorius.Z))
	test.appendLog(fmt.Sprintf("acceptance: Sorius found (object %d), "+
		"driving the accept route of %d links",
		sorius.ObjectID, len(chain.Accept)))

	// The dialog walk: the two-link accept route (the 05 challenge
	// link runs the startQuest event). The walker lives on the
	// manual hunt loop of the session (no autonomy - the elven
	// zone machinery would drag the character home).
	loop := hunt.NewLoop(session.game, tracker)
	loop.SetAutonomy(false)
	loop.SetLogger(sessionLogger(tracker, test.appendLog))
	driveStart := time.Now()
	// The quest npc conversation rides the entry prefix: the
	// static trainer page, its Quest link, the choose window link
	// of this quest - then the accept links of the CREATED pages.
	route := append(hunt.QuestEntryLinks(chain), chain.Accept...)
	if err := loop.DriveDialog(sorius.ObjectID, route); err != nil {
		return classTransferAbort(test, tracker, session, start,
			"the accept walk failed: "+err.Error())
	}
	test.appendLog(fmt.Sprintf(
		"acceptance: the accept walk completed in %.1fs, watching "+
			"the quest journal", time.Since(driveStart).Seconds()))

	// The gate: the QuestList push of the startQuest flips the
	// journal to quest 406 cond 1 (the push protocol - the client
	// never polls, the packet follows the event within a second).
	if err := awaitQuestCond(ctx, tracker, chain.QuestID, 1,
		questCondWait, test); err != nil {
		return classTransferAbort(test, tracker, session, start,
			"the journal flip: "+err.Error())
	}
	cond, _ := tracker.QuestCond(chain.QuestID)
	test.updateCheck(checkQuestAccepted, true,
		fmt.Sprintf("quest %d at cond %d", chain.QuestID, cond))

	return classTransferPass(test, tracker, session, start)
}

// classTransferAbort tears a failed drive down and writes the FAIL
// metrics row of the stage.
func classTransferAbort(
	test *Test, tracker *state.Bot, session *classTransferSession,
	start time.Time, reason string,
) error {
	writeClassTransferOutcomeMetrics(test, tracker, true,
		time.Since(start), reason)
	session.cancel()
	<-session.done

	return errors.New(reason)
}

// classTransferPass ends the session gracefully and writes the PASS
// metrics row of the staged run.
func classTransferPass(
	test *Test, tracker *state.Bot, session *classTransferSession,
	start time.Time,
) error {
	test.appendLog("acceptance: the quest journal flipped, " +
		"stopping the bot")
	session.cancel()
	sessionErr := <-session.done
	if sessionErr != nil {
		writeClassTransferOutcomeMetrics(test, tracker, true,
			time.Since(start), "session end: "+sessionErr.Error())

		return fmt.Errorf("session end: %w", sessionErr)
	}
	test.appendLog("acceptance: the bot left the world gracefully")
	writeClassTransferOutcomeMetrics(test, tracker, false,
		time.Since(start), "")

	return nil
}

// questCondWait bounds the journal flip watch (the QuestList push
// follows the startQuest event immediately; the bound covers a
// slow tick).
const questCondWait = 10 * time.Second

// awaitQuestCond waits until the quest journal reports the wanted
// cond of the quest.
func awaitQuestCond(
	ctx context.Context, tracker *state.Bot, questID, want int32,
	wait time.Duration, test *Test,
) error {
	deadline := time.Now().Add(wait)
	for {
		if cond, ok := tracker.QuestCond(questID); ok && cond == want {
			return nil
		}
		if ctx.Err() != nil {
			return fmt.Errorf("cancelled: %w", ctx.Err())
		}
		if time.Now().After(deadline) {
			cond, ok := tracker.QuestCond(questID)

			return fmt.Errorf("the quest %d never reached cond %d "+
				"(journal: cond %d, known %t)", questID, want, cond, ok)
		}
		test.appendLog(fmt.Sprintf(
			"acceptance: waiting for the quest %d journal flip", questID))
		select {
		case <-ctx.Done():
			return fmt.Errorf("cancelled: %w", ctx.Err())
		case <-time.After(time.Second):
		}
	}
}

// awaitQuestNpc waits for the quest npc to appear in the world
// store (the spawn packets of the world entry).
func awaitQuestNpc(
	ctx context.Context, tracker *state.Bot, npc hunt.QuestNpc,
	wait time.Duration, test *Test,
) (state.AttackTarget, error) {
	templates := []int32{npc.TemplateID + questNpcTemplateOffset}
	deadline := time.Now().Add(wait)
	for {
		found, ok := tracker.NearestNpcByTemplates(templates, 6000)
		if ok {
			return found, nil
		}
		if ctx.Err() != nil {
			return state.AttackTarget{}, fmt.Errorf(
				"cancelled: %w", ctx.Err())
		}
		if time.Now().After(deadline) {
			return state.AttackTarget{}, fmt.Errorf(
				"the npc %s (template %d) never appeared",
				npc.Name, npc.TemplateID)
		}
		test.appendLog(fmt.Sprintf(
			"acceptance: waiting for %s to appear in the world",
			npc.Name))
		select {
		case <-ctx.Done():
			return state.AttackTarget{}, fmt.Errorf(
				"cancelled: %w", ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// classTransferSession is the manual session of the scenario: the
// game client handle the dialog walker drives, the cancel that
// ends the receive loop and the done channel of its exit.
type classTransferSession struct {
	game   *connection.GameClient
	cancel context.CancelFunc
	done   chan error
}

// startClassTransferSession opens the game session of the scenario
// (the world entry under the scenario context) without the hunt
// autonomy: the scenario itself drives the dialog walker through
// the game handle.
func startClassTransferSession(
	ctx context.Context, m *Manager, tracker *state.Bot,
	logLine func(string),
) (*classTransferSession, error) {
	auth, game, err := m.openGame(classTransferAccount,
		classTransferPassword, m.proxy)
	if err != nil {
		return nil, err
	}
	game.SetTracker(tracker)

	charList, err := game.Authenticate(gameSessionParams(auth))
	if err != nil {
		_ = game.Close()

		return nil, fmt.Errorf("game authentication: %w", err)
	}
	slot, _, found := charList.FindCharacterByName(classTransferAccount)
	if !found {
		_ = game.Close()

		return nil, errors.New("the injected character is missing")
	}
	if err := game.EnterWorld(int32(slot)); err != nil {
		_ = game.Close()

		return nil, fmt.Errorf("enter world: %w", err)
	}
	logLine("acceptance: the class transfer session entered the world")

	sessionCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- game.Run(sessionCtx, classTransferAccount)
	}()

	return &classTransferSession{game: game, cancel: cancel, done: done}, nil
}

// writeClassTransferOutcomeMetrics appends the staged metrics row
// of the scenario (the scenario id carries the stage context, the
// quest gate of the stage rides the fail reason).
func writeClassTransferOutcomeMetrics(
	test *Test, tracker *state.Bot, failed bool, elapsed time.Duration,
	reason string,
) {
	status := statusPass
	if failed {
		status = statusFail
	}
	writeSoakMetrics(test, soakMetrics{
		Date:        nowUTC(),
		Scenario:    classTransferScenarioID,
		DurationSec: int64(elapsed / time.Second),
		StartLevel:  classTransferLevel,
		EndLevel:    tracker.SelfLevel(),
		XpPerHour:   0,
		Deaths:      0,
		Adena:       soakAdena(tracker),
		StuckEvents: 0,
		Status:      status,
		FailReason:  reason,
	})
}

// writeClassTransferFailMetrics appends the fail row of a setup
// failure (the numeric fields zero).
func writeClassTransferFailMetrics(test *Test, reason string) {
	writeSoakMetrics(test, soakMetrics{
		Date:        nowUTC(),
		Scenario:    classTransferScenarioID,
		DurationSec: 0,
		StartLevel:  0,
		EndLevel:    0,
		XpPerHour:   0,
		Deaths:      0,
		Adena:       0,
		StuckEvents: 0,
		Status:      statusFail,
		FailReason:  "setup: " + reason,
	})
}
