// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package state

import "sync"

// Registry keeps the bot sessions known to the process so that the web
// interface can enumerate and switch between them.
type Registry struct {
	mu    sync.RWMutex
	bots  map[string]*Bot
	order []string
}

// NewRegistry creates an empty bot registry.
func NewRegistry() *Registry {
	return &Registry{
		mu:    sync.RWMutex{},
		bots:  make(map[string]*Bot),
		order: nil,
	}
}

// Add registers a bot, replacing any previous bot with the same id.
func (r *Registry) Add(bot *Bot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.bots[bot.id]; !exists {
		r.order = append(r.order, bot.id)
	}
	r.bots[bot.id] = bot
}

// Get returns the bot with the given id.
func (r *Registry) Get(id string) (*Bot, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	bot, ok := r.bots[id]

	return bot, ok
}

// mustGet returns the bot with the given id or nil. Test helper.
func (r *Registry) mustGet(id string) *Bot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.bots[id]
}

// List returns the compact info of all bots in registration order.
func (r *Registry) List() []BotInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	infos := make([]BotInfo, 0, len(r.order))
	for _, id := range r.order {
		infos = append(infos, r.bots[id].Info())
	}

	return infos
}
