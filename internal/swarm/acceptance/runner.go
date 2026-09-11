// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"

	"github.com/melg8/swarm/internal/swarm/connection"
	"github.com/melg8/swarm/internal/swarm/hunt"
	"github.com/melg8/swarm/internal/swarm/proxy"
	"github.com/melg8/swarm/internal/swarm/state"
)

// Character creation constants of the temp elven fighters: the same
// values runBot uses (see cmd/swarm/main.go).
const (
	elfRaceID    = 1
	elfFighterID = 18
	male         = 0
	defaultHair  = 0
	defaultFace  = 0
)

// connectTimeout bounds the login and game dials of a test session.
const connectTimeout = 10 * time.Second

// sessionDialer is the shared connection dialer of the sessions.
//
//nolint:exhaustruct_v5 // the zero defaults are intended
var sessionDialer = &net.Dialer{Timeout: connectTimeout}

// ensurePause separates the character creation connection from the
// test session login: the game server releases the account when the
// connection drops, the pause keeps the second login of the same
// account from racing it.
const ensurePause = 2 * time.Second

// The reconnect backoff of the supervised sessions: the same shape
// the runBotForever supervisor uses (a lost session reconnects after
// the login cooldown, a stable session resets the delay).
const (
	sessionReconnectMinDelay = 2 * time.Second
	sessionReconnectMaxDelay = 30 * time.Second
	sessionStableTime        = time.Minute
)

// ensureCharacter connects to the login and game server, creates the
// temp character when it is missing (a level 1 elven fighter with the
// starter gear, standing at the creation spawn point) and drops the
// connection without entering the world. The database reset that
// follows turns the character into the scenario start state.
func (m *Manager) ensureCharacter(
	account string, password string, char string, logLine func(string),
) error {
	auth, game, err := m.openGame(account, password, m.proxy)
	if err != nil {
		return err
	}
	charList, err := game.Authenticate(gameSessionParams(auth))
	if err != nil {
		_ = game.Close()

		return fmt.Errorf("game authentication: %w", err)
	}
	charList, err = game.EnsureCharacter(connection.CharacterParams{
		Name:      char,
		Race:      elfRaceID,
		Female:    male,
		ClassID:   elfFighterID,
		HairStyle: defaultHair,
		HairColor: defaultHair,
		Face:      defaultFace,
	}, charList)
	if err != nil {
		_ = game.Close()

		return fmt.Errorf("create character: %w", err)
	}
	if _, _, found := charList.FindCharacterByName(char); !found {
		_ = game.Close()

		return fmt.Errorf("character %s missing after creation", char)
	}
	logLine("acceptance: character " + char + " ready on account " + account)

	return game.Close()
}

// gameSessionParams maps the login auth result into the game session
// authentication parameters.
func gameSessionParams(auth *connection.AuthResult,
) connection.GameSessionParams {
	return connection.GameSessionParams{
		Account:    auth.Account,
		LoginOkID1: auth.LoginOkID1,
		LoginOkID2: auth.LoginOkID2,
		PlayOkID1:  auth.PlayOkID1,
		PlayOkID2:  auth.PlayOkID2,
	}
}

// openGame performs the shared connection prologue: the login server
// authentication and the game server handshake of one temp account.
// The registrar is the client proxy the session registers with (nil
// keeps the default proxy of the process, a dedicated server owns the
// relay scenario).
func (m *Manager) openGame(
	account string, password string, registrar *proxy.Server,
) (*connection.AuthResult, *connection.GameClient, error) {
	loginConn, err := sessionDialer.Dial("tcp", m.login)
	if err != nil {
		return nil, nil, fmt.Errorf("login dial: %w", err)
	}
	auth, err := connection.Authenticate(loginConn, account, password)
	if err != nil {
		return nil, nil, fmt.Errorf("authenticate: %w", err)
	}
	if registrar != nil {
		// The emulated login server of the client proxy mirrors the
		// scrambled RSA modulus of the real one.
		registrar.SetRsaModulus(auth.RsaPublicKey)
	}
	gameConn, err := sessionDialer.Dial("tcp", gameAddress(auth))
	if err != nil {
		return nil, nil, fmt.Errorf("game dial: %w", err)
	}
	game, err := connection.NewGameClient(gameConn)
	if err != nil {
		return nil, nil, fmt.Errorf("game handshake: %w", err)
	}
	game.SetLogger(m.logger)

	return auth, game, nil
}

// gameAddress renders the game server endpoint of the auth result.
func gameAddress(auth *connection.AuthResult) string {
	return fmt.Sprintf("%d.%d.%d.%d:%d",
		auth.ServerIP[0], auth.ServerIP[1], auth.ServerIP[2], auth.ServerIP[3],
		auth.ServerPort)
}

