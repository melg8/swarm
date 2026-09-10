// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package proxy

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/melg8/swarm/internal/swarm/connection"
	"github.com/melg8/swarm/internal/swarm/state"
	"github.com/stretchr/testify/require"
)

// The packet growth repro: the user reported that with a real C1 client
// attached through the proxy the WebUI packet counter of the attached
// character explodes (~1M packets in under 30 seconds, heavy memory
// load). The harness drives the exact production wiring (a live bot
// session per bot, the proxy registration, the WebUI style SelectBot
// switch) against the deployed stack and instruments every layer:
//
//   - both bot trackers' packet counters (the WebUI number),
//   - an opcode histogram of everything each bot session receives
//     (a wrapper around the recorder tap),
//   - an opcode histogram of everything the client receives,
//   - the proxy log for the relay lifecycle.
//
// It needs the deployed stack and the explicit opt in (see
// TestProxyE2ERealStackClientFlow):
//
//	SWARM_PROXY_E2E=1 go test ./internal/swarm/proxy/ \
//	    -run TestProxyPacketGrowthRepro -v -timeout 5m
//
// Tunables (env):
//
//	SWARM_REPRO_PING     none | slow | fast | pong  (default pong)
//	SWARM_REPRO_SECONDS  per phase soak seconds      (default 15)
type reproTap struct {
	mu     sync.Mutex
	counts map[byte]int64
	next   func([]byte)
}

func newReproTap(next func([]byte)) *reproTap {
	return &reproTap{counts: make(map[byte]int64), next: next}
}

func (t *reproTap) Record(payload []byte) {
	if len(payload) > 0 {
		t.mu.Lock()
		t.counts[payload[0]]++
		t.mu.Unlock()
	}
	if t.next != nil {
		t.next(payload)
	}
}

func (t *reproTap) snapshot() (total int64, top string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	type pair struct {
		op byte
		n  int64
	}
	list := make([]pair, 0, len(t.counts))
	for op, n := range t.counts {
		list = append(list, pair{op, n})
		total += n
	}
	sort.Slice(list, func(i, j int) bool { return list[i].n > list[j].n })
	var builder strings.Builder
	for i, p := range list {
		if i >= 4 {
			break
		}
		fmt.Fprintf(&builder, " 0x%02x:%d", p.op, p.n)
	}

	return total, builder.String()
}

// reproClient reads the client stream in the background, keeps the
// opcode histogram of what the attached client receives and optionally
// reacts to received packets (the pong ping mode emulates a client that
// answers every NetPing with the next request).
type reproClient struct {
	gameClient *fakeGameClient
	mu         sync.Mutex
	reads      int64
	counts     map[byte]int64
	onPacket   func(payload []byte)
	readErr    error
}

func watchClient(
	c *fakeGameClient, onPacket func(payload []byte),
) *reproClient {
	rc := &reproClient{
		gameClient: c, counts: make(map[byte]int64), onPacket: onPacket,
	}
	go func() {
		for {
			payload, err := readWirePacket(c.conn, c.readBuf)
			if err != nil {
				rc.mu.Lock()
				rc.readErr = err
				rc.mu.Unlock()

				return
			}
			c.readBuf = payload
			if len(payload) == 0 {
				continue
			}
			c.crypt.Decrypt(payload)
			rc.mu.Lock()
			rc.reads++
			rc.counts[payload[0]]++
			rc.mu.Unlock()
			if rc.onPacket != nil {
				rc.onPacket(payload)
			}
		}
	}()

	return rc
}

func (rc *reproClient) stats() (reads int64, top string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	type pair struct {
		op byte
		n  int64
	}
	list := make([]pair, 0, len(rc.counts))
	for op, n := range rc.counts {
		list = append(list, pair{op, n})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].n > list[j].n })
	var builder strings.Builder
	for i, p := range list {
		if i >= 4 {
			break
		}
		fmt.Fprintf(&builder, " 0x%02x:%d", p.op, p.n)
	}

	return rc.reads, builder.String()
}

// runReproBotSession mirrors runE2EBotSession with a parameterized
// account and a counting tap so the repro can histogram what each bot
// session receives.
func runReproBotSession(
	ctx context.Context, server *Server, tracker *state.Bot, tap *reproTap,
	logger *log.Logger, account string,
) error {
	tracker.ResetSession()

	loginConn, err := net.DialTimeout("tcp", e2eLoginAddress, 10*time.Second)
	if err != nil {
		return fmt.Errorf("login dial: %w", err)
	}
	auth, err := connection.Authenticate(loginConn, account, e2ePassword)
	if err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}
	server.SetRsaModulus(auth.RsaPublicKey)

	gameAddress := fmt.Sprintf("%d.%d.%d.%d:%d",
		auth.ServerIP[0], auth.ServerIP[1], auth.ServerIP[2], auth.ServerIP[3],
		auth.ServerPort)
	gameConn, err := net.DialTimeout("tcp", gameAddress, 10*time.Second)
	if err != nil {
		return fmt.Errorf("game dial: %w", err)
	}

	game, err := connection.NewGameClient(gameConn)
	if err != nil {
		return fmt.Errorf("game handshake: %w", err)
	}
	game.SetTracker(tracker)
	game.SetLogger(logger)

	recorder := server.RegisterSession(account, game, tracker)
	tap.next = recorder.Record
	game.SetTap(tap.Record)
	defer server.UnregisterSession(account, recorder)

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
		Race:      1,
		Female:    0,
		ClassID:   18,
		HairStyle: 0,
		HairColor: 0,
		Face:      0,
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

	return game.Run(ctx, account)
}

