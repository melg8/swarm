// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Package spotaudit measures the real hunting spot geometry on the
// live Mobius C1 stack: for every spot of the registry it injects the
// probe character at the anchor (the database position rewrite of the
// acceptance scenarios - the character wakes up standing exactly on
// the anchor), waits out the knownlist broadcast and dumps every
// attackable npc the character actually sees (name, template, level,
// position, distance from the anchor, the leash verdict).
//
// The audit exists because the registry anchors were clustered from
// the spawn polygon squares, never measured against the live spawn
// positions: the 2026-09-13 test3 round caught a spot whose leash
// square fenced out the whole observed spider mass by 62 units (the
// bots starved on a ground that held mobs), and the fix procedure the
// owner defined is exactly this tool - spawn at the center of every
// point, dump what it really sees, then re-generate the zones from
// the measurement (tools/regenerate_spots_from_audit.py).
//
// The output JSON is the evidence file: one record per audited spot
// with the injected anchor, the position the server actually placed
// the character on, and the full npc population with distances. The
// run is resumable - the tool reloads its own output at startup and
// skips the spots already measured - so a long registry audits across
// several foreground runs without repeating the work.
package spotaudit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/melg8/swarm/internal/swarm/acceptance"
	"github.com/melg8/swarm/internal/swarm/connection"
	"github.com/melg8/swarm/internal/swarm/hunt"
	"github.com/melg8/swarm/internal/swarm/state"
)

// The character creation constants of the probe: the same elven
// fighter recipe every temp character of the acceptance scenarios
// uses (see acceptance/runner.go).
const (
	elfRaceID    = 1
	elfFighterID = 18
	male         = 0
	defaultHair  = 0
	defaultFace  = 0
)

// connectTimeout bounds the login and game dials of a probe session.
const connectTimeout = 10 * time.Second

// auditDialer is the shared connection dialer of the probe sessions.
//
//nolint:exhaustruct_v5 // the zero defaults are intended
var auditDialer = &net.Dialer{Timeout: connectTimeout}

// Config describes one audit run.
type Config struct {
	// Login is the login server address.
	Login string
	// Account, Password and Char name the probe character (the server
	// auto-creates the account, the audit creates the character).
	Account  string
	Password string
	Char     string
	// DB addresses the character position injection.
	DB acceptance.DBConfig
	// Wait is the knownlist settle window of every spot visit.
	Wait time.Duration
	// Output is the JSON evidence file (resumable).
	Output string
	// Anchors optionally overrides the audited positions per spot id
	// (the verification pass audits the re-generated geometry).
	Anchors string
	// Filter audits only the spots whose id contains the substring
	// (empty audits everything not yet measured).
	Filter string
	// Fresh drops the resume state and re-measures every spot.
	Fresh bool
}

// Default wait of one spot visit: the region knownlist broadcast
// completes within a second of the world entry, the window also
// covers one respawn cycle of the elven grounds (15-20 s) so a
// recently cleared ground shows its live population.
const defaultWait = 12 * time.Second

// The pause between the probe sessions: the game server releases the
// account when the connection drops, the next login of the same
// account must not race it.
const sessionPause = 2 * time.Second

// The offline poll of the character row: the logout store flushes a
// calm character immediately, the next position injection waits for
// it so the UPDATE never races the store of the previous visit.
const (
	offlinePollPeriod  = time.Second
	offlinePollTimeout = 45 * time.Second
)

// The default z of the injected position: the elven hunting grounds
// sit around -3500..-3700 and the server corrects the z of a stored
// position through its own ValidateLocation on the world entry - the
// knownlist grid only depends on x and y, so the exact z never
// affects the measurement.
const defaultZ = -3600

