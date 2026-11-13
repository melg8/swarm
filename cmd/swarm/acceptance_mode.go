// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "path/filepath"
    "syscall"

    "github.com/melg8/swarm/internal/swarm/acceptance"
    "github.com/melg8/swarm/internal/swarm/huntaudit"
    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
    "github.com/melg8/swarm/internal/swarm/proxy"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/melg8/swarm/internal/swarm/webserver"
    "github.com/melg8/swarm/internal/version"
)

// attachAcceptance wires the acceptance test manager into the web
// interface: the temp test bots (temp1..temp3) register in the shared
// bot registry, so they appear in the sidebar bot list and stay
// connectable through the client proxy exactly like the fleet bots.
func attachAcceptance(
    web *webserver.Server, registry *state.Registry, cfg config,
    engine *pathfind.Engine, mesh *navmesh.Mesh, proxyServer *proxy.Server,
) {
    if web == nil {
        return
    }
    manager := newAcceptanceManager(registry, cfg, engine, mesh,
        proxyServer)
    web.SetAcceptance(manager)
    log.Printf("Acceptance tests ready: %d scenarios on the accounts %s",
        len(acceptance.Definitions()), acceptance.AccountList())
}

// newAcceptanceManager builds the acceptance manager from the shared
// dependencies. The headless CLI path and the live web UI path share
// the same construction so the scenarios see the same wiring either
// way.
func newAcceptanceManager(
    registry *state.Registry, cfg config,
    engine *pathfind.Engine, mesh *navmesh.Mesh, proxyServer *proxy.Server,
) *acceptance.Manager {
    return acceptance.NewManager(acceptance.ManagerDeps{
        Registry: registry,
        Login:    cfg.loginAddress,
        Engine:   engine,
        Mesh:     mesh,
        Proxy:    proxyServer,
        Logger:   log.Default(),
        DBConfig: acceptance.DefaultDBConfig(),
        LogDir:   acceptanceLogDir(cfg),
    }, acceptance.Definitions())
}

// acceptanceLogDir resolves the directory of the per-run acceptance
// bot logs: an explicit -acceptance-log-dir wins ("off" disables the
// run logs), the empty default derives "<session-dir>/acceptance"
// from the journal directory and falls back to "logs/acceptance"
// when the journal itself is off (the run logs stay on - they are
// the feedback loop of the acceptance suite).
func acceptanceLogDir(cfg config) string {
    switch {
    case cfg.acceptanceLogDir == "off":
        return ""
    case cfg.acceptanceLogDir != "":
        return cfg.acceptanceLogDir
    case cfg.sessionDir != "":
        return filepath.Join(cfg.sessionDir, "acceptance")
    default:
        return "logs" + string(os.PathSeparator) + "acceptance"
    }
}

// runHuntAuditCLI measures the live hunting ground geometry: the probe
// account visits every cell focus through the database position
// injection, and the evidence file collects the attackable npc
// population of every ground with the polygon leash verdicts. The
// stack must be up (login 2106, game 7777, MariaDB 3306); the run
// resumes from the evidence file (-audit-fresh starts over) and exits
// 0 once every spot of the filter measured. The registry regeneration
// builds on this measurement (tools/generate_hunt_cells.py).
func runHuntAuditCLI(cfg config) {
    log.Println("Starting swarm hunt audit CLI")
    log.Printf("Build: %s", version.Identity())

    account := cfg.auditAccount
    // The signal context stops the audit on SIGINT/SIGTERM. The stop
    // call lands on the explicit cleanup path below (no defer) because
    // os.Exit skips the deferred calls - the same shape the acceptance
    // CLI uses.
    ctx, stop := signal.NotifyContext(context.Background(),
        syscall.SIGINT, syscall.SIGTERM)
    err := huntaudit.Run(ctx, huntaudit.Config{
        Login:    cfg.loginAddress,
        Account:  account,
        Password: account,
        Char:     account,
        DB:       acceptance.DefaultDBConfig(),
        Wait:     cfg.auditWait,
        Output:   cfg.huntAudit,
        Anchors:  cfg.auditAnchors,
        Filter:   cfg.auditFilter,
        Stride:   cfg.auditStride,
        Fresh:    cfg.auditFresh,
    }, log.Default())
    stop()
    if err != nil {
        log.Printf("Hunt audit: FAIL %s", err.Error())
        os.Exit(1)
    }
    log.Println("Hunt audit: PASS")
}

// runAcceptanceCLI drives the acceptance scenarios headless: no fleet
// supervisor, only the acceptance manager. "list" prints the ids and
// exits, "all" runs every scenario in definition order, "all-parallel"
// launches them at once and collects every failure, and an id runs
// just that one; the exit code is 0 on a pass and 1 on a fail, so an
// agent or a CI gate drives the suite without the web UI. `-web
// address` watches the run from the UI (`-web ""` keeps it silent);
// the proxy relay scenario arms its own ephemeral proxy.
func runAcceptanceCLI(cfg config) {
    log.Println("Starting swarm acceptance CLI")
    log.Printf("Build: %s", version.Identity())

    if cfg.acceptanceRun == "list" {
        for _, id := range acceptance.DefinitionsIDs() {
            log.Println("  " + id)
        }

        return
    }

    registry := state.NewRegistry()
    var proxyServer *proxy.Server
    if cfg.proxy {
        proxyServer = startProxy(cfg)
    }
    var engine *pathfind.Engine
    dir := cfg.geodataDir
    if dir == "" {
        dir = detectGeodataDir()
    }
    engine = pathfind.NewEngine(dir)
    engine.SetMaxPassableHeight(uint16(cfg.maxPassable))
    mesh := loadNavmesh(cfg)

    manager := newAcceptanceManager(registry, cfg, engine, mesh,
        proxyServer)
    web := startWebInterface(cfg, registry, nil, proxyServer)
    if web != nil {
        web.SetAcceptance(manager)
    }

    // The signal context stops the manager on SIGINT/SIGTERM. The
    // stop call lands on the explicit cleanup path below (no defer)
    // because os.Exit skips the deferred calls - the shutdown chain
    // stays straight on both the pass and the fail path.
    ctx, stop := signal.NotifyContext(context.Background(),
        syscall.SIGINT, syscall.SIGTERM)

    var err error
    switch cfg.acceptanceRun {
    case "all":
        log.Printf("Acceptance: running %d scenarios sequentially",
            len(manager.IDs()))
        err = manager.RunAll(ctx)
    case "all-parallel":
        log.Printf("Acceptance: running %d scenarios in parallel",
            len(manager.IDs()))
        err = manager.RunAllParallel(ctx)
    default:
        log.Printf("Acceptance: running scenario %s",
            cfg.acceptanceRun)
        err = manager.Run(ctx, cfg.acceptanceRun)
    }
    stop()
    shutdownWebInterface(web)
    shutdownProxy(proxyServer)
    if err != nil {
        log.Printf("Acceptance: FAIL %s", err.Error())
        os.Exit(1)
    }
    log.Println("Acceptance: PASS")
}
