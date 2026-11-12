// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "context"
    "flag"
    "fmt"
    "io"
    "log"
    "net"
    "os"
    "strconv"
    "strings"
    "sync"
    "time"

    "github.com/melg8/swarm/internal/swarm/connection"
    "github.com/melg8/swarm/internal/swarm/gear"
    "github.com/melg8/swarm/internal/swarm/hunt"
    "github.com/melg8/swarm/internal/swarm/memwatch"
    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
    "github.com/melg8/swarm/internal/swarm/proxy"
    "github.com/melg8/swarm/internal/swarm/session"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/melg8/swarm/internal/version"
)

// Reconnect backoff of the 24/7 supervisor: a lost session is retried
// with a growing pause, a long lived session resets the pause so a drop
// after hours reconnects immediately.
const (
    reconnectMinDelay = 2 * time.Second
    reconnectMaxDelay = 30 * time.Second
    // A session shorter than this counts as a failed attempt and grows
    // the backoff; a longer one resets it.
    stableSessionTime = time.Minute
)

// Character creation constants for the elven fighter.
const (
    elfRaceID     = 1
    elfFighterID  = 18
    male          = 0
    defaultHair   = 0
    defaultFace   = 0
    characterSlot = 0
)

// swarmDialer dials the login and game server connections with the
// connect timeout.
// Listing the deprecated Dialer fields explicitly would trip
// staticcheck SA1019, so the struct stays partial.
//
//nolint:exhaustruct_v5 // net.Dialer carries deprecated fields (see above)
var swarmDialer = &net.Dialer{Timeout: connectTimeout}

// connectLoginServer establishes the login server connection.
func connectLoginServer(address string) (net.Conn, error) {
    conn, err := swarmDialer.Dial("tcp", address)
    if err != nil {
        return nil, fmt.Errorf(
            "failed to connect to login server: %w", err)
    }
    log.Println("Connected to login server at " + address)

    return conn, nil
}

// connectGameServer establishes the game server connection from the auth
// result.
func connectGameServer(auth *connection.AuthResult) (net.Conn, error) {
    address := fmt.Sprintf("%d.%d.%d.%d:%d",
        auth.ServerIP[0], auth.ServerIP[1], auth.ServerIP[2], auth.ServerIP[3],
        auth.ServerPort)
    conn, err := swarmDialer.Dial("tcp", address)
    if err != nil {
        return nil, fmt.Errorf(
            "failed to connect to game server: %w", err)
    }
    log.Println("Connected to game server at " + address)

    return conn, nil
}

