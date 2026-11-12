// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "context"
    "log"
    "os/signal"
    "strconv"
    "strings"
    "sync"
    "syscall"

    "github.com/melg8/swarm/internal/swarm/proxy"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/melg8/swarm/internal/version"
)

// runFleet launches multiple bot sessions in one process: each bot gets
// its own account (the base -account/-char name plus the 1-based index,
// so -account test1 -bots 3 makes test1, test2, test3), its own tracker
// in a shared registry, and its own runBotForever goroutine. All bots
// share one web interface (the sidebar lists every bot, clicking
// switches the observed one), one proxy (the web UI selects which bot a
// connecting C1 client attaches to) and one geodata engine. The process
// stays alive until every bot supervisor returns (SIGINT/SIGTERM stops
// them all through the shared context).
//
//nolint:funlen // the fleet start walks the phases in order
func runFleet(cfg config) {
    plan := cfg.botPlans
    log.Printf("Starting swarm fleet of %d bots (%s)",
        len(plan), compositionText(plan))
    log.Printf("Build: %s", version.Identity())

    registry := state.NewRegistry()
    trackers := make([]*state.Bot, 0, len(plan))
    for _, entry := range plan {
        tracker := state.NewBot(entry.Account)
        tracker.SetKind(state.KindLongRunning)
        registry.Add(tracker)
        trackers = append(trackers, tracker)
    }
    log.Printf("Fleet accounts: %s", fleetAccountList(cfg.account, len(plan)))

    // The journal opens before the web interface so the session
    // report endpoint registers with it (the fleet mode once missed
    // the wiring: the button answered 404 while the journal itself
    // kept collecting - the two symptoms of a fleet dump look broken).
    journal := openSessionJournal(cfg)

    var proxyServer *proxy.Server
    if cfg.proxy {
        proxyServer = startProxy(cfg)
    }

    // The geodata engine is shared by all bots: the town trips and the
    // manual long walks of every session read through the same LRU
    // cache of parsed regions.
    engine := newBotEngine(cfg)
    stats := engine.Stats()
    if stats.HasData {
        log.Printf("Geodata ready: %d region files in %s, town trips "+
            "and manual long walks enabled", stats.RegionFiles, stats.Dir)
    } else {
        log.Println("No geodata files found in " + stats.Dir +
            ", the bot hunts without town trips")
    }
    mesh := loadNavmesh(cfg)

    web := startWebInterface(cfg, registry, nil, proxyServer)
    attachAcceptance(web, registry, cfg, engine, mesh, proxyServer)
    if web != nil && journal != nil {
        web.SetSessionJournal(journal)
    }

    ctx, stop := signal.NotifyContext(context.Background(),
        syscall.SIGINT, syscall.SIGTERM)
    startMemoryWatch(ctx)

    journalWG := &sync.WaitGroup{}
    for _, tracker := range trackers {
        wireSessionBot(ctx, journal, tracker, journalWG)
    }

    // Launch every bot supervisor in its own goroutine. A per-bot
    // config carries the plan slot (the derived account and the bot
    // type); the rest of the flags (login, hunt, geodata, navmesh,
    // proxy) stay shared.
    var wg sync.WaitGroup
    for i, tracker := range trackers {
        botCfg := cfg
        botCfg.account = plan[i].Account
        botCfg.charName = plan[i].Account
        botCfg.botType = plan[i].Type
        botCfg.kite = plan[i].Kite
        wg.Add(1)
        go func(c config, t *state.Bot) {
            defer wg.Done()
            log.Printf("Bot %s of type %s joining the fleet",
                c.account, c.botType)
            runBotForever(ctx, c, t, engine, mesh, proxyServer, journal,
                journalWG)
        }(botCfg, tracker)
    }
    wg.Wait()
    stop()
    shutdownWebInterface(web)
    shutdownProxy(proxyServer)
    // The hunt loops and the samplers hold their final records until
    // their contexts unwind: they join here, before the journal
    // closes, so the last story lines and samples land in the file
    // instead of dropping on a closed queue.
    journalWG.Wait()
    if journal != nil {
        journal.Shutdown("fleet finished")
        journal.Close()
    }
    log.Println("Fleet finished")
}

// fleetAccountName derives the account name of bot i from the base
// name. The base name (cfg.account, default "test1") is used as-is for
// the first bot; a numbered base continues its own ladder
// (test1 -> test2, test3 - and temp2 -> temp3, temp4), a base without
// a trailing digit starts the ladder at two (bot -> bot2, bot3).
func fleetAccountName(base string, i int) string {
    if i == 0 {
        return base
    }
    stripped := strings.TrimRight(base, "0123456789")
    // A numbered base continues its own ladder: the temp2 fleet runs
    // temp2, temp3, temp4 - the follower index rides the base number
    // instead of colliding with the base itself (temp2, temp2).
    baseNum := 1
    if digits := base[len(stripped):]; digits != "" {
        if parsed, err := strconv.Atoi(digits); err == nil {
            baseNum = parsed
        }
    }

    return stripped + strconv.Itoa(baseNum+i)
}

// fleetAccountList builds the comma separated account list for the
// startup log line.
func fleetAccountList(base string, count int) string {
    if count <= 0 {
        return ""
    }
    var sb strings.Builder
    sb.WriteString(fleetAccountName(base, 0))
    for i := 1; i < count; i++ {
        sb.WriteString(", ")
        sb.WriteString(fleetAccountName(base, i))
    }

    return sb.String()
}
