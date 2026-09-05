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
	"syscall"
	"time"

	"github.com/melg8/swarm/internal/swarm/connection"
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
	attackNearest bool
}

func parseFlags() config {
	cfg := config{
		loginAddress:  "",
		account:       "",
		password:      "",
		charName:      "",
		webAddress:    "",
		attackNearest: false,
	}
	flag.StringVar(&cfg.loginAddress, "login", defaultLoginAddress,
		"login server address")
	flag.StringVar(&cfg.account, "account", defaultAccount, "account name")
	flag.StringVar(&cfg.password, "password", defaultPassword, "account password")
	flag.StringVar(&cfg.charName, "char", defaultCharName, "character name")
	flag.StringVar(&cfg.webAddress, "web", defaultWebAddress,
		"web interface address, empty disables it")
	flag.BoolVar(&cfg.attackNearest, "attack-nearest", false,
		"debug helper: keep attacking the nearest attackable npc")
	flag.Parse()

	return cfg
}

// connectLoginServer establishes the login server connection.
func connectLoginServer(address string) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", address, connectTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to login server: %w", err)
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
		return nil, fmt.Errorf("failed to connect to game server: %w", err)
	}
	log.Println("Connected to game server at " + address)

	return conn, nil
}

// runBot performs the full bot flow: login, game handshake, authentication,
// character creation and staying in the world until the context is done.
func runBot(ctx context.Context, cfg config, tracker *state.Bot) error {
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
	log.Printf("Playing character %s of level %d", charInfo.Name, charInfo.Level)

	//nolint:gosec // slot index is bounded by the character list length
	if err := game.EnterWorld(int32(slot)); err != nil {
		return fmt.Errorf("failed to enter world: %w", err)
	}
	log.Println("Character " + cfg.charName + " entered the world")

	if cfg.attackNearest {
		go attackNearestLoop(ctx, game)
	}

	return game.Run(ctx, cfg.charName)
}

// attackPeriod is the refresh rate of the attack helper loop.
const attackPeriod = 3 * time.Second

// attackNearestLoop keeps attacking the nearest attackable npc while the
// context runs.
func attackNearestLoop(ctx context.Context, game *connection.GameClient) {
	ticker := time.NewTicker(attackPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := game.AttackNearest(); err != nil {
				log.Printf("Attack nearest failed: %v", err)
			}
		}
	}
}

func main() {
	cfg := parseFlags()
	log.SetOutput(os.Stdout)
	log.Println("Starting swarm bot for account " + cfg.account)

	registry := state.NewRegistry()
	tracker := state.NewBot(cfg.account)
	registry.Add(tracker)

	web := startWebInterface(cfg, registry)

	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	err := runBot(ctx, cfg, tracker)
	stop()
	shutdownWebInterface(web)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Bot finished")
}

// startWebInterface runs the web server in the background when enabled.
func startWebInterface(
	cfg config, registry *state.Registry,
) *webserver.Server {
	if cfg.webAddress == "" {
		return nil
	}
	server := webserver.NewServer(registry, cfg.webAddress, log.Default())
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
