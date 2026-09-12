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

// zoneReturnScenario runs the stuck cell round: the temp character
// wakes at the reported freeze position with the reported item set
// and must walk to its selected hunting zone on its own - the freeze
// of the report (the round 58 dump) held a character on that very
// cell forever, the reproduction and the fix live in the hunt package.
func zoneReturnScenario(ctx context.Context, m *Manager, t *Test) error {
	test := t
	test.setChecks(zoneReturnChecks())

	if err := m.ensureCharacter(returnAccount, returnPassword, returnAccount,
		test.appendLog); err != nil {
		return fmt.Errorf("ensure character: %w", err)
	}
	time.Sleep(ensurePause)
	if err := m.injectReset(zoneReturnReset(returnAccount), test); err != nil {
		return fmt.Errorf("inject start state: %w", err)
	}
	time.Sleep(ensurePause)

	sessionCtx, cancelSession := context.WithCancel(ctx)
	defer cancelSession()
	sessionDone := make(chan error, 1)
	go func() {
		sessionDone <- m.runSessionSupervised(sessionCtx, returnAccount,
			returnPassword, returnAccount, true, m.proxy, test.appendLog)
	}()

	tracker := m.tracker(test)
	if err := waitOnline(ctx, tracker, test); err != nil {
		cancelSession()
		<-sessionDone

		return err
	}
	test.appendLog("acceptance: the bot is in the world, watching the " +
		"zone return")

	for {
		if ctx.Err() != nil {
			cancelSession()
			<-sessionDone

			return fmt.Errorf("cancelled: %w", ctx.Err())
		}
		evaluateZoneReturnConditions(tracker, test)
		if allChecksDone(test) {
			test.appendLog("acceptance: the bot reached its hunting zone, " +
				"stopping the bot")
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
