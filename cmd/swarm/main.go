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
	"strings"
	"syscall"
	"time"

	"github.com/melg8/swarm/internal/swarm/connection"
	"github.com/melg8/swarm/internal/swarm/hunt"
	"github.com/melg8/swarm/internal/swarm/pathfind"
	"github.com/melg8/swarm/internal/swarm/proxy"
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
	defaultProxyLogPath = "proxy.log"
)

// Candidate geodata directories, checked in order when -geodata is empty:
// the relative server layouts first (the sandbox checkout next to the swarm
// root keeps the server tree at l2j_mobius/L2J_Mobius_C1_HarbingersOfWar,
// the reference Windows deployment runs the server straight from its
// L2J_Mobius_C1_HarbingersOfWar/game folder, see AGENTS.md), then the bot
// tree itself.
var defaultGeodataCandidates = []string{
	filepath.Join("data", "geodata"),
	filepath.Join("..", "l2j_mobius", "L2J_Mobius_C1_HarbingersOfWar",
		"dist", "game", "data", "geodata"),
	filepath.Join("L2J_Mobius_C1_HarbingersOfWar", "game", "data", "geodata"),
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
	loginAddress  string
	account       string
	password      string
	charName      string
	webAddress    string
	hunt          bool
	pathfindTest  bool
	testFightUI   bool
	testFightUIV1 bool
	geodataDir    string
	maxPassable   uint
	proxy         bool
	proxyLogin    string
	proxyGame     string
	proxyLog      string
}

func parseFlags() config {
	cfg := config{
		loginAddress:  "",
		account:       "",
		password:      "",
		charName:      "",
		webAddress:    "",
		hunt:          false,
		pathfindTest:  false,
		testFightUI:   false,
		testFightUIV1: false,
		geodataDir:    "",
		maxPassable:   uint(pathfind.DefaultMaxPassableHeight),
		proxy:         false,
		proxyLogin:    "",
		proxyGame:     "",
		proxyLog:      "",
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
	flag.BoolVar(&cfg.testFightUI, "test-fight-ui", false,
		"fight FX comparison gallery instead of the bot: no game "+
			"connection, a horizontal grid of numbered damage "+
			"visualization variants, each shown with the enemy "+
			"above, below, left and right of the character")
	flag.BoolVar(&cfg.testFightUIV1, "test-fight-ui-v1", false,
		"combat animation variant showcase UI (v1 idea set) instead of the "+
			"bot: no game connection, a looping hero versus enemy demo fight "+
			"plays every damage visualization idea side by side for picking one")
	flag.StringVar(&cfg.geodataDir, "geodata", "",
		"geodata directory with X_Y.l2j region files for the pathfind "+
			"test (auto detected when empty)")
	flag.BoolVar(&cfg.proxy, "proxy", false,
		"run the MITM proxy for real C1 clients: an emulated login "+
			"server on 127.0.0.1:2107 (+127.0.0.2:2106) and an emulated "+
			"game server on 127.0.0.1:7778 that attach a connecting "+
			"client to the live bot session (any login/password pair is "+
			"accepted, the char list shows the selected bot)")
	flag.StringVar(&cfg.proxyLogin, "proxy-login",
		proxy.DefaultLoginAddress+","+proxy.LoginFallbackAddress,
		"comma separated login listen addresses of the proxy (the first "+
			"is mandatory, the rest are optional fallbacks)")
	flag.StringVar(&cfg.proxyGame,
		"proxy-game", proxy.DefaultGameAddress+","+proxy.GameFallbackAddress,
		"comma separated game listen addresses of the proxy")
	flag.StringVar(&cfg.proxyLog, "proxy-log", defaultProxyLogPath,
		"file the proxy writes its client connection log to")
	flag.UintVar(&cfg.maxPassable, "max-passable",
		uint(pathfind.DefaultMaxPassableHeight),
		"maximum walkable height difference between neighbouring cells")
	flag.Parse()

	return cfg
}

// swarmDialer dials the login and game server connections with the
// connect timeout.
// Listing the deprecated Dialer fields explicitly would trip
// staticcheck SA1019, so the struct stays partial.
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
	ctx context.Context, cfg config, tracker *state.Bot, engine *pathfind.Engine,
	proxyServer *proxy.Server,
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
		return fmt.Errorf("game handshake failed: %w", err)
	}
	game.SetTracker(tracker)

	// The proxy observes the whole session (the recorder replays it to
	// connecting C1 clients) and forwards their packets through the
	// shared outbound cipher of the session. The registration must
	// happen before the session starts reading so nothing is missed.
	var sessionRecorder *proxy.Recorder
	if proxyServer != nil {
		sessionRecorder = proxyServer.RegisterSession(cfg.account, game, tracker)
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

	// The loop always runs: with -hunt it hunts autonomously, without
	// it stays in the manual mode and only executes the commands of the
	// web UI (map clicks, equipment drags) so the interface stays
	// interactive in both launch modes.
	loop := hunt.NewLoop(game, tracker)
	if engine != nil {
		loop.SetNavigator(hunt.NewNavigator(engine))
	} else if cfg.hunt {
		log.Println("Hunt runs without town trips: no geodata available")
	}
	if cfg.hunt {
		loop.SetHuntingZoneRegion("elven")
	} else {
		loop.SetAutonomy(false)
	}
	go loop.Run(sessionCtx)

	return game.Run(sessionCtx, cfg.charName)
}

