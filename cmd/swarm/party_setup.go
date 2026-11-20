// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "log"

    "github.com/melg8/swarm/internal/swarm/party"
)

// loadPartyHub reads the party configuration file and builds the process wide
// hub every session of the fleet coordinates through. A missing or broken file
// disables the group play with a log line: the fleet still hunts alone.
func loadPartyHub(path string) *party.Hub {
    if path == "" {
        return nil
    }
    config, err := party.LoadConfig(path)
    if err != nil {
        log.Printf("Party coordination off: %v", err)

        return nil
    }
    log.Printf("Party coordination ready: %s", config.Describe())

    return party.NewHub(config)
}
