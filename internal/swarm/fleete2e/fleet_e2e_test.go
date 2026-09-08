// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Package fleete2e benchmarks the real 100 bot fleet against the
// local Mobius C1 stack: a hundred live sessions (login, game
// handshake, elven fighter in the world, autonomous hunt loop) keep
// receiving real game packets while the benchmarks sweep the state
// layer the web view and the hunt ticks pay for. The suite needs the
// deployed stack (tools/swarm_fast_deploy.sh, ports 2106/7777/3306)
// and the explicit opt in - the sessions, the account creation and
// the packet load are far too slow for the regular test runs:
//
//	SWARM_FLEET_E2E=1 go test ./internal/swarm/fleete2e/ \
//	    -bench . -benchtime 30x -timeout 30m
package fleete2e

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/connection"
	"github.com/melg8/swarm/internal/swarm/hunt"
	"github.com/melg8/swarm/internal/swarm/state"
)

// fleetSize is the stretch goal of AGENTS.md: one hundred concurrent
// bot sessions against the live server.
const fleetSize = 100

// loginAddress is the local Mobius login server of the deployed
// stack.
const loginAddress = "127.0.0.1:2106"

// defaultPassword matches the account convention of the E2E script
// (the server auto-creates missing accounts).
const defaultPassword = "test"

// Session launch constants of the elven fighter (the same recipe the
// cmd/swarm supervisor uses).
const (
	elfRaceID     = 1
	elfFighterID  = 18
	elfFemale     = 0
	defaultHair   = 0
	defaultFace   = 0
	connectLimit  = 10 * time.Second
	onlineTimeout = 10 * time.Minute
)

// fleet is the lazily launched live fleet shared by the benchmarks:
// the sessions stay connected until the test binary exits.
var fleet struct {
	once   sync.Once
	bots   []*state.Bot
	online int
	err    error
}

// requireFleet returns the live bot trackers, launching the fleet on
// the first call. Every benchmark of the suite shares the sessions so
// the setup cost is paid once.
func requireFleet(b *testing.B) []*state.Bot {
	b.Helper()
	if os.Getenv("SWARM_FLEET_E2E") != "1" {
		b.Skip("set SWARM_FLEET_E2E=1 and deploy the stack with " +
			"tools/swarm_fast_deploy.sh to run the live fleet benchmark")
	}
	fleet.once.Do(launchFleet)
	if fleet.err != nil {
		b.Fatalf("fleet launch failed: %v", fleet.err)
	}

	return fleet.bots
}

