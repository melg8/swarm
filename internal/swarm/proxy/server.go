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

// Default listen addresses of the proxy. The login listeners cover every
// combination a C1 client can dial: the classic executable hardcodes the
// auth port 2106 (the [URL] Port line of l2.ini is an Unreal Engine
// leftover the auth connection ignores - the stock C1 l2.ini ships
// Port=7777), and l2.ini ServerAddr decides the loopback address. So the
// proxy answers 2106 and 2107 on both 127.0.0.1 and 127.0.0.2: a client
// honoring the ini port reaches 127.0.0.1:2107, a hardcoded-port client
// with ServerAddr=127.0.0.1 reaches 127.0.0.1:2106, and ServerAddr
// =127.0.0.2 variants reach the second loopback pair. The game listeners
// mirror the same addresses on port 7778 because the emulated login
// server advertises the address family the client used for the login
// connection. Only the first login address is mandatory; the rest are
// optional (127.0.0.1:2106 is normally owned by the real Mobius login
// server, see the bind diagnostics in Listen).
const (
	DefaultLoginAddress   = "127.0.0.1:2107"
	LoginInterceptAddress = "127.0.0.1:2106"
	LoginFallbackAddress  = "127.0.0.2:2106"
	LoginAltPortAddress   = "127.0.0.2:2107"
	DefaultGameAddress    = "127.0.0.1:7778"
	GameFallbackAddress   = "127.0.0.2:7778"
)

// DefaultLoginAddresses is the login listener set of the default
// configuration (see the const block comment for the routing rationale).
func DefaultLoginAddresses() []string {
	return []string{
		DefaultLoginAddress, LoginInterceptAddress,
		LoginFallbackAddress, LoginAltPortAddress,
	}
}

// DefaultGameAddresses is the game listener set of the default
// configuration.
func DefaultGameAddresses() []string {
	return []string{DefaultGameAddress, GameFallbackAddress}
}

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
	// selectionCh is closed and replaced whenever SelectBot changes the
	// selection to a different id: the live relay goroutines select on
	// the current channel so they wake up immediately and resync the
	// connected client onto the newly selected bot without waiting for
	// a reconnect.
	selectionCh chan struct{}

	rsaModulus     atomic.Value // []byte
	connSeq        atomic.Int64
	clients        atomic.Int64
	loginListeners []net.Listener
	gameListeners  []net.Listener
	done           chan struct{}
	stopOnce       sync.Once
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
		loginAddrs:     DefaultLoginAddresses(),
		gameAddrs:      DefaultGameAddresses(),
		logger:         logger,
		transformer:    PassthroughTransformer{},
		mu:             sync.Mutex{},
		sessions:       nil,
		selected:       "",
		selectionCh:    make(chan struct{}),
		rsaModulus:     atomic.Value{},
		connSeq:        atomic.Int64{},
		clients:        atomic.Int64{},
		loginListeners: nil,
		gameListeners:  nil,
		done:           make(chan struct{}),
		stopOnce:       sync.Once{},
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

// SelectBot marks the bot the next connecting client attaches to, and
// when the id differs from the current selection it wakes every live
// relay goroutine so a connected C1 client resyncs onto the newly
// selected bot immediately (no reconnect needed). An unknown id is
// stored anyway (the client falls back to the first session until
// that bot registers).
func (s *Server) SelectBot(id string) {
	s.mu.Lock()
	if s.selected == id {
		s.mu.Unlock()

		return
	}
	s.selected = id
	// Close the current selection channel (wakes every relay waiting
	// on it) and install a fresh one for the next change.
	close(s.selectionCh)
	s.selectionCh = make(chan struct{})
	s.mu.Unlock()
	s.logger.Printf("Proxy selection switched to bot %q", id)
}

// selectionChannel returns the current selection notification channel.
// The caller uses it to detect a selection change: the returned channel
// is closed when SelectBot picks a different id. A snapshot read under
// the mutex keeps the channel stable for the select call.
func (s *Server) selectionChannel() chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.selectionCh
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

// tryResolveSelectedSession returns the online session of the currently
// selected bot, or nil when the selection is empty, unregistered or not
// in the world yet. A char selected answer served right after a
// selection change re-resolves through it, so the client always enters
// the newest WebUI selection even when it fired mid dance or mid load.
func (s *Server) tryResolveSelectedSession() *botSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.selected == "" {
		return nil
	}
	for i := range s.sessions {
		if s.sessions[i].id == s.selected &&
			s.sessions[i].tracker.Status() == state.StatusOnline {
			return s.sessions[i]
		}
	}

	return nil
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

// sessionByID returns the currently registered session of the given bot
// id, nil when the id is not registered. The relogin handoff of a held
// client polls it to notice the replacement session of the same bot.
func (s *Server) sessionByID(id string) *botSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.sessions {
		if s.sessions[i].id == id {
			return s.sessions[i]
		}
	}

	return nil
}