// reproPhase samples both bots' packet counters over one soak window
// while the client behaves like a real C1 client (reading the stream,
// pinging at the configured rate). It returns the peak packets/second
// rate and the total counter growth of the window.
func reproPhase(
	t *testing.T, label string, seconds int, pingMode string,
	botA, botB *state.Bot, tapA, tapB *reproTap, client *reproClient,
) (peakRate int64, growth int64) {
	t.Helper()

	startA, startB := botA.Packets(), botB.Packets()
	start := time.Now()
	var peak int64

	pingEvery := 300 * time.Millisecond
	if pingMode == "fast" {
		pingEvery = 30 * time.Millisecond
	}
	stopPing := func() {}
	// A nil channel blocks forever in a select, so the modes without a
	// ticker ride the same loop.
	var pingCh <-chan time.Time
	if pingMode == "slow" || pingMode == "fast" {
		pingTicker := time.NewTicker(pingEvery)
		defer pingTicker.Stop()
		pingCh = pingTicker.C
	}
	if pingMode == "pong" {
		client.onPacket = func(payload []byte) {
			if payload[0] == 0xEC {
				client.gameClient.sendPacket([]byte{0xA8})
			}
		}
		stopPing = func() { client.onPacket = nil }
	}
	defer stopPing()

	sampleTick := time.NewTicker(3 * time.Second)
	defer sampleTick.Stop()
	deadline := time.After(time.Duration(seconds) * time.Second)

	lastA, lastB := startA, startB
	for {
		select {
		case <-deadline:
			return peak, max(botA.Packets()-startA, botB.Packets()-startB)
		case <-pingCh:
			client.gameClient.sendPacket([]byte{0xA8})
		case <-sampleTick.C:
			curA, curB := botA.Packets(), botB.Packets()
			reads, topClient := client.stats()
			totalA, histA := tapA.snapshot()
			totalB, histB := tapB.snapshot()
			elapsed := int64(max(time.Since(start).Round(time.Second), time.Second) /
				time.Second)
			rateA, rateB := (curA-lastA)/3, (curB-lastB)/3
			lastA, lastB = curA, curB
			peak = max(peak, rateA, rateB)
			t.Logf("[%s] t+%ds: botA %d (+%d/s, tap %d |%s) botB %d (+%d/s, tap %d |%s) "+
				"client reads %d |%s",
				label, elapsed, curA, rateA, totalA, histA,
				curB, rateB, totalB, histB, reads, topClient)
		}
	}
}