// launchFleet connects the whole fleet: one hundred accounts, each
// with its elven fighter entering the world, its autonomous hunt
// loop and the 24/7 reconnect supervisor of the cmd/swarm binary
// (the emergency logout of the crowded starting area cycles the
// sessions through their login cooldowns - the steady fleet, not a
// one shot connection). The launches run in waves so the login
// server and the account creation never queue behind a thundering
// herd. The launch returns once the fleet reached its steady state:
// the majority of the sessions online at the same time.
func launchFleet() {
	logger := log.New(os.Stderr, "[fleete2e] ", log.LstdFlags)
	//nolint:gosec // intentional: the fleet sessions live until the
	// test binary exits, the cancel is dropped on purpose
	ctx, cancel := context.WithCancel(context.Background())
	_ = cancel

	fleet.bots = make([]*state.Bot, fleetSize)
	for i := range fleetSize {
		account := fmt.Sprintf("fleet%03d", i+1)
		tracker := state.NewBot(account)
		fleet.bots[i] = tracker
		go func() {
			runFleetForever(ctx, tracker, account, logger)
		}()
		if (i+1)%fleetWave == 0 {
			// Give the wave time to pass the login and the character
			// creation before the next one dials in.
			time.Sleep(fleetWavePause)
		}
	}
	steadyDeadline := time.After(onlineTimeout)
	for {
		fleet.online = countOnline(fleet.bots)
		if fleet.online >= fleetSteadyOnline {
			logger.Printf("fleet steady state: %d sessions online",
				fleet.online)

			return
		}
		select {
		case <-steadyDeadline:
			fleet.err = fmt.Errorf(
				"only %d of %d sessions online after %s",
				fleet.online, fleetSize, onlineTimeout)

			return
		default:
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// fleetSteadyOnline is the steady state threshold of the launch:
// the fraction of sessions the crowded starting area keeps online
// at once (the emergency logout cooldowns of the losing fights cycle
// the rest through their reconnects).
const fleetSteadyOnline = fleetSize * 3 / 5

// fleetWave and fleetWavePause pace the logins: ten sessions dial
// in, then a pause lets the login server and the character creation
// finish before the next wave.
const (
	fleetWave      = 10
	fleetWavePause = 2 * time.Second
)

// reconnectBackoff bounds the retry pause of a dropped session (the
// same values the cmd/swarm supervisor uses).
const (
	reconnectMinDelay = 2 * time.Second
	reconnectMaxDelay = 30 * time.Second
)

// countOnline returns how many trackers report the online status.
func countOnline(bots []*state.Bot) int {
	online := 0
	for _, bot := range bots {
		if bot.Status() == state.StatusOnline {
			online++
		}
	}

	return online
}

// runFleetForever keeps the session alive around the clock like the
// runBotForever supervisor of cmd/swarm: a lost session (a death
// pile up, the emergency logout, a server hiccup) is retried with a
// growing backoff, and the login cooldown of the emergency logout
// holds the reconnect back until the danger window passed.
func runFleetForever(
	ctx context.Context, tracker *state.Bot, account string,
	logger *log.Logger,
) {
	delay := reconnectMinDelay
	for {
		started := time.Now()
		err := runFleetSession(ctx, tracker, account, logger)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			logger.Printf("session %s failed: %v", account, err)
		}
		if cooldown := tracker.LoginCooldownRemaining(); cooldown > delay {
			delay = cooldown
		}
		if time.Since(started) >= stableSessionTime {
			delay = reconnectMinDelay
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = min(delay*2, reconnectMaxDelay)
	}
}

// stableSessionTime marks a healthy session: a connection that held
// this long resets the reconnect backoff (the value of the cmd/swarm
// supervisor).
const stableSessionTime = time.Minute

// runFleetSession performs one bot session against the live stack:
// login, game handshake, elven fighter creation, entering the world
// and the autonomous hunt loop, exactly like the cmd/swarm supervisor
// (runBot) without the web interface and the geodata.
func runFleetSession(
	ctx context.Context, tracker *state.Bot, account string,
	logger *log.Logger,
) error {
	tracker.ResetSession()

	loginConn, err := net.DialTimeout("tcp", loginAddress, connectLimit)
	if err != nil {
		return fmt.Errorf("login dial: %w", err)
	}
	auth, err := connection.Authenticate(loginConn, account, defaultPassword)
	if err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}

	gameAddress := fmt.Sprintf("%d.%d.%d.%d:%d",
		auth.ServerIP[0], auth.ServerIP[1], auth.ServerIP[2], auth.ServerIP[3],
		auth.ServerPort)
	gameConn, err := net.DialTimeout("tcp", gameAddress, connectLimit)
	if err != nil {
		return fmt.Errorf("game dial: %w", err)
	}

	game, err := connection.NewGameClient(gameConn)
	if err != nil {
		return fmt.Errorf("game handshake: %w", err)
	}
	game.SetTracker(tracker)
	game.SetLogger(logger)

	charList, err := game.Authenticate(connection.GameSessionParams{
		Account:    auth.Account,
		LoginOkID1: auth.LoginOkID1,
		LoginOkID2: auth.LoginOkID2,
		PlayOkID1:  auth.PlayOkID1,
		PlayOkID2:  auth.PlayOkID2,
	})
	if err != nil {
		return fmt.Errorf("game auth: %w", err)
	}

	charList, err = game.EnsureCharacter(connection.CharacterParams{
		Name:      account,
		Race:      elfRaceID,
		Female:    elfFemale,
		ClassID:   elfFighterID,
		HairStyle: defaultHair,
		HairColor: defaultHair,
		Face:      defaultFace,
	}, charList)
	if err != nil {
		return fmt.Errorf("ensure character: %w", err)
	}

	slot, _, found := charList.FindCharacterByName(account)
	if !found {
		return fmt.Errorf("character %s not found", account)
	}
	if err := game.EnterWorld(int32(slot)); err != nil {
		return fmt.Errorf("enter world: %w", err)
	}

	loop := hunt.NewLoop(game, tracker)
	loop.SetHuntingZoneRegion("elven")
	go loop.Run(ctx)

	return game.Run(ctx, account)
}

// BenchmarkFleetE2ELiveEncodeSweep measures the aggregate snapshot
// encode of the whole live fleet: one sweep is the worst case of the
// web view watching every bot (the SSE version-change poll of a
// hundred streams firing together), with the game sessions mutating
// the trackers from real packet traffic at the same time.
func BenchmarkFleetE2ELiveEncodeSweep(b *testing.B) {
	bots := requireFleet(b)
	payload := make([]byte, 0, 128<<10)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for _, bot := range bots {
			payload = bot.AppendSnapshotJSON(payload[:0])
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(len(bots)), "bots/sweep")
}

// BenchmarkFleetE2EEngageScanSweep measures the aggregate hunt tick
// scan of the fleet: the constrained target search over every live
// world (a hundred search calls per iteration, the shape of a fleet
// wide engage tick), with the real packet apply paths contending for
// the same trackers.
func BenchmarkFleetE2EEngageScanSweep(b *testing.B) {
	bots := requireFleet(b)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for _, bot := range bots {
			bot.NearestAttackableConstrained(1500, nil, nil, 0, true)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(len(bots)), "bots/sweep")
}

// TestFleetE2EPacketRate samples the real packet throughput of the
// fleet over a 30 second window: the game sessions receive their
// broadcast traffic while the trackers mutate. The logged rate is the
// aggregate input load the state layer of a hundred hunting sessions
// actually digests.
func TestFleetE2EPacketRate(t *testing.T) {
	if os.Getenv("SWARM_FLEET_E2E") != "1" {
		t.Skip("set SWARM_FLEET_E2E=1 and deploy the stack with " +
			"tools/swarm_fast_deploy.sh to run the live fleet test")
	}
	fleet.once.Do(launchFleet)
	if fleet.err != nil {
		t.Fatalf("fleet launch failed: %v", fleet.err)
	}
	const window = 30 * time.Second
	before := fleetPacketCount(fleet.bots)
	time.Sleep(window)
	after := fleetPacketCount(fleet.bots)
	rate := float64(after-before) / window.Seconds()
	t.Logf("fleet packet rate: %.0f packets/s (%d online of %d)",
		rate, countOnline(fleet.bots), fleetSize)
	if rate <= 0 {
		t.Fatalf("fleet receives no packets: rate %.1f", rate)
	}
}

// fleetPacketCount sums the received packet counters of the fleet.
func fleetPacketCount(bots []*state.Bot) int64 {
	total := int64(0)
	for _, bot := range bots {
		total += bot.Packets()
	}

	return total
}

// TestFleetE2ESessionHealth pins the fixture itself: with the opt in
// and the stack up, the fleet reaches its steady state, receives
// packets and keeps hunting (the version counters of the trackers
// advance, the emergency logout cooldowns cycle the rest).
func TestFleetE2ESessionHealth(t *testing.T) {
	if os.Getenv("SWARM_FLEET_E2E") != "1" {
		t.Skip("set SWARM_FLEET_E2E=1 and deploy the stack with " +
			"tools/swarm_fast_deploy.sh to run the live fleet test")
	}
	fleet.once.Do(launchFleet)
	if fleet.err != nil {
		t.Fatalf("fleet launch failed: %v", fleet.err)
	}
	if len(fleet.bots) != fleetSize {
		t.Fatalf("fleet size: %d", len(fleet.bots))
	}
	online := countOnline(fleet.bots)
	if online < fleetSteadyOnline {
		// The fleet cycles through the emergency logout cooldowns of
		// the crowded starting area: the online count breathes
		// around the steady threshold, so a single low sample gets
		// one settling window before the verdict.
		time.Sleep(10 * time.Second)
		online = countOnline(fleet.bots)
		if online < fleetSteadyOnline {
			t.Fatalf("only %d of %d sessions online (steady state %d)",
				online, fleetSize, fleetSteadyOnline)
		}
	}
	if !huntingAdvances(fleet.bots) {
		t.Fatal("no online tracker advances its version: " +
			"the fleet stopped receiving packets")
	}
	t.Logf("fleet healthy: %d/%d online, packets %d",
		online, fleetSize, fleetPacketCount(fleet.bots))
}

// huntingAdvances reports whether at least one online tracker keeps
// receiving packets: its version counter moves over a three second
// window. An offline tracker freezes (the emergency logout holds its
// login cooldown), so the check polls the online ones only.
func huntingAdvances(bots []*state.Bot) bool {
	type sample struct {
		bot     *state.Bot
		version uint64
	}
	samples := make([]sample, 0, len(bots))
	for _, bot := range bots {
		if bot.Status() == state.StatusOnline {
			samples = append(samples, sample{bot: bot, version: bot.Version()})
		}
	}
	if len(samples) == 0 {
		return false
	}
	time.Sleep(3 * time.Second)
	for _, s := range samples {
		if s.bot.Status() == state.StatusOnline &&
			s.bot.Version() > s.version {
			return true
		}
	}

	return false
}
