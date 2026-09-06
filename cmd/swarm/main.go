// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/melg8/swarm/internal/swarm/connection"
	"github.com/melg8/swarm/internal/swarm/hunt"
	"github.com/melg8/swarm/internal/swarm/pathfind"
	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/melg8/swarm/internal/swarm/webserver"
)

// Default configuration values.
const (
	defaultLoginAddress = "127.0.0.1:2106"
	defaultAccount      = "test1"
	defaultPassword     = "test"
	defaultCharName     = "test1"
	defaultWebAddress   = "127.0.0.1:8080"
	connectTimeout      = 10 * time.Second
)

// Candidate geodata directories of the pathfind test mode, checked in
// order when -geodata is empty: the relative server layout first, then
// the reference Windows deployment of this project (see AGENTS.md).
var defaultGeodataCandidates = []string{
	filepath.Join("data", "geodata"),
	filepath.Join("E:\\", "work", "lineage_workspace_fresh",
		"L2J_Mobius_C1_HarbingersOfWar", "game", "data", "geodata"),
}

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

type config struct {
	loginAddress string
	account      string
	password     string
	charName     string
	webAddress   string
	hunt         bool
	pathfindTest bool
	geodataDir   string
	maxPassable  uint
}

func parseFlags() config {
	cfg := config{
		loginAddress: "",
		account:      "",
		password:     "",
		charName:     "",
		webAddress:   "",
		hunt:         false,
		pathfindTest: false,
		geodataDir:   "",
		maxPassable:  uint(pathfind.DefaultMaxPassableHeight),
	}
	flag.StringVar(&cfg.loginAddress, "login", defaultLoginAddress,
		"login server address")
	flag.StringVar(&cfg.account, "account", defaultAccount, "account name")
	flag.StringVar(&cfg.password, "password", defaultPassword, "password")
	flag.StringVar(&cfg.charName, "char", defaultCharName, "character name")
	flag.StringVar(&cfg.webAddress, "web", defaultWebAddress,
		"web interface address, empty disables it")
	flag.BoolVar(&cfg.hunt, "hunt", false,
		"auto hunt: attack, pick up loot and manage inventory")
	flag.BoolVar(&cfg.pathfindTest, "pathfind-test", false,
		"map pathfinding test UI instead of the bot: no game connection, "+
			"draggable start and end markers show the found path")
	flag.StringVar(&cfg.geodataDir, "geodata", "",
		"geodata directory with X_Y.l2j region files for the pathfind "+
			"test (auto detected when empty)")
	flag.UintVar(&cfg.maxPassable, "max-passable",
		uint(pathfind.DefaultMaxPassableHeight),
		"maximum walkable height difference between neighbouring cells")
	flag.Parse()

	return cfg
}

// connectLoginServer establishes the login server connection.
func connectLoginServer(address string) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", address, connectTimeout)
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
	conn, err := net.DialTimeout("tcp", address, connectTimeout)
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
// bound to a derived context so it stops with the session.
func runBot(ctx context.Context, cfg config, tracker *state.Bot) error {
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

	gameConn, err := connectGameServer(auth)
	if err != nil {
		return err
	}

	game, err := connection.NewGameClient(gameConn)
	if err != nil {
		return fmt.Errorf("game handshake failed: %w", err)
	}
	game.SetTracker(tracker)

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

	//nolint:gosec // slot index is bounded by the character list length
	if err := game.EnterWorld(int32(slot)); err != nil {
		return fmt.Errorf("failed to enter world: %w", err)
	}
	log.Println("Character " + cfg.charName + " entered the world")

	if cfg.hunt {
		loop := hunt.NewLoop(game, tracker)
		loop.SetHuntingZone(hunt.DefaultHuntingZone())
		go loop.Run(sessionCtx)
	}

	return game.Run(sessionCtx, cfg.charName)
}

// runBotForever keeps the bot in the world around the clock: a lost
// session (server restart, kicked connection, network failure) is logged
// and the whole login flow is retried with a growing backoff until the
// user stops the process with SIGINT/SIGTERM. The process never exits on
// its own.
func runBotForever(ctx context.Context, cfg config, tracker *state.Bot) {
	delay := reconnectMinDelay
	for {
		started := time.Now()
		err := runBot(ctx, cfg, tracker)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Println("Bot failed: " + err.Error())
		}
		if time.Since(started) >= stableSessionTime {
			delay = reconnectMinDelay
		}
		log.Printf("Reconnecting in %s", delay)
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = min(delay*2, reconnectMaxDelay)
	}
}

func main() {
	cfg := parseFlags()
	log.SetOutput(os.Stdout)

	if cfg.pathfindTest {
		runPathfindTest(cfg)

		return
	}

	log.Println("Starting swarm bot for account " + cfg.account)

	registry := state.NewRegistry()
	tracker := state.NewBot(cfg.account)
	registry.Add(tracker)

	web := startWebInterface(cfg, registry, nil)

	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)

	runBotForever(ctx, cfg, tracker)
	stop()
	shutdownWebInterface(web)
	log.Println("Bot finished")
}

// runPathfindTest serves the bot less map pathfinding test UI: the geodata
// engine is loaded from the configured or auto detected directory and the
// web interface answers path requests until the process is stopped.
func runPathfindTest(cfg config) {
	dir := cfg.geodataDir
	if dir == "" {
		dir = detectGeodataDir()
	}
	engine := pathfind.NewEngine(dir)
	engine.SetMaxPassableHeight(uint16(cfg.maxPassable))

	stats := engine.Stats()
	if stats.HasData {
		log.Printf("Pathfind test: %d geodata region files in %s",
			stats.RegionFiles, stats.Dir)
	} else {
		log.Println("Pathfind test: no geodata files found in " + stats.Dir +
			", pass -geodata with the game server data/geodata directory")
	}

	web := startWebInterface(cfg, nil, engine)
	if web == nil {
		log.Println("Pathfind test needs the web interface, " +
			"pass a -web address")

		return
	}

	log.Println("Pathfind test UI is ready")

	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	shutdownWebInterface(web)
	log.Println("Pathfind test finished")
}

// detectGeodataDir picks the first candidate directory that exists.
func detectGeodataDir() string {
	for _, candidate := range defaultGeodataCandidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	log.Println("No geodata directory found, pass -geodata explicitly")

	return defaultGeodataCandidates[0]
}

// startWebInterface runs the web server in the background when enabled.
// A non nil pathfind engine switches the server into the pathfind test
// mode, otherwise the bot registry is served.
func startWebInterface(
	cfg config, registry *state.Registry, engine *pathfind.Engine,
) *webserver.Server {
	if cfg.webAddress == "" {
		return nil
	}
	var server *webserver.Server
	if engine != nil {
		// The test opens on the hunting area: that is the terrain the
		// bot actually walks and the most useful pathfind playground.
		zoneX, zoneY, _ := hunt.DefaultHuntingZone()
		server = webserver.NewPathfindServer(engine, cfg.webAddress,
			log.Default(), webserver.PathfindOptions{
				ViewCenter: &pathfind.Vec3{
					X: float64(zoneX),
					Y: float64(zoneY),
					Z: 0,
				},
			})
	} else {
		server = webserver.NewServer(registry, cfg.webAddress, log.Default())
	}
	go func() {
		if err := server.ListenAndServe(); err != nil {
			if !errors.Is(err, http.ErrServerClosed) {
				log.Printf("Web interface stopped: %v", err)
			}
		}
	}()

	return server
}

// shutdownWebInterface gracefully stops the web server.
func shutdownWebInterface(server *webserver.Server) {
	if server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Web interface shutdown failed: %v", err)
	}
}
