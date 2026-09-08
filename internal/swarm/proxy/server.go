// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package proxy

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/melg8/swarm/internal/swarm/state"
)

// Default listen addresses of the proxy. The login listeners carry both
// redirect paths: 127.0.0.1:2107 answers the l2.ini Port redirect, and
// 127.0.0.2:2106 answers clients whose executable hardcodes the login
// port 2106 and whose l2.ini ServerAddr points at the second loopback
// address (the whole 127.0.0.0/8 block is loopback, so the client needs
// no patching). The game listeners mirror the same addresses on port
// 7778 because the emulated login server advertises the address the
// client used for the login connection.
const (
	DefaultLoginAddress  = "127.0.0.1:2107"
	LoginFallbackAddress = "127.0.0.2:2106"
	DefaultGameAddress   = "127.0.0.1:7778"
	GameFallbackAddress  = "127.0.0.2:7778"
)

// gameProxyPort is the game server port the emulated login server
// advertises in its server list.
const gameProxyPort = 7778

// RawSender sends a raw decrypted client packet to the real game
// server through a live bot session (connection.GameClient implements
// it with SendRaw).
type RawSender interface {
	SendRaw(payload []byte) error
}

// Server is the MITM proxy: it emulates the login and game servers for
// real C1 clients and relays their traffic through the bot sessions of
// the process.
type Server struct {
	loginAddrs  []string
	gameAddrs   []string
	logger      *log.Logger
	transformer Transformer

	mu       sync.Mutex
	sessions []*botSession
	selected string

	rsaModulus atomic.Value // []byte
	connSeq    atomic.Int64
	clients    atomic.Int64
	listeners  []net.Listener
	done       chan struct{}
	stopOnce   sync.Once
}

// botSession couples the live pieces of one bot the proxy serves: the
// recorded server packet history, the raw send path to the real game
// server and the state tracker with the played character.
type botSession struct {
	id       string
	recorder *Recorder
	client   RawSender
	tracker  *state.Bot
}

// Option configures a proxy Server.
type Option func(*Server)

// WithLoginAddresses overrides the login listen addresses. The first
// entry is mandatory (a bind failure there is fatal), the rest are
// optional fallbacks whose bind failures are only logged.
func WithLoginAddresses(addrs ...string) Option {
	return func(s *Server) { s.loginAddrs = addrs }
}

// WithGameAddresses overrides the game listen addresses with the same
// mandatory-first rule.
func WithGameAddresses(addrs ...string) Option {
	return func(s *Server) { s.gameAddrs = addrs }
}

// WithTransformer installs a packet transformer; the default is the
// transparent passthrough.
func WithTransformer(t Transformer) Option {
	return func(s *Server) { s.transformer = t }
}

// NewServer creates the proxy bound to the default addresses.
func NewServer(logger *log.Logger, opts ...Option) *Server {
	server := &Server{
		loginAddrs:  []string{DefaultLoginAddress, LoginFallbackAddress},
		gameAddrs:   []string{DefaultGameAddress, GameFallbackAddress},
		logger:      logger,
		transformer: PassthroughTransformer{},
		mu:          sync.Mutex{},
		sessions:    nil,
		selected:    "",
		rsaModulus:  atomic.Value{},
		connSeq:     atomic.Int64{},
		clients:     atomic.Int64{},
		listeners:   nil,
		done:        make(chan struct{}),
		stopOnce:    sync.Once{},
	}
	for _, opt := range opts {
		opt(server)
	}

	return server
}

// SetRsaModulus publishes the scrambled RSA modulus of the real
// login server so the emulated Init packet mirrors it (captured by
// the bot's own login flow, see connection.Authenticate).
func (s *Server) SetRsaModulus(modulus []byte) {
	s.rsaModulus.Store(modulus)
}

// rsaModulusBytes returns the published modulus or 128 zero bytes.
func (s *Server) rsaModulusBytes() []byte {
	if value, ok := s.rsaModulus.Load().([]byte); ok && len(value) > 0 {
		return value
	}

	return make([]byte, 128)
}

// RegisterSession publishes one live bot session. The recorder it
// returns feeds the GameClient tap; the same GameClient must implement
// the RawSender of the session. Registering an id again replaces its
// previous session (the reconnect supervisor cycles sessions).
func (s *Server) RegisterSession(
	id string, client RawSender, tracker *state.Bot,
) *Recorder {
	recorder := NewRecorder()
	session := &botSession{
		id: id, recorder: recorder, client: client, tracker: tracker,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	replaced := false
	for i := range s.sessions {
		if s.sessions[i].id == id {
			old := s.sessions[i]
			s.sessions[i] = session
			replaced = true
			go old.recorder.Close()

			break
		}
	}
	if !replaced {
		s.sessions = append(s.sessions, session)
	}

	return recorder
}

// UnregisterSession removes the bot session and ends its history, which
// also disconnects the game clients attached to it. The recorder
// argument guards the reconnect cycle: only the session that still owns
// the id is removed, a replaced session unregisters nothing.
func (s *Server) UnregisterSession(id string, recorder *Recorder) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.sessions {
		if s.sessions[i].id == id {
			if s.sessions[i].recorder != recorder {
				return
			}
			session := s.sessions[i]
			s.sessions = append(s.sessions[:i], s.sessions[i+1:]...)
			session.recorder.Close()

			return
		}
	}
}