// TestProxyPacketGrowthRepro reproduces the reported packet explosion:
// a client attached through the proxy (optionally through the WebUI
// bot switch) must not inflate the attached bot's inbound packet
// counter. The threshold is generous: a healthy relay adds the client
// ping round trips (a few per second) on top of the ambient world
// traffic, anything past a few hundred packets per 15 seconds is the
// feedback loop the user observed.
func TestProxyPacketGrowthRepro(t *testing.T) {
	if os.Getenv("SWARM_PROXY_E2E") != "1" {
		t.Skip("set SWARM_PROXY_E2E=1 and deploy the stack with " +
			"tools/swarm_fast_deploy.sh to run the packet growth repro")
	}
	requireStackPort(t, 2106)
	requireStackPort(t, 7777)

	pingMode := os.Getenv("SWARM_REPRO_PING")
	if pingMode == "" {
		// The pong mode is the default: it emulates the answer driven
		// keepalive of the real C1 client (one new RequestNetPing per
		// delivered NetPing answer) that armed the feedback loop of the
		// user report, so the regression stays sharp by default.
		pingMode = "pong"
	}
	seconds := 15
	if v := os.Getenv("SWARM_REPRO_SECONDS"); v != "" {
		var parsed int
		_, err := fmt.Sscanf(v, "%d", &parsed)
		require.NoError(t, err)
		seconds = parsed
	}

	logDir := t.TempDir()
	logPath := filepath.Join(logDir, "proxy.log")
	logFile, err := os.Create(logPath)
	require.NoError(t, err)
	logger := log.New(logFile, "proxy ", log.LstdFlags|log.Lmicroseconds)
	t.Cleanup(func() {
		content, readErr := os.ReadFile(logPath)
		if readErr == nil {
			text := string(content)
			t.Logf("---- proxy.log (full, %d bytes) ----\n%s", len(text), text)
		}
	})

	server := NewServer(logger,
		WithLoginAddresses("127.0.0.1:0"),
		WithGameAddresses("127.0.0.1:0"))
	require.NoError(t, server.Listen())
	go func() {
		_ = server.Serve()
	}()
	t.Cleanup(func() { _ = server.Shutdown(context.Background()) })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	botA := state.NewBot("stormreproa")
	tapA := newReproTap(nil)
	sessionErrs := make(chan error, 4)
	// The bot logins are staggered: the Mobius login flood protector closes
	// a second connection from the same IP inside 350 ms (FastConnectionTime).
	go func() {
		sessionErrs <- runReproBotSession(ctx, server, botA, tapA, logger, "stormreproa")
	}()
	requireOnline(t, botA)
	botB := state.NewBot("stormreprob")
	tapB := newReproTap(nil)
	go func() {
		sessionErrs <- runReproBotSession(ctx, server, botB, tapB, logger, "stormreprob")
	}()
	requireOnline(t, botB)
	select {
	case err := <-sessionErrs:
		t.Fatalf("a bot session died during the setup: %v", err)
	default:
	}

	// --- the client side: the full attach flow through the proxy ---
	loginClient := dialLogin(t, server.LoginAddr())
	loginClient.readInit()
	loginClient.login("whatever", "credentials")
	list := loginClient.requestServerList()
	entry := list.Servers[0]
	playOk := loginClient.requestServerLogin(byte(entry.ServerID))

	gameAddress := fmt.Sprintf("%d.%d.%d.%d:%d",
		entry.IP[0], entry.IP[1], entry.IP[2], entry.IP[3], entry.Port)
	gameClient := dialGame(t, gameAddress)
	require.NoError(t, gameClient.conn.SetDeadline(
		time.Now().Add(10*time.Minute)))
	gameClient.handshake()

	authLogin := []byte{0x08}
	authLogin = appendUTF16(authLogin, "whatever")
	authLogin = appendInt32LE(authLogin, playOk.PlayOkID2)
	authLogin = appendInt32LE(authLogin, playOk.PlayOkID1)
	authLogin = appendInt32LE(authLogin, int32(0x1111))
	authLogin = appendInt32LE(authLogin, int32(0x2222))
	gameClient.sendPacket(authLogin)

	charListPayload := readPacketUntil(t, gameClient, e2eReadTimeout,
		func(payload []byte) bool { return payload[0] == 0x1F },
		"the char list")
	require.Equal(t, byte(0x1F), charListPayload[0])

	gameClient.sendPacket([]byte{0x0D, 0x00, 0x00, 0x00, 0x00})
	selectedPayload := readPacketUntil(t, gameClient, e2eReadTimeout,
		func(payload []byte) bool { return payload[0] == 0x21 },
		"the char selected answer")
	require.Equal(t, byte(0x21), selectedPayload[0])

	gameClient.sendPacket([]byte{0x03})
	readPacketUntil(t, gameClient, e2eReadTimeout,
		func(payload []byte) bool { return payload[0] == 0x04 },
		"the replayed user info")

	client := watchClient(gameClient, nil)

	// Phase 1: plain attach soak (the pre-switch mode).
	peak1, growth1 := reproPhase(t, "attach", seconds, pingMode,
		botA, botB, tapA, tapB, client)
	t.Logf("attach phase: peak rate %d/s, total growth %d", peak1, growth1)

	// Phase 2: the WebUI style switch onto bot B (the restart dance),
	// then the post-switch soak - the mode of the user report.
	server.SelectBot("stormreprob")
	// The fake client plays its side of the dance: after the
	// RestartResponse and the auto selected answer it enters the world.
	// watchClient started after the first char selected answer, so the
	// dance answer is the first 0x21 it ever sees.
	danceDeadline := time.Now().Add(20 * time.Second)
	entered := false
	for !entered && time.Now().Before(danceDeadline) {
		time.Sleep(100 * time.Millisecond)
		client.mu.Lock()
		sawRestart := client.counts[0x74] > 0
		sawSelected := client.counts[0x21] >= 1
		readerErr := client.readErr
		client.mu.Unlock()
		if readerErr != nil {
			t.Fatalf("the client reader died during the dance: %v", readerErr)
		}
		if sawRestart && sawSelected {
			gameClient.sendPacket([]byte{0x03})
			entered = true
		}
	}
	require.True(t, entered, "the restart dance did not reach the client")
	time.Sleep(3 * time.Second)

	peak2, growth2 := reproPhase(t, "switched", seconds, pingMode,
		botA, botB, tapA, tapB, client)
	t.Logf("switched phase: peak rate %d/s, total growth %d", peak2, growth2)

	threshold := int64(300 * seconds / 15)
	require.LessOrEqual(t, peak1, threshold,
		"the attached bot packet rate exploded in the attach phase "+
			"(the reported feedback loop)")
	require.LessOrEqual(t, peak2, threshold,
		"the attached bot packet rate exploded after the switch "+
			"(the reported feedback loop)")

	t.Logf("REPRO_RESULT: no explosion (attach peak %d/s, switched peak %d/s, "+
		"threshold %d/s) with ping mode %q",
		peak1, peak2, threshold, pingMode)
}

// appendInt32LE appends a little endian int32 to the packet builder.
func appendInt32LE(dst []byte, value int32) []byte {
	return append(dst,
		byte(value), byte(value>>8), byte(value>>16), byte(value>>24))
}