// runBot performs one bot session: login, game handshake, authentication,
// character creation, entering the world and staying in it until the
// context is done or the session fails. The hunt loop of the session is
// bound to a derived context so it stops with the session. The optional
// geodata engine serves the town trips of the hunt loop.
func runBot( //nolint:funlen // linear session script
    ctx context.Context, cfg config, tracker *state.Bot,
    engine *pathfind.Engine, mesh *navmesh.Mesh,
    proxyServer *proxy.Server, journal *session.Journal,
    journalWG *sync.WaitGroup,
) error {
    sessionCtx, cancelSession := context.WithCancel(ctx)
    defer cancelSession()

    tracker.ResetSession()

    loginConn, err := connectLoginServer(cfg.loginAddress)
    if err != nil {
        return err
    }

    auth, err := connection.Authenticate(loginConn, cfg.account, cfg.password)
    if err != nil {
        return fmt.Errorf("failed to authenticate: %w", err)
    }

    // The emulated login server of the proxy mirrors the Init packet of
    // the real one, so its scrambled RSA modulus is published here.
    if proxyServer != nil {
        proxyServer.SetRsaModulus(auth.RsaPublicKey)
    }

    gameConn, err := connectGameServer(auth)
    if err != nil {
        return err
    }

    game, err := connection.NewGameClient(gameConn)
    if err != nil {
        // NewGameClient owns the connection only on success: the
        // handshake failure must not leak it (the supervisor retries
        // around the clock and every leaked fd and server account
        // slot compounds across the retries).
        _ = gameConn.Close()

        return fmt.Errorf("game handshake failed: %w", err)
    }
    // Every later pre-Run failure (game authentication, character
    // preparation, world entry) closes the session through this
    // defer; on the happy path Run already disconnected and the
    // second close is a no-op.
    defer func() { _ = game.Close() }()
    game.SetTracker(tracker)

    // The proxy observes the whole session (the recorder replays it to
    // connecting C1 clients) and forwards their packets through the
    // shared outbound cipher of the session. The registration must
    // happen before the session starts reading so nothing is missed.
    var sessionRecorder *proxy.Recorder
    if proxyServer != nil {
        sessionRecorder = proxyServer.RegisterSession(
            cfg.account, game, tracker)
        game.SetTap(sessionRecorder.Record)
        defer proxyServer.UnregisterSession(cfg.account, sessionRecorder)
    }

    charList, err := game.Authenticate(connection.GameSessionParams{
        Account:    auth.Account,
        LoginOkID1: auth.LoginOkID1,
        LoginOkID2: auth.LoginOkID2,
        PlayOkID1:  auth.PlayOkID1,
        PlayOkID2:  auth.PlayOkID2,
    })
    if err != nil {
        return fmt.Errorf("game authentication failed: %w", err)
    }

    charList, err = game.EnsureCharacter(connection.CharacterParams{
        Name:      cfg.charName,
        Race:      elfRaceID,
        Female:    male,
        ClassID:   elfFighterID,
        HairStyle: defaultHair,
        HairColor: defaultHair,
        Face:      defaultFace,
    }, charList)
    if err != nil {
        return fmt.Errorf("failed to prepare character: %w", err)
    }

    slot, charInfo, found := charList.FindCharacterByName(cfg.charName)
    if !found {
        return fmt.Errorf("character %s not found", cfg.charName)
    }
    log.Printf("Playing character %s of level %d",
        charInfo.Name, charInfo.Level)

    if err := game.EnterWorld(int32(slot)); err != nil {
        return fmt.Errorf("failed to enter world: %w", err)
    }
    log.Println("Character " + cfg.charName + " entered the world")
    if journal != nil {
        journal.Connect(cfg.account, "entered",
            "level "+strconv.Itoa(int(charInfo.Level)))
    }

    // The loop always runs: with -hunt it hunts autonomously, without
    // it stays in the manual mode and only executes the commands of the
    // web UI (map clicks, equipment drags) so the interface stays
    // interactive in both launch modes.
    loop := hunt.NewLoop(game, tracker)
    // The launch bot type seeds the gear profile: an archer typed slot
    // scores the ranged gear (the bow as the weapon milestone, the
    // quiver as the ammo, issue #17) from its very first equip
    // decision. The fighter and every mode run without a type keep
    // the melee fighter default of the loop, and the class based pick
    // of the loop may still override a caster class later (the
    // physical class wins over the label - a mystic cannot shoot the
    // bow the label pretends).
    if cfg.botType == botTypeArcher {
        loop.SetGearProfile(gear.Archer{})
    }
    // The per-plan kite override lands after the gear profile: an
    // archer spec with a kite section runs the named knobs (the
    // baseline profile disables the layer whole, the tuned profile
    // edits the numbers - owner issue #29), every other spec keeps
    // the shipped tuning of DefaultKiteParams.
    if cfg.kite != nil {
        loop.SetKiteParams(cfg.kite.toParams())
    }
    // Every session start IS a relogin into the same world spot: the
    // spawn protection settle holds the character there while it
    // regenerates and opens the first fight deliberately (see
    // hunt/settle.go and the verified H-005 server facts).
    loop.EnableSpawnSettle()
    // The hunt decisions mirror into the tracker event log: the web UI
    // log tab and the state dump button then carry the reasoning of
    // the loop (zone switches, escapes, stuck re-paths) next to the
    // raw game events - the debugging material of the live sessions.
    loop.SetLogger(huntEventLogger(tracker))
    loop.SetJournal(journal)
    installNavigator(loop, engine, mesh, cfg.hunt)
    if cfg.hunt {
        loop.SetHuntingZoneRegion("elven")
    } else {
        loop.SetAutonomy(false)
    }
    journalWG.Add(1)
    go func() {
        defer journalWG.Done()
        loop.Run(sessionCtx)
    }()

    return game.Run(sessionCtx, cfg.charName)
}