// SelectBot marks the bot the next connecting client attaches to. An
// unknown id is stored anyway (the client falls back to the first
// session until that bot registers).
func (s *Server) SelectBot(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.selected = id
	s.logger.Printf("Proxy selection switched to bot %q", id)
}

// SelectedBot returns the stored selection ("" when nothing selected).
func (s *Server) SelectedBot() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.selected
}

// currentSessionLocked resolves the session a connecting client should
// attach to: the web UI selection when it is registered, otherwise the
// first registered session. The caller holds the lock.
func (s *Server) currentSessionLocked() *botSession {
	if s.selected != "" {
		for i := range s.sessions {
			if s.sessions[i].id == s.selected {
				return s.sessions[i]
			}
		}
	}
	if len(s.sessions) > 0 {
		return s.sessions[0]
	}

	return nil
}

// SessionWait bounds how long a connecting client waits for a bot
// session to appear (the bot may still be logging in when the client
// arrives).
const SessionWait = 8 * time.Second

// resolveSession returns the session to serve, waiting up to
// SessionWait for a session whose character entered the world.
func (s *Server) resolveSession() *botSession {
	deadline := time.Now().Add(SessionWait)
	for {
		s.mu.Lock()
		session := s.currentSessionLocked()
		s.mu.Unlock()
		if session != nil && session.tracker.Status() == state.StatusOnline {
			return session
		}
		if time.Now().After(deadline) {
			return session
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// SessionIDs lists the registered bot session ids in registration order.
func (s *Server) SessionIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.sessions))
	for i := range s.sessions {
		ids = append(ids, s.sessions[i].id)
	}

	return ids
}

// ClientCount returns the number of connected game clients.
func (s *Server) ClientCount() int {
	return int(s.clients.Load())
}

// gamePort returns the game port advertised to clients: the port of
// the primary game listener (the default 7778, custom ports follow the
// -proxy-game flag).
func (s *Server) gamePort() int32 {
	if len(s.listeners) > 1 {
		addr := s.listeners[1].Addr().String()
		if _, port, err := net.SplitHostPort(addr); err == nil {
			if value, err := strconv.ParseInt(port, 10, 32); err == nil {
				return int32(value)
			}
		}
	}

	return gameProxyPort
}

// nextConnID numbers client connections for the log.
func (s *Server) nextConnID() int64 {
	return s.connSeq.Add(1)
}

// Listen binds the login and game listeners. The first address of each
// family is mandatory (a bind failure there returns the error), the
// fallback listeners only log their failures (a busy 127.0.0.2:2106
// means the real login server still owns 0.0.0.0:2106, see
// data/client/Readme.txt).
func (s *Server) Listen() error {
	err := s.listenFamily(true, s.loginAddrs, s.serveLoginListener)
	if err != nil {
		return err
	}

	return s.listenFamily(false, s.gameAddrs, s.serveGameListener)
}

// ListenAndServe binds the listeners and serves connections until the
// server is shut down.
func (s *Server) ListenAndServe() error {
	if err := s.Listen(); err != nil {
		return err
	}
	s.logger.Printf("Proxy ready: login on %v, game on %v",
		s.loginAddrs, s.gameAddrs)

	return s.Serve()
}

// LoginAddr returns the address of the primary login listener (the
// empty string before Listen).
func (s *Server) LoginAddr() string {
	if len(s.listeners) > 0 {
		return s.listeners[0].Addr().String()
	}

	return ""
}

// GameAddr returns the address of the primary game listener (the empty
// string before Listen).
func (s *Server) GameAddr() string {
	if len(s.listeners) > 1 {
		return s.listeners[1].Addr().String()
	}

	return ""
}

// Serve accepts and serves connections until the server is shut down.
func (s *Server) Serve() error {
	<-s.done

	return nil
}

// serve accepts connections of one listener until the server stops.
func (s *Server) serve(listener net.Listener, handle func(net.Conn)) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-s.done:
				return
			default:
			}
			s.logger.Printf("Proxy accept failed: %v", err)

			return
		}
		go handle(conn)
	}
}

// listenFamily binds every address of one family and starts accepting.
func (s *Server) listenFamily(
	mandatory bool, addrs []string, serve func(net.Listener),
) error {
	for i, addr := range addrs {
		//nolint:exhaustruct // the zero fields of ListenConfig are the defaults
		listener, err := (&net.ListenConfig{}).Listen(
			context.Background(), "tcp", addr)
		if err != nil {
			if i == 0 && mandatory {
				return fmt.Errorf("proxy cannot bind %s: %w", addr, err)
			}
			s.logger.Printf("Proxy optional listener %s skipped: %v", addr, err)

			continue
		}
		s.listeners = append(s.listeners, listener)
		go serve(listener)
	}

	return nil
}

// serveLoginListener accepts login server connections.
func (s *Server) serveLoginListener(listener net.Listener) {
	s.serve(listener, s.handleLoginConn)
}

// serveGameListener accepts game server connections.
func (s *Server) serveGameListener(listener net.Listener) {
	s.serve(listener, s.handleGameConn)
}

// Shutdown stops the proxy and closes the listeners.
func (s *Server) Shutdown(_ context.Context) error {
	s.stopOnce.Do(func() { close(s.done) })
	for _, listener := range s.listeners {
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			s.logger.Printf("Proxy listener close failed: %v", err)
		}
	}

	return nil
}