// NpcRecord is one observed attackable npc of a spot visit.
type NpcRecord struct {
	ObjectID   int32   `json:"object_id"`
	Name       string  `json:"name"`
	WireID     int32   `json:"wire_id"`
	Level      int32   `json:"level"`
	X          int32   `json:"x"`
	Y          int32   `json:"y"`
	Z          int32   `json:"z"`
	Distance   float64 `json:"distance"`
	InLeash    bool    `json:"in_leash"`
	Attackable bool    `json:"attackable"`
	Alive      bool    `json:"alive"`
}

// SpotRecord is the audit evidence of one spot.
type SpotRecord struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	AnchorX   int32  `json:"anchor_x"`
	AnchorY   int32  `json:"anchor_y"`
	AnchorZ   int32  `json:"anchor_z"`
	Radius    int32  `json:"radius"`
	LeashHalf int32  `json:"leash_half"`
	// StandingX/Y/Z is the position the server actually placed the
	// character on (the injected anchor plus the server side
	// ValidateLocation correction, when one applied).
	StandingX    int32       `json:"standing_x"`
	StandingY    int32       `json:"standing_y"`
	StandingZ    int32       `json:"standing_z"`
	Attackable   int         `json:"attackable_count"`
	InLeashCount int         `json:"in_leash_count"`
	Npcs         []NpcRecord `json:"npcs"`
}

// AuditFile is the JSON evidence file of a whole audit run.
type AuditFile struct {
	Account string       `json:"account"`
	WaitSec int          `json:"wait_sec"`
	Spots   []SpotRecord `json:"spots"`
}

// Run audits the spot registry on the live stack: one probe session
// per unaudited spot, the evidence file grows after every visit.
func Run(ctx context.Context, cfg Config, logger *log.Logger) error {
	if cfg.Wait <= 0 {
		cfg.Wait = defaultWait
	}
	spots := hunt.ElvenHuntingSpots()
	anchors, err := loadAnchors(cfg.Anchors)
	if err != nil {
		return err
	}
	audit, err := loadAudit(cfg.Output, cfg.Fresh)
	if err != nil {
		return err
	}
	done := make(map[string]bool, len(audit.Spots))
	for index := range audit.Spots {
		done[audit.Spots[index].ID] = true
	}
	db, err := acceptance.ConnectDB(cfg.DB)
	if err != nil {
		return fmt.Errorf("audit database: %w", err)
	}
	defer db.Close()
	charID, err := resolveCharacter(db, cfg.Account, cfg.Char)
	if err != nil {
		// The probe character does not exist yet: create it through
		// the game protocol (a level 1 elven fighter at the creation
		// spawn), then resolve the row again.
		if cerr := ensureCharacter(cfg); cerr != nil {
			return fmt.Errorf("audit probe character: %w", cerr)
		}
		charID, err = resolveCharacter(db, cfg.Account, cfg.Char)
		if err != nil {
			return err
		}
	}
	logger.Printf("spot audit: %d spots in the registry, %d already "+
		"measured, probe %s (char %d)", len(spots), len(audit.Spots),
		cfg.Account, charID)
	auditRemaining(ctx, cfg, db, charID, spots, anchors, done, audit, logger)
	if err := writeAudit(cfg.Output, cfg.Account, cfg.Wait, audit); err != nil {
		return err
	}
	logger.Printf("spot audit: %d of %d spots measured, evidence in %s",
		len(audit.Spots), len(spots), cfg.Output)

	return nil
}

// auditRemaining visits every unaudited spot of the registry in
// order, growing the evidence file after each visit (the resume
// protocol: an interrupted run continues with the next spot).
func auditRemaining(
	ctx context.Context, cfg Config, db *acceptance.DB, charID int64,
	spots []hunt.Spot, anchors map[string]anchor, done map[string]bool,
	audit *AuditFile, logger *log.Logger,
) {
	for index := range spots {
		if ctx.Err() != nil {
			return
		}
		spot := spots[index]
		if cfg.Filter != "" && !strings.Contains(spot.ID, cfg.Filter) {
			continue
		}
		if done[spot.ID] {
			continue
		}
		record, err := auditSpot(ctx, cfg, db, charID, spot, anchors, logger)
		if err != nil {
			logger.Printf("spot audit: %s failed: %v", spot.ID, err)

			continue
		}
		audit.Spots = append(audit.Spots, record)
		done[spot.ID] = true
		if err := writeAudit(cfg.Output, cfg.Account, cfg.Wait, audit); err != nil {
			logger.Printf("spot audit: evidence write failed: %v", err)

			return
		}
		logger.Printf("spot audit: %s (%s): %d attackable, %d in leash",
			spot.ID, spot.Name, record.Attackable, record.InLeashCount)
	}
}