// installNavigator binds the path planning behind the Navigator
// seam of the hunt loop: the navmesh hybrid when the tile mesh
// exists (the live integration round of docs/navmesh.md - the long
// routes serve from the mesh corridor search, the grid engine stays
// the click validation and the fallback authority), the pure grid
// engine navigator otherwise. Without geodata the loop hunts without
// the town trips at all.
func installNavigator(
    loop *hunt.Loop, engine *pathfind.Engine, mesh *navmesh.Mesh,
    huntMode bool,
) {
    switch {
    case engine != nil && mesh != nil:
        loop.SetNavigator(hunt.NewNavmeshNavigator(engine, mesh))
    case engine != nil:
        loop.SetNavigator(hunt.NewNavigator(engine))
    case huntMode:
        log.Println("Hunt runs without town trips: no geodata available")
    }
}

// huntEventLogger builds the logger of the hunt loop: the console copy
// keeps the standard format, and every hunt decision line mirrors into
// the tracker event log (stripped of the timestamp prefix the console
// format adds). The mirrored lines then stream to the web UI log tab
// and travel inside the state dump.
func huntEventLogger(tracker *state.Bot) *log.Logger {
    mirror := huntEventMirror{tracker: tracker}

    return log.New(io.MultiWriter(os.Stdout, mirror), "", log.LstdFlags)
}

// openSessionJournal opens the persistent session journal when the
// mode is on (-session-dir defaults to "logs", the empty value
// disables it). The journal carries the process identity line and
// logs its path so the user knows what to attach to a report.
func openSessionJournal(cfg config) *session.Journal {
    if cfg.sessionDir == "" {
        return nil
    }
    journal, err := session.NewJournal(cfg.sessionDir, log.Default())
    if err != nil {
        log.Printf("Session journal unavailable (continuing without): %v",
            err)

        return nil
    }
    journal.Build(version.Identity())
    log.Printf("Session journal: %s", journal.Path())

    return journal
}

// wireSessionBot connects one tracker to the journal: the event story
// mirror (every recorded tracker event lands in the journal file) and
// the periodic state sampler of the quantitative trail. The sampler
// goroutine joins the wait group so the shutdown waits for its final
// records before the journal closes.
func wireSessionBot(
    ctx context.Context, journal *session.Journal, tracker *state.Bot,
    journalWG *sync.WaitGroup,
) {
    if journal == nil {
        return
    }
    botID := tracker.ID()
    tracker.SetEventSink(func(at time.Time, message string) {
        journal.Story(botID, message, at)
    })
    journalWG.Add(1)
    go func() {
        defer journalWG.Done()
        session.NewSampler(botID, tracker, journal).Run(ctx)
    }()
}

// explicitAccount reports whether the -account flag was passed on the
// command line: the offline journal tools treat the bot filter as
// opt-in, so the default login account of the bot modes never filters
// a journal whose bots carry other names.
func explicitAccount() bool {
    set := false
    flag.Visit(func(f *flag.Flag) {
        if f.Name == "account" {
            set = true
        }
    })

    return set
}

// huntEventMirror writes hunt log lines into the bot event log.
type huntEventMirror struct {
    tracker *state.Bot
}

// Write implements io.Writer for the log package: one call carries one
// complete line.
func (m huntEventMirror) Write(p []byte) (int, error) {
    line := strings.TrimSpace(string(p))
    // The console prefix (date time) stays console only: the event log
    // carries its own timestamps.
    if at := strings.Index(line, "Hunt: "); at >= 0 {
        line = line[at:]
    }
    if line != "" {
        m.tracker.RecordEvent(line)
    }

    return len(p), nil
}

