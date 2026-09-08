// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import "sync"

// Registry keeps the bot sessions known to the process so that the web
// interface can enumerate and switch between them.
//
// Data oriented layout: the sessions live in one dense slice walked
// by List (sequential memory, no per bot map hashing on the poll path
// of the web view), while the id lookup stays a map. The map holds
// positions instead of pointers, so both structures stay consistent
// through the same write lock.
type Registry struct {
	mu    sync.RWMutex
	bots  []*Bot
	index map[string]int32
}

// NewRegistry creates an empty bot registry.
func NewRegistry() *Registry {
	return &Registry{
		mu:    sync.RWMutex{},
		bots:  nil,
		index: make(map[string]int32),
	}
}

// Add registers a bot, replacing any previous bot with the same id in
// place so the enumeration order stays stable.
func (r *Registry) Add(bot *Bot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if slot, exists := r.index[bot.id]; exists {
		r.bots[slot] = bot

		return
	}
	r.index[bot.id] = int32(len(r.bots))
	r.bots = append(r.bots, bot)
}

// Get returns the bot with the given id.
func (r *Registry) Get(id string) (*Bot, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	bot, ok := r.lookupLocked(id)

	return bot, ok
}

// lookupLocked resolves the id into the bot under the read lock.
func (r *Registry) lookupLocked(id string) (*Bot, bool) {
	slot, ok := r.index[id]
	if !ok || slot < 0 || int(slot) >= len(r.bots) {
		return nil, false
	}

	return r.bots[slot], true
}

// mustGet returns the bot with the given id or nil. Test helper.
func (r *Registry) mustGet(id string) *Bot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	bot, _ := r.lookupLocked(id)

	return bot
}

// List returns the compact info of all bots in registration order.
// The dense slice walk keeps the per bot cost at the Info() call
// itself, not at a map hash plus a random pointer hop per session.
func (r *Registry) List() []BotInfo {
	r.mu.RLock()
	infos := make([]BotInfo, len(r.bots))
	for i, bot := range r.bots {
		infos[i] = bot.Info()
	}
	r.mu.RUnlock()

	return infos
}
