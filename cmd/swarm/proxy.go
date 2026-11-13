// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "context"
    "log"
    "os"
    "strings"
    "time"

    "github.com/melg8/swarm/internal/swarm/proxy"
)

// startProxy builds and runs the client proxy with its own log file so
// the C1 client connection attempts can be diagnosed without digging
// through the console output of the bot (see docs/proxy.md).
func startProxy(cfg config) *proxy.Server {
    file, err := os.OpenFile(cfg.proxyLog,
        os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
    logger := log.New(file, "proxy ", log.LstdFlags|log.Lmicroseconds)
    if err != nil {
        logger = log.Default()
        logger.Printf("Proxy log file %s unavailable: %v",
            cfg.proxyLog, err)
    }
    // The log file stays open for the process lifetime: the OS closes
    // it at exit, no deferred close here (the logger would write into a
    // closed file otherwise).

    server := proxy.NewServer(logger,
        proxy.WithLoginAddresses(splitAddresses(cfg.proxyLogin)...),
        proxy.WithGameAddresses(splitAddresses(cfg.proxyGame)...))
    if err := server.Listen(); err != nil {
        logger.Printf("Proxy failed to start: %v", err)
        log.Printf("Proxy failed to start: %v", err)

        return nil
    }
    go func() {
        if err := server.Serve(); err != nil {
            logger.Printf("Proxy stopped: %v", err)
        }
    }()

    logger.Printf("Proxy started, login bound to %v, game bound to %v, log %s",
        server.LoginAddrs(), server.GameAddrs(), cfg.proxyLog)
    // The routing banner: a C1 client reaching none of the login
    // listeners never appears in this file (the classic exe dials
    // ServerAddr:2106 with the ini [URL] Port line ignored), so the
    // banner names the address every client path lands on.
    logger.Printf("client routing: a C1 client connects to the l2.ini "+
        "ServerAddr on the hardcoded auth port 2106 unless its build "+
        "honors the ini Port; this proxy answers 127.0.0.1 and "+
        "127.0.0.2 on both 2106 and 2107 (bound: %v), the emulated "+
        "login server then hands out the game address %v",
        server.LoginAddrs(), server.GameAddrs())
    log.Printf("Proxy for C1 clients ready: login %v, game %v, client log %s",
        server.LoginAddrs(), server.GameAddrs(), cfg.proxyLog)

    return server
}

func splitAddresses(value string) []string {
    addresses := strings.Split(value, ",")
    cleaned := make([]string, 0, len(addresses))
    for _, address := range addresses {
        address = strings.TrimSpace(address)
        if address != "" {
            cleaned = append(cleaned, address)
        }
    }

    return cleaned
}

func shutdownProxy(server *proxy.Server) {
    if server == nil {
        return
    }
    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()
    if err := server.Shutdown(ctx); err != nil {
        log.Printf("Proxy shutdown failed: %v", err)
    }
}