// startMemoryWatch runs the process memory logger (one footprint
// line a minute, see internal/swarm/memwatch) until the context ends:
// the long unattended runs demonstrate the memory behavior from the
// log alone, not only from the web statistics chart.
func startMemoryWatch(ctx context.Context) {
    go memwatch.Watch(ctx, log.Default(), memwatch.DefaultPeriod)
}

// runBotForever keeps the bot in the world around the clock: a lost
// session (server restart, kicked connection, network failure) is logged
// and the whole login flow is retried with a growing backoff until the
// user stops the process with SIGINT/SIGTERM. The process never exits on
// its own. The geodata engine and the navmesh mesh survive the
// reconnects.
func runBotForever(
    ctx context.Context, cfg config, tracker *state.Bot,
    engine *pathfind.Engine,
    mesh *navmesh.Mesh, proxyServer *proxy.Server, journal *session.Journal,
    journalWG *sync.WaitGroup,
) {
    delay := reconnectMinDelay
    for {
        started := time.Now()
        err := runBot(ctx, cfg, tracker, engine, mesh, proxyServer,
            journal, journalWG)
        if ctx.Err() != nil {
            return
        }
        if err != nil {
            // An emergency logout closes the socket itself, so
            // the session error names the close ("use of closed
            // network connection") instead of the pile up that
            // drove it: the tracker carries the honest reason
            // over the boundary and the lost record names it
            // (once - later network drops of the same
            // supervisor report themselves).
            reason := err.Error()
            if honest, ok := tracker.ConsumeEmergencyLogout(); ok {
                reason = "emergency logout: " + honest
            }
            log.Println("Bot failed: " + reason)
            if journal != nil {
                journal.Lost(cfg.account, reason)
            }
        }
        // An emergency logout of the hunt loop armed a login
        // cooldown: honor it on top of the reconnect backoff so the
        // next session starts after the danger window (the mobs
        // reset, the character regenerates) instead of the seconds
        // of the backoff.
        if cooldown := tracker.LoginCooldownRemaining(); cooldown > delay {
            log.Printf("Login cooldown %s holds the reconnect back",
                cooldown)
            delay = cooldown
        }
        if time.Since(started) >= stableSessionTime {
            delay = reconnectMinDelay
        }
        log.Printf("Reconnecting in %s", delay)
        if journal != nil {
            journal.Connect(cfg.account, "reconnect-wait",
                delay.String())
        }
        select {
        case <-ctx.Done():
            return
        case <-time.After(delay):
        }
        delay = min(delay*2, reconnectMaxDelay)
    }
}

// runNavmeshViewer serves the bot less 3D navmesh viewer (the
// -show-navmesh mode): the mesh tiles of the configured or auto
// detected directory render in the browser, a double click pair runs
// the real corridor search with its construction timer, and the
// process keeps serving until it is stopped. The flag selection
// bounds the initially visible tiles (every tile when empty), the
// route queries always run over the full directory mesh.
// newBotEngine builds the shared geodata engine of the bot modes:
// the town trips, the manual long walks and the hybrid navigator of
// every session read through one LRU cache of parsed regions, with
// the capsule clearance armed (docs/pathfinding.md).
func newBotEngine(cfg config) *pathfind.Engine {
    dir := cfg.geodataDir
    if dir == "" {
        dir = detectGeodataDir()
    }
    engine := pathfind.NewEngine(dir)
    engine.SetMaxPassableHeight(uint16(cfg.maxPassable))
    // The capsule clearance keeps every planned waypoint and segment
    // from the walls: the server movement validation is cell level and
    // never checks the character capsule (the elven fighter template
    // radius 7.5), so the planner owns the clearance
    // (docs/pathfinding.md).
    engine.SetCapsuleClearance(pathfind.DefaultCollisionRadius)

    return engine
}