// auditSpot performs one probe visit: the position injection, the
// session with the knownlist settle window, the npc dump and the
// graceful logout.
func auditSpot(
	ctx context.Context, cfg Config, db *acceptance.DB,
	charID int64, spot hunt.Spot, anchors map[string]anchor,
	logger *log.Logger,
) (SpotRecord, error) {
	ax, ay, radius := spot.AnchorX, spot.AnchorY, spot.Radius
	var z int32 = defaultZ
	if override, ok := anchors[spot.ID]; ok {
		ax, ay, z = override.X, override.Y, override.Z
		if override.Radius > 0 {
			radius = override.Radius
		}
	}
	waitCharacterOffline(db, charID, logger)
	if err := injectPosition(db, charID, ax, ay, z); err != nil {
		return SpotRecord{}, fmt.Errorf("position inject: %w", err)
	}
	record := SpotRecord{
		ID: spot.ID, Name: spot.Name,
		AnchorX: ax, AnchorY: ay, AnchorZ: z,
		Radius: radius, LeashHalf: leashHalf(radius),
		StandingX: 0, StandingY: 0, StandingZ: 0,
		Attackable: 0, InLeashCount: 0, Npcs: nil,
	}
	tracker := state.NewBot(cfg.Account)
	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	game, err := openSession(sessionCtx, cfg, tracker)
	if err != nil {
		return SpotRecord{}, err
	}
	if sx, sy, sz, ok := tracker.SelfPosition(); ok {
		record.StandingX, record.StandingY, record.StandingZ = sx, sy, sz
	}
	select {
	case <-sessionCtx.Done():
		return SpotRecord{}, errors.New("audit canceled")
	case <-time.After(cfg.Wait):
	}
	record.Npcs = collectNpcs(tracker, ax, ay, record.LeashHalf)
	record.Attackable = len(record.Npcs)
	for index := range record.Npcs {
		if record.Npcs[index].InLeash {
			record.InLeashCount++
		}
	}
	_ = game.RequestLogout()
	time.Sleep(sessionPause)
	_ = game.Close()

	return record, nil
}

// openSession connects the probe, enters the world and runs the
// packet loop in the background until the caller cancels.
func openSession(
	ctx context.Context, cfg Config, tracker *state.Bot,
) (*connection.GameClient, error) {
	loginConn, err := auditDialer.Dial("tcp", cfg.Login)
	if err != nil {
		return nil, fmt.Errorf("login dial: %w", err)
	}
	auth, err := connection.Authenticate(loginConn, cfg.Account, cfg.Password)
	if err != nil {
		return nil, fmt.Errorf("authenticate: %w", err)
	}
	gameConn, err := auditDialer.Dial("tcp", gameAddress(auth))
	if err != nil {
		return nil, fmt.Errorf("game dial: %w", err)
	}
	game, err := connection.NewGameClient(gameConn)
	if err != nil {
		return nil, fmt.Errorf("game handshake: %w", err)
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
		_ = game.Close()

		return nil, fmt.Errorf("game authentication: %w", err)
	}
	slot, _, found := charList.FindCharacterByName(cfg.Char)
	if !found {
		_ = game.Close()

		return nil, fmt.Errorf("character %s not found", cfg.Char)
	}
	if err := game.EnterWorld(int32(slot)); err != nil {
		_ = game.Close()

		return nil, fmt.Errorf("enter world: %w", err)
	}
	go func() { _ = game.Run(ctx, cfg.Char) }()

	return game, nil
}