// runBotForever keeps the bot in the world around the clock: a lost
// session (server restart, kicked connection, network failure) is logged
// and the whole login flow is retried with a growing backoff until the
// user stops the process with SIGINT/SIGTERM. The process never exits on
// its own. The geodata engine survives the reconnects.
func runBotForever(
	ctx context.Context, cfg config, tracker *state.Bot, engine *pathfind.Engine,
	proxyServer *proxy.Server,
) {
	delay := reconnectMinDelay
	for {
		started := time.Now()
		err := runBot(ctx, cfg, tracker, engine, proxyServer)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Println("Bot failed: " + err.Error())
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

	if cfg.testFightUI {
		runTestFightUI(cfg)

		return
	}

	if cfg.pathfindTest {
		runPathfindTest(cfg)

		return
	}

	if cfg.testFightUIV1 {
		runTestFightUIV1(cfg)

		return
	}

	log.Println("Starting swarm bot for account " + cfg.account)

	registry := state.NewRegistry()
	tracker := state.NewBot(cfg.account)
	registry.Add(tracker)

	var proxyServer *proxy.Server
	if cfg.proxy {
		proxyServer = startProxy(cfg)
	}

	web := startWebInterface(cfg, registry, nil, proxyServer)

	// The geodata engine serves the town trips of the hunt and the
	// long manual walks of the web UI (the server side pathfinder
	// refuses far targets), so it loads in every mode.
	var engine *pathfind.Engine
	dir := cfg.geodataDir
	if dir == "" {
		dir = detectGeodataDir()
	}
	engine = pathfind.NewEngine(dir)
	engine.SetMaxPassableHeight(uint16(cfg.maxPassable))
	stats := engine.Stats()
	if stats.HasData {
		log.Printf("Geodata ready: %d region files in %s, town trips "+
			"and manual long walks enabled", stats.RegionFiles, stats.Dir)
	} else {
		log.Println("No geodata files found in " + stats.Dir +
			", the bot hunts without town trips")
	}

	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)

	runBotForever(ctx, cfg, tracker, engine, proxyServer)
	stop()
	shutdownWebInterface(web)
	shutdownProxy(proxyServer)
	log.Println("Bot finished")
}

// startProxy builds and runs the client proxy with its own log file so
// the C1 client connection attempts can be diagnosed without digging
// through the console output of the bot (see docs/proxy.md).
func startProxy(cfg config) *proxy.Server {
	file, err := os.OpenFile(cfg.proxyLog,
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	logger := log.New(file, "proxy ", log.LstdFlags|log.Lmicroseconds)
	if err != nil {
		logger = log.Default()
		logger.Printf("Proxy log file %s unavailable: %v",
			cfg.proxyLog, err)
	}
	// The log file stays open for the process lifetime: the OS closes
	// it at exit, no deferred close here (the logger would write into a
	// closed file otherwise).

	server := proxy.NewServer(logger,
		proxy.WithLoginAddresses(splitAddresses(cfg.proxyLogin)...),
		proxy.WithGameAddresses(splitAddresses(cfg.proxyGame)...))
	if err := server.Listen(); err != nil {
		logger.Printf("Proxy failed to start: %v", err)

		return nil
	}
	go func() {
		if err := server.Serve(); err != nil {
			logger.Printf("Proxy stopped: %v", err)
		}
	}()

	return server
}

// splitAddresses splits a comma separated flag value.
func splitAddresses(value string) []string {
	addresses := strings.Split(value, ",")
	cleaned := make([]string, 0, len(addresses))
	for _, address := range addresses {
		address = strings.TrimSpace(address)
		if address != "" {
			cleaned = append(cleaned, address)
		}
	}

	return cleaned
}

// shutdownProxy stops the client proxy.
func shutdownProxy(server *proxy.Server) {
	if server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Proxy shutdown failed: %v", err)
	}
}

// runTestFightUI serves the bot less fight FX comparison gallery: the
// web interface answers with the static variant grid until the process
// is stopped. No game connection and no geodata is needed - the map
// background of the gallery cells is the static tile pyramid shipped
// with the web content.
func runTestFightUI(cfg config) {
	if cfg.webAddress == "" {
		log.Println("Fight FX test needs the web interface, " +
			"pass a -web address")

		return
	}

	web := webserver.NewTestFightServer(cfg.webAddress, log.Default())
	go func() {
		if err := web.ListenAndServe(); err != nil {
			log.Println("Web interface failed: " + err.Error())
		}
	}()

	log.Println("Fight FX test UI is ready")

	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	shutdownWebInterface(web)
	log.Println("Fight FX test finished")
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

	web := startWebInterface(cfg, nil, engine, nil)
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

// runTestFightUIV1 serves the combat animation variant showcase (the
// v1 idea set): no game connection and no bot, the web interface plays
// a looping hero versus enemy demo fight through every damage
// visualization idea (four enemy positions vertically, the variants
// horizontally with a scroll bar, the map tiles as the background)
// until the process is stopped. The user compares the numbered
// variants in the browser and picks the winner for the live map
// implementation.
func runTestFightUIV1(cfg config) {
	if cfg.webAddress == "" {
		log.Println("Fight test UI needs the web interface, " +
			"pass a -web address")

		return
	}

	server := webserver.NewFightServer(cfg.webAddress, log.Default())
	go func() {
		if err := server.ListenAndServe(); err != nil {
			log.Printf("Web interface stopped: %v", err)
		}
	}()

	log.Println("Fight test UI is ready")

	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	shutdownWebInterface(server)
	log.Println("Fight test finished")
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
	proxyServer *proxy.Server,
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
		if proxyServer != nil {
			server.SetProxy(proxyServer)
		}
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
