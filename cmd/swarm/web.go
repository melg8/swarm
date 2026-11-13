// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "context"
    "errors"
    "log"
    "net/http"
    "time"

    "github.com/melg8/swarm/internal/swarm/hunt"
    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/proxy"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/melg8/swarm/internal/swarm/webserver"
)

// startWebInterface runs the web server in the background when enabled.
// A non nil pathfind engine switches the server into the pathfind test
// mode, otherwise the bot registry is served.
func startWebInterface(
    cfg config, registry *state.Registry, engine *pathfind.Engine,
    proxyServer *proxy.Server,
) *webserver.Server {
    if cfg.webAddress == "" {
        return nil
    }
    var server *webserver.Server
    if engine != nil {
        // The test opens on the hunting area: that is the terrain the
        // bot actually walks and the most useful pathfind playground.
        zoneX, zoneY, _ := hunt.DefaultHuntingZone()
        server = webserver.NewPathfindServer(engine, cfg.webAddress,
            log.Default(), webserver.PathfindOptions{
                ViewCenter: &pathfind.Vec3{
                    X: float64(zoneX),
                    Y: float64(zoneY),
                    Z: 0,
                },
            })
    } else {
        server = webserver.NewServer(registry, cfg.webAddress, log.Default())
        if proxyServer != nil {
            server.SetProxy(proxyServer)
        }
    }
    go func() {
        if err := server.ListenAndServe(); err != nil {
            if !errors.Is(err, http.ErrServerClosed) {
                log.Printf("Web interface stopped: %v", err)
            }
        }
    }()

    return server
}

func shutdownWebInterface(server *webserver.Server) {
    if server == nil {
        return
    }
    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()
    if err := server.Shutdown(ctx); err != nil {
        log.Printf("Web interface shutdown failed: %v", err)
    }
}