// ensureCharacter creates the probe character through the game
// protocol when it is missing (the same creation recipe of the
// acceptance scenarios) and drops the connection without entering
// the world.
func ensureCharacter(cfg Config) error {
	loginConn, err := auditDialer.Dial("tcp", cfg.Login)
	if err != nil {
		return fmt.Errorf("login dial: %w", err)
	}
	auth, err := connection.Authenticate(loginConn, cfg.Account, cfg.Password)
	if err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}
	gameConn, err := auditDialer.Dial("tcp", gameAddress(auth))
	if err != nil {
		return fmt.Errorf("game dial: %w", err)
	}
	game, err := connection.NewGameClient(gameConn)
	if err != nil {
		return fmt.Errorf("game handshake: %w", err)
	}
	charList, err := game.Authenticate(connection.GameSessionParams{
		Account:    auth.Account,
		LoginOkID1: auth.LoginOkID1,
		LoginOkID2: auth.LoginOkID2,
		PlayOkID1:  auth.PlayOkID1,
		PlayOkID2:  auth.PlayOkID2,
	})
	if err != nil {
		_ = game.Close()

		return fmt.Errorf("game authentication: %w", err)
	}
	_, err = game.EnsureCharacter(connection.CharacterParams{
		Name:      cfg.Char,
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
	time.Sleep(sessionPause)

	return game.Close()
}

// collectNpcs dumps the attackable npc population of the tracker
// with the leash verdict of every mob.
func collectNpcs(
	tracker *state.Bot, ax int32, ay int32, half int32,
) []NpcRecord {
	ids := tracker.KnownObjectIDs()
	npcs := make([]NpcRecord, 0, len(ids))
	for _, id := range ids {
		if !tracker.ObjectAttackable(id) {
			continue
		}
		x, y, z, ok := tracker.ObjectPosition(id)
		if !ok {
			continue
		}
		level := int32(0)
		if observed, levelOK := tracker.ObjectLevel(id); levelOK {
			level = observed
		}
		npcs = append(npcs, NpcRecord{
			ObjectID:   id,
			Name:       tracker.ObjectName(id),
			WireID:     tracker.ObjectTemplateID(id),
			Level:      level,
			X:          x,
			Y:          y,
			Z:          z,
			Distance:   math.Hypot(float64(x-ax), float64(y-ay)),
			InLeash:    inLeashSquare(x, y, ax, ay, half),
			Attackable: true,
			Alive:      tracker.ObjectAlive(id),
		})
	}

	return npcs
}

// inLeashSquare reports whether the world point sits inside the leash
// square of the anchor (the engage geometry: a mob outside the square
// never enters a fight, whatever its distance).
func inLeashSquare(x int32, y int32, ax int32, ay int32, half int32) bool {
	return x >= ax-half && x <= ax+half && y >= ay-half && y <= ay+half
}

// leashHalf mirrors hunt.Spot.leashHalf: the square inscribed in the
// visibility circle of the radius.
func leashHalf(radius int32) int32 {
	half := int32(math.Round(float64(radius) / math.Sqrt2))
	if half < 1 {
		half = 1
	}

	return half
}

// gameAddress renders the game server endpoint of the auth result.
func gameAddress(auth *connection.AuthResult) string {
	return fmt.Sprintf("%d.%d.%d.%d:%d",
		auth.ServerIP[0], auth.ServerIP[1], auth.ServerIP[2], auth.ServerIP[3],
		auth.ServerPort)
}

// injectPosition rewrites the stored position of the probe character
// (and restores the vitals a previous visit may have spent on the
// aggressive grounds) while it stays offline.
func injectPosition(
	db *acceptance.DB, charID int64, x int32, y int32, z int32,
) error {
	query := "UPDATE characters SET x=" + strconv.Itoa(int(x)) +
		", y=" + strconv.Itoa(int(y)) +
		", z=" + strconv.Itoa(int(z)) +
		", heading=0, online=0" +
		", curHp=maxHp, curMp=maxMp, curCp=maxCp" +
		" WHERE charId=" + strconv.FormatInt(charID, 10)
	if _, err := db.Exec(query); err != nil {
		return fmt.Errorf("inject position: %w", err)
	}

	return nil
}

// resolveCharacter finds the character row id of the probe account.
func resolveCharacter(
	db *acceptance.DB, account string, char string,
) (int64, error) {
	rows, err := db.Query(
		"SELECT charId FROM characters WHERE account_name='" + account +
			"' AND char_name='" + char + "'")
	if err != nil {
		return 0, fmt.Errorf("resolve character: %w", err)
	}
	if len(rows) != 1 {
		return 0, fmt.Errorf("character %s not found on account %s",
			char, account)
	}
	id, err := strconv.ParseInt(rows[0][0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse character id %q: %w", rows[0][0], err)
	}

	return id, nil
}

// waitCharacterOffline polls the character row until the server
// flushed the previous probe session (the online flag drops with the
// logout store; a character that left in combat stores fifteen
// seconds later). The poll never fails the visit: a character that
// stayed marked online (a crashed server left the flag set) resets
// anyway - the position UPDATE below clears the flag itself.
func waitCharacterOffline(db *acceptance.DB, charID int64, logger *log.Logger) {
	deadline := time.Now().Add(offlinePollTimeout)
	query := "SELECT online FROM characters WHERE charId=" +
		strconv.FormatInt(charID, 10)
	for time.Now().Before(deadline) {
		rows, err := db.Query(query)
		if err == nil && len(rows) == 1 && rows[0][0] == "0" {
			return
		}
		if err != nil {
			logger.Printf("spot audit: the offline poll failed: %v", err)

			return
		}
		time.Sleep(offlinePollPeriod)
	}
	logger.Printf("spot audit: the character stayed marked online, " +
		"resetting anyway")
}

// newAuditFile starts an empty evidence file.
func newAuditFile() *AuditFile {
	return &AuditFile{Account: "", WaitSec: 0, Spots: nil}
}

// loadAudit reads the evidence file of a previous (interrupted) run:
// the resume state of the audit.
func loadAudit(path string, fresh bool) (*AuditFile, error) {
	if fresh || path == "" {
		return newAuditFile(), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return newAuditFile(), nil
		}

		return nil, fmt.Errorf("read audit %s: %w", path, err)
	}
	var audit AuditFile
	if err := json.Unmarshal(data, &audit); err != nil {
		return nil, fmt.Errorf("parse audit %s: %w", path, err)
	}

	return &audit, nil
}

// writeAudit stores the evidence file atomically enough for the
// resume protocol (a temp file plus the rename).
func writeAudit(
	path string, account string, wait time.Duration, audit *AuditFile,
) error {
	if path == "" {
		return nil
	}
	audit.Account = account
	audit.WaitSec = int(wait.Seconds())
	data, err := json.MarshalIndent(audit, "", "  ")
	if err != nil {
		return fmt.Errorf("encode audit: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write audit temp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename audit: %w", err)
	}

	return nil
}

// anchor is one position override of the verification pass.
type anchor struct {
	X      int32 `json:"x"`
	Y      int32 `json:"y"`
	Z      int32 `json:"z"`
	Radius int32 `json:"radius"`
}

// loadAnchors reads the optional anchor overrides of the
// verification pass: {"spots": {"<spot id>": {"x":.., "y":.., "z":..}}}.
func loadAnchors(path string) (map[string]anchor, error) {
	if path == "" {
		return map[string]anchor{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read anchors %s: %w", path, err)
	}
	var file struct {
		Spots map[string]anchor `json:"spots"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse anchors %s: %w", path, err)
	}

	return file.Spots, nil
}
