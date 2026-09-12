// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
	"context"
	"fmt"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
)

// farmReadinessScenario runs the user round: a reset level 15
// character with the injected wallet shops, learns, walks to its farm
// zone and kills a mob there under the auras. The hunt loop does the
// autonomous part (it is the same -hunt wiring the fleet sessions
// use); the monitor reads the tracker state until every condition of
// the story holds at once.
func farmReadinessScenario(ctx context.Context, m *Manager, t *Test) error {
	test := t
	test.setChecks(farmChecks())

	if err := m.ensureCharacter(farmAccount, farmPassword, farmAccount,
		test.appendLog); err != nil {
		return fmt.Errorf("ensure character: %w", err)
	}
	time.Sleep(ensurePause)
	if err := m.injectReset(farmReadinessReset(farmAccount), test); err != nil {
		return fmt.Errorf("inject start state: %w", err)
	}
	time.Sleep(ensurePause)

	sessionCtx, cancelSession := context.WithCancel(ctx)
	defer cancelSession()
	sessionDone := make(chan error, 1)
	go func() {
		sessionDone <- m.runSessionSupervised(sessionCtx, farmAccount,
			farmPassword, farmAccount, true, m.proxy, test.appendLog)
	}()

	tracker := m.tracker(test)
	if err := waitOnline(ctx, tracker, test); err != nil {
		cancelSession()
		<-sessionDone

		return err
	}
	test.appendLog("acceptance: the bot is in the world, watching the " +
		"conditions")

	runStartMs := time.Now().UnixMilli()
	for {
		if ctx.Err() != nil {
			cancelSession()
			<-sessionDone

			return fmt.Errorf("cancelled: %w", ctx.Err())
		}
		evaluateFarmConditions(tracker, test, runStartMs)
		if allChecksDone(test) {
			test.appendLog("acceptance: every condition holds, stopping " +
				"the bot")
			cancelSession()
			if err := <-sessionDone; err != nil {
				return fmt.Errorf("session end: %w", err)
			}
			test.appendLog("acceptance: the bot left the world gracefully")

			return nil
		}
		select {
		case <-ctx.Done():
			cancelSession()
			<-sessionDone

			return fmt.Errorf("cancelled: %w", ctx.Err())
		case <-time.After(monitorPeriod):
		}
	}
}

// buildingEntryScenario runs the trainer hall round of the
// 2026-09-12 03:56 freeze dump: the temp character wakes at the west
// aisle entrance of the trainer hall with the dump inventory and the
// learning trip armed - the sell stop walks first (every vendor trip
// sells the accumulated junk), then the teacher leg must cross the
// building entrance and reach the class master Ellenia inside the
// hall, and the lessons begin. The dump freeze held the character at
// the entrance forever; the frozen corridor ban, the detour re-plan
// and the direct server routed walk own the recovery, the offline
// reproduction lives in hunt/building_entry_test.go.
//
//nolint:dupl // mirrors zoneReturnScenario with the dump hall state and checks
func buildingEntryScenario(ctx context.Context, m *Manager, t *Test) error {
	test := t
	test.setChecks(buildingEntryChecks())

	if err := m.ensureCharacter(entryAccount, entryPassword, entryAccount,
		test.appendLog); err != nil {
		return fmt.Errorf("ensure character: %w", err)
	}
	time.Sleep(ensurePause)
	if err := m.injectReset(buildingEntryReset(entryAccount), test); err != nil {
		return fmt.Errorf("inject start state: %w", err)
	}
	time.Sleep(ensurePause)

	sessionCtx, cancelSession := context.WithCancel(ctx)
	defer cancelSession()
	sessionDone := make(chan error, 1)
	go func() {
		sessionDone <- m.runSessionSupervised(sessionCtx, entryAccount,
			entryPassword, entryAccount, true, m.proxy, test.appendLog)
	}()

	tracker := m.tracker(test)
	if err := waitOnline(ctx, tracker, test); err != nil {
		cancelSession()
		<-sessionDone

		return err
	}
	test.appendLog("acceptance: the bot is in the world at the hall " +
		"entrance, watching the teacher walk")

	for {
		if ctx.Err() != nil {
			cancelSession()
			<-sessionDone

			return fmt.Errorf("cancelled: %w", ctx.Err())
		}
		evaluateBuildingEntryConditions(tracker, test)
		if allChecksDone(test) {
			test.appendLog("acceptance: the character reached the teacher " +
				"and the lessons began, stopping the bot")
			cancelSession()
			if err := <-sessionDone; err != nil {
				return fmt.Errorf("session end: %w", err)
			}
			test.appendLog("acceptance: the bot left the world gracefully")

			return nil
		}
		select {
		case <-ctx.Done():
			cancelSession()
			<-sessionDone

			return fmt.Errorf("cancelled: %w", ctx.Err())
		case <-time.After(monitorPeriod):
		}
	}
}