// runSession plays the temp character until the context ends: the
// full runBot wiring (login, handshake, authentication, the character
// selection, the world entry, the hunt loop with the geodata
// navigator, the proxy registration) squeezed into the acceptance
// runner. The autonomous flag mirrors -hunt of the command line: the
// farm scenarios hunt on their own, the lifetime and proxy scenarios
// stay in the manual only mode.
func (m *Manager) runSession(
	ctx context.Context, account string, password string, char string,
	autonomous bool, registrar *proxy.Server, logLine func(string),
) error {
	tracker := m.registryTracker(account)
	tracker.ResetSession()

	auth, game, err := m.openGame(account, password, registrar)
	if err != nil {
		return err
	}
	game.SetTracker(tracker)

	// The proxy observes the whole session (the recorder replays it
	// to connecting C1 clients) so the user can attach a real client
	// to the running test bot exactly like to any fleet bot.
	if registrar != nil {
		sessionRecorder := registrar.RegisterSession(account, game, tracker)
		game.SetTap(sessionRecorder.Record)
		defer registrar.UnregisterSession(account, sessionRecorder)
	}

	charList, err := game.Authenticate(gameSessionParams(auth))
	if err != nil {
		return fmt.Errorf("game authentication: %w", err)
	}

	// The temp character must exist before the selection: a fresh
	// database starts every scenario with an empty account, so the
	// session creates the missing elven fighter exactly like runBot
	// does (the same creation values ensureCharacter uses).
	charList, err = game.EnsureCharacter(connection.CharacterParams{
		Name:      char,
		Race:      elfRaceID,
		Female:    male,
		ClassID:   elfFighterID,
		HairStyle: defaultHair,
		HairColor: defaultHair,
		Face:      defaultFace,
	}, charList)
	if err != nil {
		return fmt.Errorf("prepare character: %w", err)
	}

	slot, charInfo, found := charList.FindCharacterByName(char)
	if !found {
		return fmt.Errorf("character %s not found", char)
	}
	logLine(fmt.Sprintf("acceptance: playing %s, level %d",
		charInfo.Name, charInfo.Level))

	if err := game.EnterWorld(int32(slot)); err != nil {
		return fmt.Errorf("enter world: %w", err)
	}
	logLine("acceptance: entered the world")

	// The hunt loop always runs: with autonomy it hunts, shops and
	// learns on its own; without it executes the manual commands only
	// (the web UI stays interactive either way).
	loop := hunt.NewLoop(game, tracker)
	loop.SetLogger(sessionLogger(tracker, logLine))
	if m.engine != nil {
		loop.SetNavigator(hunt.NewNavigator(m.engine))
	}
	if autonomous {
		loop.SetHuntingZoneRegion("elven")
	} else {
		loop.SetAutonomy(false)
	}
	go loop.Run(ctx)

	return game.Run(ctx, char)
}

// runSessionSupervised keeps the temp session alive the way the
// runBotForever supervisor does: a lost session (the emergency logout
// of the hunt loop, a server kick, a transport error) reconnects
// after the tracker login cooldown with a growing backoff, and the
// run context ends the loop. The return mirrors a single session: nil
// means the run context ended while the session was healthy.
func (m *Manager) runSessionSupervised(
	ctx context.Context, account string, password string, char string,
	autonomous bool, registrar *proxy.Server, logLine func(string),
) error {
	delay := sessionReconnectMinDelay
	for {
		started := time.Now()
		err := m.runSession(ctx, account, password, char, autonomous,
			registrar, logLine)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			logLine("acceptance: session lost, reconnecting: " + err.Error())
		}
		tracker := m.registryTracker(account)
		if cooldown := tracker.LoginCooldownRemaining(); cooldown > delay {
			logLine("acceptance: the login cooldown holds the reconnect " +
				"back for " + cooldown.String())
			delay = cooldown
		}
		if time.Since(started) >= sessionStableTime {
			delay = sessionReconnectMinDelay
		}
		logLine("acceptance: reconnecting in " + delay.String())
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}
		delay = min(delay*2, sessionReconnectMaxDelay)
	}
}

// registryTracker resolves the tracker of a temp account.
func (m *Manager) registryTracker(account string) *state.Bot {
	if bot, ok := m.registry.Get(account); ok {
		return bot
	}

	return state.NewBot(account)
}

// sessionLogger builds the hunt loop logger: the console copy and the
// mirrored lines of the test log. The hunt decisions explain what the
// scenario is doing (the shopping trips, the lessons, the zone
// switches) exactly like the fleet sessions.
func sessionLogger(
	tracker *state.Bot, logLine func(string),
) *log.Logger {
	return log.New(io.MultiWriter(os.Stdout, logMirror{
		tracker: tracker,
		logLine: logLine,
	}), "", log.LstdFlags)
}

// logMirror forwards the hunt log lines into the tracker event log
// and the test log (without the console timestamp prefix).
type logMirror struct {
	tracker *state.Bot
	logLine func(string)
}

// Write implements io.Writer for the log package.
func (m logMirror) Write(p []byte) (int, error) {
	line := trimLogSpace(string(p))
	if line == "" {
		return len(p), nil
	}
	m.tracker.RecordEvent(line)
	m.logLine(line)

	return len(p), nil
}

// trimLogSpace strips the line ends of a console log line.
func trimLogSpace(value string) string {
	start := 0
	for start < len(value) && isLogSpace(value[start]) {
		start++
	}
	end := len(value)
	for end > start && isLogSpace(value[end-1]) {
		end--
	}

	return value[start:end]
}

// isLogSpace reports a latin space character.
func isLogSpace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}
