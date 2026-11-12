// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "sync"
    "syscall"

    "github.com/melg8/swarm/internal/swarm/proxy"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/melg8/swarm/internal/version"
)

//nolint:cyclop,funlen // one flat mode switch by design
func main() {
    cfg := parseFlags()
    log.SetOutput(os.Stdout)

    // The fleet composition normalizes to one plan whatever its
    // origin: the -config file (validated, expanded) or the plain
    // -bots ladder. The mode switches below (the acceptance runs,
    // the viewers, the journal queries) never launch a bot session,
    // so the normalization only has to precede the fleet dispatch.
    if cfg.botPlans == nil {
        cfg.botPlans = classicFleetPlan(cfg.account, cfg.bots)
    }
    cfg.botType = cfg.botPlans[0].Type
    cfg.kite = cfg.botPlans[0].Kite

    if cfg.sessionAnomalies != "" {
        runSessionAnomaliesCLI(cfg)

        return
    }

    if cfg.sessionQuery != "" {
        runSessionQueryCLI(cfg)

        return
    }

    if cfg.sessionReport != "" {
        runSessionReportCLI(cfg)

        return
    }

    if cfg.testFightUI {
        runTestFightUI(cfg)

        return
    }

    if cfg.testPathUI {
        runTestPathUI(cfg)

        return
    }

    if cfg.testPathV2UI {
        runTestPathV2UI(cfg)

        return
    }

    if cfg.testPathV3UI {
        runTestPathV3UI(cfg)

        return
    }

    if cfg.pathfindTest {
        runPathfindTest(cfg)

        return
    }

    if cfg.navmeshShow.enabled {
        runNavmeshViewer(cfg)

        return
    }

    if cfg.testFightUIV1 {
        runTestFightUIV1(cfg)

        return
    }

    if cfg.huntAudit != "" {
        runHuntAuditCLI(cfg)

        return
    }

    if cfg.acceptanceRun != "" {
        runAcceptanceCLI(cfg)

        return
    }

    if len(cfg.botPlans) > 1 {
        runFleet(cfg)

        return
    }

    log.Println("Starting swarm bot for account " + cfg.account +
        " (type " + cfg.botType + ")")
    // The identity line pairs every bot log with the exact code state
    // - the state dump of the web UI carries the same line.
    log.Printf("Build: %s", version.Identity())

    registry := state.NewRegistry()
    tracker := state.NewBot(cfg.account)
    tracker.SetKind(state.KindLongRunning)
    registry.Add(tracker)

    journal := openSessionJournal(cfg)
    ctx, stop := signal.NotifyContext(context.Background(),
        syscall.SIGINT, syscall.SIGTERM)
    journalWG := &sync.WaitGroup{}
    wireSessionBot(ctx, journal, tracker, journalWG)
    startMemoryWatch(ctx)

    var proxyServer *proxy.Server
    if cfg.proxy {
        proxyServer = startProxy(cfg)
    }

    // The geodata engine serves the town trips of the hunt and the
    // long manual walks of the web UI (the server side pathfinder
    // refuses far targets), so it loads in every mode.
    engine := newBotEngine(cfg)
    stats := engine.Stats()
    if stats.HasData {
        log.Printf("Geodata ready: %d region files in %s, town trips "+
            "and manual long walks enabled", stats.RegionFiles, stats.Dir)
    } else {
        log.Println("No geodata files found in " + stats.Dir +
            ", the bot hunts without town trips")
    }

    // The navmesh tiles of the offline build accelerate the long
    // hunt routes when they exist (see loadNavmesh); a missing tile
    // set keeps everything on the grid engine.
    mesh := loadNavmesh(cfg)

    web := startWebInterface(cfg, registry, nil, proxyServer)
    attachAcceptance(web, registry, cfg, engine, mesh, proxyServer)
    if web != nil && journal != nil {
        web.SetSessionJournal(journal)
    }

    runBotForever(ctx, cfg, tracker, engine, mesh, proxyServer, journal,
        journalWG)
    stop()
    shutdownWebInterface(web)
    shutdownProxy(proxyServer)
    // The hunt loop and the sampler hold their final records until
    // their contexts unwind: they join here, before the journal
    // closes, so the last story lines and samples land in the file
    // instead of dropping on a closed queue.
    journalWG.Wait()
    if journal != nil {
        journal.Shutdown("process finished")
        journal.Close()
    }
    log.Println("Bot finished")
}