// evaluateBuildingEntryConditions rewrites the check list of the
// trainer hall entry scenario from the live tracker state: the walk
// up to the teacher npc and the first learned lesson.
func evaluateBuildingEntryConditions(tracker *state.Bot, test *Test) {
	if tracker.Status() != state.StatusOnline {
		return
	}
	teacherDetail, atTeacher := evaluateTeacher(tracker)
	test.updateCheck(checkTeacher, atTeacher, teacherDetail)
	lessonDetail, learned := evaluateLesson(tracker)
	test.updateCheck(checkLesson, learned, lessonDetail)
}

// zoneReturnScenario runs the stuck cell round: the temp character
// wakes at the reported freeze position with the reported item set
// and must walk to its selected hunting zone on its own - the freeze
// of the report (the round 58 dump) held a character on that very
// cell forever, the reproduction and the fix live in the hunt package.
// runSupervisedScenario drives the common skeleton of the supervised
// temp bot scenarios: ensure the temp character exists, inject the
// start state, run the supervised hunt session behind it and watch
// the check list until every condition holds (the context cancel
// unwinds the session either way). The zone return and the gear gap
// rounds share it - their stories differ only in the start state and
// the conditions.
func runSupervisedScenario(
	ctx context.Context, m *Manager, t *Test, reset characterReset,
	evaluate func(*state.Bot, *Test), watchLog, doneLog string,
) error {
	test := t
	if err := m.ensureCharacter(reset.Account, reset.Account, reset.Char,
		test.appendLog); err != nil {
		return fmt.Errorf("ensure character: %w", err)
	}
	time.Sleep(ensurePause)
	if err := m.injectReset(reset, test); err != nil {
		return fmt.Errorf("inject start state: %w", err)
	}
	time.Sleep(ensurePause)

	sessionCtx, cancelSession := context.WithCancel(ctx)
	defer cancelSession()
	sessionDone := make(chan error, 1)
	go func() {
		sessionDone <- m.runSessionSupervised(sessionCtx, reset.Account,
			reset.Account, reset.Char, true, m.proxy, test.appendLog)
	}()

	tracker := m.tracker(test)
	if err := waitOnline(ctx, tracker, test); err != nil {
		cancelSession()
		<-sessionDone

		return err
	}
	test.appendLog(watchLog)

	for {
		if ctx.Err() != nil {
			cancelSession()
			<-sessionDone

			return fmt.Errorf("cancelled: %w", ctx.Err())
		}
		evaluate(tracker, test)
		if allChecksDone(test) {
			test.appendLog(doneLog)
			cancelSession()
			if err := <-sessionDone; err != nil {
				return fmt.Errorf("session end: %w", err)
			}
			test.appendLog("acceptance: the bot left the world gracefully")

			return nil
		}
		select {
		case <-ctx.Done():
			cancelSession()
			<-sessionDone

			return fmt.Errorf("cancelled: %w", ctx.Err())
		case <-time.After(monitorPeriod):
		}
	}
}

// zoneReturnScenario runs the stuck cell round: the temp character
// wakes at the reported freeze position with the reported item set
// and must walk to its selected hunting zone on its own - the freeze
// of the report (the round 58 dump) held a character on that very
// cell forever, the reproduction and the fix live in the hunt package.
func zoneReturnScenario(ctx context.Context, m *Manager, t *Test) error {
	t.setChecks(zoneReturnChecks())

	return runSupervisedScenario(ctx, m, t, zoneReturnReset(returnAccount),
		evaluateZoneReturnConditions,
		"acceptance: the bot is in the world, watching the zone return",
		"acceptance: the bot reached its hunting zone, stopping the bot")
}

// gearGapScenario runs the pantsless dump round: the temp character
// wakes at the reported farm spot of the 2026-09-12 04:58 dump with
// the reported pantsless paperdoll and wallet, and must buy its legs
// armor back through the ordinary shop strategy (the report's bot
// farmed on without it - the gear debt machinery of the round 60 fix
// and its reproduction live in the hunt package).
func gearGapScenario(ctx context.Context, m *Manager, t *Test) error {
	t.setChecks(gearGapChecks())

	return runSupervisedScenario(ctx, m, t, gearGapReset(gearAccount),
		evaluateGearGapConditions,
		"acceptance: the bot is in the world, watching the gear gap refill",
		"acceptance: the legs slot is dressed, stopping the bot")
}

// evaluateZoneReturnConditions rewrites the check list of the stuck
// cell scenario from the live tracker state: the zone check holds
// once the character stands inside the hunting zone its own spot
// economy selected.
func evaluateZoneReturnConditions(tracker *state.Bot, test *Test) {
	if tracker.Status() != state.StatusOnline {
		return
	}
	_, inside, _ := evaluateZone(tracker)
	test.updateCheck(checkZone, inside, "the hunt zone")
}

