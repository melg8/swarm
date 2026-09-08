// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Package proxy implements the MITM server the real Lineage 2 C1 client
// connects to: an emulated login server, an emulated game server and a
// transparent packet relay over the live bot session. The client talks
// to the proxy exactly like it would talk to the real Mobius servers,
// while the swarm stays connected to the real servers as the bot.
package proxy

import (
	"sync"
)

// Recorder keeps the decrypted server-to-client packet stream of one bot
// session so a connecting game client can be brought up to the current
// world state (the replay) and then continue with the live feed.
//
// Every entry owns an immutable copy of its payload: entries are shared
// with replay readers and live subscribers, and the senders that encrypt
// the payloads in place always work on their own copies.
//
// The history is bounded: the first recorderPrologueEntries entries (the
// char selected packet and the enter world burst) and a rolling tail are
// always kept, the middle is dropped once the byte cap is reached. A
// dropped middle only degrades the replayed world state, never the
// cipher chains (the proxy re-encrypts everything it sends).
type Recorder struct {
	mu          sync.Mutex
	entries     []RecorderEntry
	bytes       int
	nextSeq     int64
	closed      bool
	closeSignal chan struct{}
	subscribers map[*Subscriber]struct{}
}

// RecorderEntry is one recorded packet with its sequence number.
type RecorderEntry struct {
	seq     int64
	payload []byte
}

// Recorder size policy: the prologue covers the enter world burst (a few
// hundred packets around a populated spawn), the tail keeps the recent
// minutes of the session and the cap bounds the memory of one bot.
const (
	recorderPrologueEntries = 2048
	recorderTailEntries     = 16384
	recorderMaxBytes        = 32 << 20
)

// NewRecorder creates an empty packet history.
func NewRecorder() *Recorder {
	return &Recorder{
		mu:          sync.Mutex{},
		entries:     nil,
		bytes:       0,
		nextSeq:     1,
		closed:      false,
		closeSignal: make(chan struct{}),
		subscribers: make(map[*Subscriber]struct{}),
	}
}

// Record appends one decrypted server packet payload. It is called from
// the session reader goroutine through the GameClient tap and must stay
// fast: it copies the payload once and never blocks on the subscribers
// (a subscriber that fell behind is poisoned instead).
func (r *Recorder) Record(payload []byte) {
	if len(payload) == 0 {
		return
	}
	entry := RecorderEntry{
		seq:     0,
		payload: make([]byte, len(payload)),
	}
	copy(entry.payload, payload)

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}

	entry.seq = r.nextSeq
	r.nextSeq++
	r.entries = append(r.entries, entry)
	r.bytes += len(entry.payload)
	r.trimLocked()

	update := liveUpdate(entry) // S1016: direct conversion of the same layout
	for sub := range r.subscribers {
		sub.deliver(update)
	}
}

// trimLocked drops middle entries once the byte cap is exceeded, keeping
// the prologue and the tail intact. The caller holds the lock.
func (r *Recorder) trimLocked() {
	if r.bytes <= recorderMaxBytes {
		return
	}
	dropFrom := recorderPrologueEntries
	dropTo := len(r.entries) - recorderTailEntries
	if dropTo <= dropFrom {
		return
	}
	dropped := 0
	for i := dropFrom; i < dropTo; i++ {
		dropped += len(r.entries[i].payload)
	}
	kept := make([]RecorderEntry, 0, dropFrom+recorderTailEntries)
	kept = append(kept, r.entries[:dropFrom]...)
	kept = append(kept, r.entries[dropTo:]...)
	r.entries = kept
	r.bytes -= dropped
}

// Close ends the history: the live subscribers are released so their
// client connections wind down, and further records are ignored.
func (r *Recorder) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	r.closed = true
	close(r.closeSignal)
	r.subscribers = nil
}

// Closed reports whether the history ended (the bot session died).
func (r *Recorder) Closed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.closed
}

// CloseSignal returns a channel closed when the recorder ends.
func (r *Recorder) CloseSignal() <-chan struct{} {
	return r.closeSignal
}

// FirstPacketSeq returns the sequence number of the first entry with the
// given opcode, or 0 when no such entry exists.
func (r *Recorder) FirstPacketSeq(opcode byte) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.entries {
		if len(r.entries[i].payload) > 0 && r.entries[i].payload[0] == opcode {
			return r.entries[i].seq
		}
	}

	return 0
}

// Entry returns the payload of the entry with the given sequence number,
// or nil when the entry is gone.
func (r *Recorder) Entry(seq int64) []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.entries {
		if r.entries[i].seq == seq {
			return r.entries[i].payload
		}
	}

	return nil
}

// Attach returns every entry recorded after the given sequence together
// with a live subscription that continues exactly where the returned
// entries end: the snapshot and the subscription are taken under one
// lock, so no entry is lost between them. The returned payloads must not
// be modified.
func (r *Recorder) Attach(afterSeq int64) ([]RecorderEntry, *Subscriber) {
	r.mu.Lock()
	defer r.mu.Unlock()

	snapshot := make([]RecorderEntry, 0, len(r.entries))
	for i := range r.entries {
		if r.entries[i].seq > afterSeq {
			snapshot = append(snapshot, r.entries[i])
		}
	}

	sub := &Subscriber{
		ch:      make(chan liveUpdate, subscriberQueueSize),
		poison:  make(chan struct{}),
		stopped: false,
	}
	if !r.closed {
		r.subscribers[sub] = struct{}{}
	}

	return snapshot, sub
}

// removeSubscriber drops a subscription of a detached client.
func (r *Recorder) removeSubscriber(sub *Subscriber) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.subscribers, sub)
}

// Walk calls fn for every recorded entry in sequence order until fn
// returns false. The payloads are immutable shared copies: the callback
// must not modify them.
func (r *Recorder) Walk(fn func(seq int64, payload []byte) bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.entries {
		if !fn(r.entries[i].seq, r.entries[i].payload) {
			return
		}
	}
}

// Stats summarizes the history for logs.
type Stats struct {
	Entries int
	Bytes   int
}

// Stats reports the current history size.
func (r *Recorder) Stats() Stats {
	r.mu.Lock()
	defer r.mu.Unlock()

	return Stats{Entries: len(r.entries), Bytes: r.bytes}
}

// liveUpdate is one live packet handed to a subscriber.
type liveUpdate struct {
	seq     int64
	payload []byte
}

// subscriberQueueSize bounds the live queue of one client connection: a
// client that lets the queue overflow is poisoned and disconnected
// because skipping packets would corrupt its world view.
const subscriberQueueSize = 1024

// Subscriber receives the live packets of a recorder.
type Subscriber struct {
	ch      chan liveUpdate
	poison  chan struct{}
	stopped bool
}

// deliver hands one update to the subscriber without blocking the
// recorder: a full queue poisons the subscriber. The caller holds the
// recorder lock.
func (s *Subscriber) deliver(update liveUpdate) {
	if s.stopped {
		return
	}
	select {
	case s.ch <- update:
	default:
		s.poisonSubscriber()
	}
}

// poisonSubscriber marks the subscriber as fallen behind exactly once.
func (s *Subscriber) poisonSubscriber() {
	if s.stopped {
		return
	}
	s.stopped = true
	close(s.poison)
}
