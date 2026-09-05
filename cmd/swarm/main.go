// SPDX-FileCopyrightText: 2024 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/melg8/swarm/internal/swarm/connection"
)

// Default configuration values.
const (
	defaultLoginAddress = "127.0.0.1:2106"
	defaultAccount      = "test1"
	defaultPassword     = "test"
	defaultCharName     = "test1"
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
	loginAddress string
	account      string
	password     string
	charName     string
}

func parseFlags() config {
	cfg := config{
		loginAddress: "",
		account:      "",
		password:     "",
		charName:     "",
	}
	flag.StringVar(&cfg.loginAddress, "login", defaultLoginAddress,
		"login server address")
	flag.StringVar(&cfg.account, "account", defaultAccount, "account name")
	flag.StringVar(&cfg.password, "password", defaultPassword, "account password")
	flag.StringVar(&cfg.charName, "char", defaultCharName, "character name")
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
func runBot(ctx context.Context, cfg config) error {
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

	return game.Run(ctx, cfg.charName)
}

func main() {
	cfg := parseFlags()
	log.SetOutput(os.Stdout)
	log.Println("Starting swarm bot for account " + cfg.account)

	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	err := runBot(ctx, cfg)
	stop()
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Bot finished")
}