// evaluateFarmConditions rewrites the farm readiness check list from
// the live tracker state.
func evaluateFarmConditions(
	tracker *state.Bot, test *Test, runStartMs int64,
) {
	if tracker.Status() != state.StatusOnline {
		return
	}
	counts, equipped := evaluateEquipment(tracker)
	test.updateCheck(checkEquipped, equipped,
		fmt.Sprintf("%d armor slots, %d jewels%s", counts.armor(),
			counts.jewels, weaponWord(counts.weapon)))
	skillDetail, skillsDone := evaluateSkills(tracker)
	test.updateCheck(checkSkills, skillsDone, skillDetail)
	_, inside, zone := evaluateZone(tracker)
	test.updateCheck(checkZone, inside, "the hunt zone")
	buffed := tracker.SelfHasBuff(attackAuraSkillID) &&
		tracker.SelfHasBuff(defenseAuraSkillID)
	test.updateCheck(checkBuffs, buffed, auraWord(buffed))
	killDetail, killed := evaluateKill(tracker, zone, runStartMs)
	test.updateCheck(checkKill, killed, killDetail)
}

// weaponWord renders the weapon state of the equipment detail.
func weaponWord(weapon int) string {
	if weapon == 1 {
		return ", weapon in hand"
	}

	return ", no weapon"
}

// auraWord renders the aura state of the buff detail.
func auraWord(buffed bool) string {
	if buffed {
		return "attack aura and defence aura running"
	}

	return "the auras are not running"
}

// allChecksDone reports whether every check of the test holds.
func allChecksDone(test *Test) bool {
	test.mu.Lock()
	defer test.mu.Unlock()
	for i := range test.checks {
		if !test.checks[i].Done {
			return false
		}
	}

	return len(test.checks) > 0
}

// botLifetimeScenario mirrors tools/mobius_e2e.sh in process: a
// fresh level 1 character enters the world, stays online for the
// observation window and shuts down gracefully.
func botLifetimeScenario(ctx context.Context, m *Manager, t *Test) error {
	test := t
	test.setChecks(lifetimeChecks())

	sessionCtx, cancelSession := context.WithCancel(ctx)
	defer cancelSession()
	sessionDone := make(chan error, 1)
	go func() {
		sessionDone <- m.runSession(sessionCtx, lifeAccount, lifePassword,
			lifeAccount, false, m.proxy, test.appendLog)
	}()

	tracker := m.tracker(test)
	if err := waitOnline(ctx, tracker, test); err != nil {
		cancelSession()
		<-sessionDone

		return err
	}
	onlineSince := time.Now()
	test.appendLog("acceptance: watching the " +
		itoa(int(lifeOnlineTime/time.Second)) + "s online window")
	for {
		if ctx.Err() != nil {
			cancelSession()
			<-sessionDone

			return fmt.Errorf("cancelled: %w", ctx.Err())
		}
		if tracker.Status() != state.StatusOnline {
			cancelSession()
			<-sessionDone

			return fmt.Errorf("the session dropped after %s",
				time.Since(onlineSince).Round(time.Second))
		}
		if time.Since(onlineSince) >= lifeOnlineTime {
			break
		}
		select {
		case <-ctx.Done():
			cancelSession()
			<-sessionDone

			return fmt.Errorf("cancelled: %w", ctx.Err())
		case <-time.After(monitorPeriod):
		}
	}
	test.updateCheck("stable", true, "online for "+
		time.Since(onlineSince).Round(time.Second).String())

	// The graceful stop: the context cancel mirrors the SIGINT path
	// of the e2e script (the logout announcement, then the socket
	// close) and a nil session error proves it.
	test.appendLog("acceptance: the online window passed, stopping")
	cancelSession()
	err := <-sessionDone
	if err != nil {
		return fmt.Errorf("session end: %w", err)
	}
	test.updateCheck("graceful", true, "logout announced, no error")
	test.appendLog("acceptance: the session ended gracefully")

	return nil
}

// proxyRelayScenario mirrors tools/proxy_e2e.sh in process: the temp
// bot enters the world behind a dedicated proxy and a fake C1 client
// walks the whole client path - the emulated login, the char list,
// the world entry replay, the live movement relay and the locally
// answered net pings.
func proxyRelayScenario(ctx context.Context, m *Manager, t *Test) error {
	test := t
	test.setChecks(relayChecks())

	server, serverAddr, err := startRelayProxy()
	if err != nil {
		return err
	}
	defer server.Stop()

	sessionCtx, cancelSession := context.WithCancel(ctx)
	defer cancelSession()
	sessionDone := make(chan error, 1)
	go func() {
		sessionDone <- m.runRelaySession(sessionCtx, server, test.appendLog)
	}()

	tracker := m.tracker(test)
	if err := waitOnline(ctx, tracker, test); err != nil {
		cancelSession()
		<-sessionDone

		return err
	}

	client, err := driveRelayClient(ctx, serverAddr, tracker, test)
	if err != nil {
		cancelSession()
		<-sessionDone

		return err
	}
	client.close()

	cancelSession()
	if err := <-sessionDone; err != nil {
		return fmt.Errorf("session end: %w", err)
	}
	test.appendLog("acceptance: the relay client path completed")

	return nil
}
