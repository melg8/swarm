// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Package party holds the group composition of the fleet and the in-process
// hub the group members coordinate through. The server party itself is formed
// with the C1 party packets; the hub only shares the live placement and the
// current target the members publish every hunt tick, because all bots of a
// fleet run in one process and the follower decisions need the leader state
// even when the game client has lost sight of it.
package party

import (
    "encoding/json"
    "errors"
    "fmt"
    "os"
    "strings"
    "sync"
    "time"
)

// maxGroupMembers is the server party cap (Party.java, nine players).
const maxGroupMembers = 9

// Config is the party configuration file: one or more groups, each with a
// leader and the accounts that follow it. The file is separate from the
// launch configuration so a fleet composition change never touches the
// grouping.
type Config struct {
    Groups []Group `json:"groups"`
}

// Group is one party: the leader account and every member account, the
// leader included.
type Group struct {
    Leader  string   `json:"leader"`
    Members []string `json:"members"`
}

// LoadConfig reads and validates the party configuration file.
func LoadConfig(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("failed to read the party config: %w", err)
    }
    var config Config
    if err := json.Unmarshal(data, &config); err != nil {
        return nil, fmt.Errorf("failed to parse the party config: %w", err)
    }
    if err := config.normalize(); err != nil {
        return nil, fmt.Errorf("invalid party config: %w", err)
    }

    return &config, nil
}

// normalize trims the account names and validates the grouping: every group
// carries a leader listed among its members, no account sits in two groups and
// a group never exceeds the server party cap.
func (c *Config) normalize() error {
    if len(c.Groups) == 0 {
        return errors.New("no groups defined")
    }
    owner := make(map[string]string)
    for i := range c.Groups {
        group := &c.Groups[i]
        group.Leader = strings.TrimSpace(group.Leader)
        if group.Leader == "" {
            return fmt.Errorf("group %d has no leader", i)
        }
        if len(group.Members) == 0 {
            return fmt.Errorf("group of %s has no members", group.Leader)
        }
        if len(group.Members) > maxGroupMembers {
            return fmt.Errorf("group of %s holds %d members, the party "+
                "cap is %d", group.Leader, len(group.Members),
                maxGroupMembers)
        }
        leaderListed := false
        for j := range group.Members {
            member := strings.TrimSpace(group.Members[j])
            if member == "" {
                return fmt.Errorf("group of %s has an empty member",
                    group.Leader)
            }
            group.Members[j] = member
            if member == group.Leader {
                leaderListed = true
            }
            if previous, taken := owner[member]; taken {
                return fmt.Errorf("account %s belongs to two groups "+
                    "(leaders %s and %s)", member, previous, group.Leader)
            }
            owner[member] = group.Leader
        }
        if !leaderListed {
            return fmt.Errorf("the leader %s is not listed among the "+
                "members of its group", group.Leader)
        }
    }

    return nil
}

// Describe renders the grouping for the startup log.
func (c *Config) Describe() string {
    parts := make([]string, 0, len(c.Groups))
    for _, group := range c.Groups {
        parts = append(parts, group.Leader+"["+
            strings.Join(group.Members, ",")+"]")
    }

    return strings.Join(parts, " ")
}

// MemberState is the live placement one member publishes every hunt tick: the
// object id its client sees itself under, the current target, the position and
// the liveness. At stamps the publish for the freshness check.
type MemberState struct {
    Account  string
    ObjectID int32
    TargetID int32
    X        int32
    Y        int32
    Z        int32
    Alive    bool
    At       time.Time
}

// Hub is the process wide party registry: the group of every account and the
// last published state. One fleet process owns one hub and every loop of the
// fleet reads through it.
type Hub struct {
    mu     sync.Mutex
    groups map[string]Group
    states map[string]MemberState
}

// NewHub builds the hub from a validated configuration.
func NewHub(config *Config) *Hub {
    hub := &Hub{
        groups: make(map[string]Group, len(config.Groups)),
        states: make(map[string]MemberState),
    }
    for _, group := range config.Groups {
        for _, member := range group.Members {
            hub.groups[member] = group
        }
    }

    return hub
}

// GroupOf returns the leader and the members of the group the account belongs
// to. The member slice is a copy: the caller keeps it past the lock.
func (h *Hub) GroupOf(account string) (string, []string, bool) {
    h.mu.Lock()
    defer h.mu.Unlock()
    group, ok := h.groups[account]
    if !ok {
        return "", nil, false
    }
    members := make([]string, len(group.Members))
    copy(members, group.Members)

    return group.Leader, members, true
}

// Publish records the live state of one member.
func (h *Hub) Publish(state MemberState) {
    h.mu.Lock()
    defer h.mu.Unlock()
    h.states[state.Account] = state
}

// Member returns the last published state of one account.
func (h *Hub) Member(account string) (MemberState, bool) {
    h.mu.Lock()
    defer h.mu.Unlock()
    state, ok := h.states[account]

    return state, ok
}

// LeaderOf returns the last published state of the leader of the account's
// group.
func (h *Hub) LeaderOf(account string) (MemberState, bool) {
    h.mu.Lock()
    defer h.mu.Unlock()
    group, ok := h.groups[account]
    if !ok {
        return MemberState{}, false
    }
    state, ok := h.states[group.Leader]

    return state, ok
}