// ClientCount returns the number of connected game clients.
func (s *Server) ClientCount() int {
	return int(s.clients.Load())
}

// gamePort returns the game port advertised to clients: the port of
// the primary game listener (the default 7778, custom ports follow the
// -proxy-game flag).
func (s *Server) gamePort() int32 {
	if len(s.gameListeners) > 0 {
		addr := s.gameListeners[0].Addr().String()
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
// fallback listeners only log their failures together with the likely
// causes and remedies (a busy 127.0.0.1:2106 or 127.0.0.2:2106 means the
// real login server still owns the auth port, see data/client/Readme.txt
// and docs/proxy.md).
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
		s.boundLoginAddrs(), s.boundGameAddrs())

	return s.Serve()
}

// LoginAddr returns the address of the primary login listener (the
// empty string before Listen).
func (s *Server) LoginAddr() string {
	if len(s.loginListeners) > 0 {
		return s.loginListeners[0].Addr().String()
	}

	return ""
}

// GameAddr returns the address of the primary game listener (the empty
// string before Listen).
func (s *Server) GameAddr() string {
	if len(s.gameListeners) > 0 {
		return s.gameListeners[0].Addr().String()
	}

	return ""
}

// LoginAddrs lists the successfully bound login listener addresses (the
// first entry is the primary of LoginAddr). Empty before Listen.
func (s *Server) LoginAddrs() []string {
	return s.boundLoginAddrs()
}

// GameAddrs lists the successfully bound game listener addresses (the
// first entry is the primary of GameAddr). Empty before Listen.
func (s *Server) GameAddrs() []string {
	return s.boundGameAddrs()
}

// boundLoginAddrs lists the successfully bound login listener addresses.
func (s *Server) boundLoginAddrs() []string {
	addrs := make([]string, 0, len(s.loginListeners))
	for _, listener := range s.loginListeners {
		addrs = append(addrs, listener.Addr().String())
	}

	return addrs
}

// boundGameAddrs lists the successfully bound game listener addresses.
func (s *Server) boundGameAddrs() []string {
	addrs := make([]string, 0, len(s.gameListeners))
	for _, listener := range s.gameListeners {
		addrs = append(addrs, listener.Addr().String())
	}

	return addrs
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
// The listener bookkeeping is family specific: gamePort and the primary
// address accessors read the first listener of their own family, so
// extra login listeners must not interleave into the game slice.
func (s *Server) listenFamily(
	login bool, addrs []string, serve func(net.Listener),
) error {
	for i, addr := range addrs {
		//nolint:exhaustruct_v5 // the zero fields of ListenConfig are the defaults
		listener, err := (&net.ListenConfig{}).Listen(
			context.Background(), "tcp", addr)
		if err != nil {
			if i == 0 {
				return fmt.Errorf("proxy cannot bind %s: %w", addr, err)
			}
			s.logger.Printf("Proxy optional listener %s skipped: %v", addr, err)
			s.logBindHint(login, addr)

			continue
		}
		if login {
			s.loginListeners = append(s.loginListeners, listener)
		} else {
			s.gameListeners = append(s.gameListeners, listener)
		}
		go serve(listener)
	}

	return nil
}

// logBindHint explains why an optional listener could not bind and how
// to free it. The interesting case is the hardcoded auth port 2106: the
// real Mobius login server owns it by default (a wildcard 0.0.0.0:2106
// bind also blocks 127.0.0.2:2106 on Windows with the access permissions
// error), and Windows itself can reserve the port range through Hyper-V
// or WinNAT. The hint mirrors the two recipes of docs/proxy.md.
func (s *Server) logBindHint(login bool, addr string) {
	if !login {
		return // a busy custom game port has no generic remedy
	}
	if _, port, err := net.SplitHostPort(addr); err == nil && port != "2106" {
		return // a custom -proxy-login port: nothing generic to explain
	}
	s.logger.Printf(
		"hint: classic C1 clients hardcode the login port 2106, so a client "+
			"whose l2.ini ServerAddr matches %s dials this address. The port is "+
			"normally owned by the real Mobius login server: either point the "+
			"client elsewhere (set ServerAddr=127.0.0.2 in l2.ini, the proxy "+
			"answers 127.0.0.2:2106 too) or free 127.0.0.1:2106 for the proxy "+
			"(set LoginserverHostname=127.0.0.3 in the Mobius login Server.ini "+
			"and run swarm with -login 127.0.0.3:2106). A bind rejected with "+
			"access permissions on Windows also means the port is reserved "+
			"(Hyper-V/WinNAT: 'netsh interface ipv4 show excludedportrange "+
			"protocol=tcp' lists 2106, 'net stop winnat' or a reboot frees it).",
		addr)
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
	for _, listener := range append(s.loginListeners, s.gameListeners...) {
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			s.logger.Printf("Proxy listener close failed: %v", err)
		}
	}

	return nil
}
