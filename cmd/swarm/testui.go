// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "context"
    "log"
    "os/signal"
    "syscall"

    "github.com/melg8/swarm/internal/swarm/webserver"
)

// runTestFightUI serves the bot less fight FX comparison gallery: the
// web interface answers with the static variant grid until the process
// is stopped. No game connection and no geodata is needed - the map
// background of the gallery cells is the static tile pyramid shipped
// with the web content.
func runTestFightUI(cfg config) {
    if cfg.webAddress == "" {
        log.Println("Fight FX test needs the web interface, " +
            "pass a -web address")

        return
    }

    web := webserver.NewTestFightServer(cfg.webAddress, log.Default())
    go func() {
        if err := web.ListenAndServe(); err != nil {
            log.Println("Web interface failed: " + err.Error())
        }
    }()

    log.Println("Fight FX test UI is ready")

    ctx, stop := signal.NotifyContext(context.Background(),
        syscall.SIGINT, syscall.SIGTERM)
    defer stop()
    <-ctx.Done()
    shutdownWebInterface(web)
    log.Println("Fight FX test finished")
}

// runTestPathUI serves the bot less walk plan visualization gallery:
// the web interface answers with the animated variant grid until the
// process is stopped. No game connection and no geodata is needed -
// the map background of the gallery cells is the static tile pyramid
// shipped with the web content.
func runTestPathUI(cfg config) {
    if cfg.webAddress == "" {
        log.Println("Path gallery needs the web interface, " +
            "pass a -web address")

        return
    }

    web := webserver.NewTestPathServer(cfg.webAddress, log.Default())
    go func() {
        if err := web.ListenAndServe(); err != nil {
            log.Println("Web interface failed: " + err.Error())
        }
    }()

    log.Println("Walk plan gallery is ready")

    ctx, stop := signal.NotifyContext(context.Background(),
        syscall.SIGINT, syscall.SIGTERM)
    defer stop()
    <-ctx.Done()
    shutdownWebInterface(web)
    log.Println("Walk plan gallery finished")
}

// runTestPathV2UI serves the bot less smooth curve animation lab: the
// web interface answers with the thirty variant grid until the process
// is stopped. No game connection and no geodata is needed - the map
// background of the lab cells is the static tile pyramid shipped with
// the web content.
func runTestPathV2UI(cfg config) {
    if cfg.webAddress == "" {
        log.Println("Curve lab needs the web interface, " +
            "pass a -web address")

        return
    }

    web := webserver.NewTestPathCurveServer(cfg.webAddress,
        log.Default())
    go func() {
        if err := web.ListenAndServe(); err != nil {
            log.Println("Web interface failed: " + err.Error())
        }
    }()

    log.Println("Smooth curve lab is ready")

    ctx, stop := signal.NotifyContext(context.Background(),
        syscall.SIGINT, syscall.SIGTERM)
    defer stop()
    <-ctx.Done()
    shutdownWebInterface(web)
    log.Println("Smooth curve lab finished")
}

// runTestPathV3UI serves the bot less flow and snake speed lab: the
// web interface answers with the forty variant grid until the process
// is stopped. No game connection and no geodata is needed - the map
// background of the lab cells is the static tile pyramid shipped with
// the web content.
func runTestPathV3UI(cfg config) {
    if cfg.webAddress == "" {
        log.Println("Flow lab needs the web interface, " +
            "pass a -web address")

        return
    }

    web := webserver.NewTestPathFlowServer(cfg.webAddress,
        log.Default())
    go func() {
        if err := web.ListenAndServe(); err != nil {
            log.Println("Web interface failed: " + err.Error())
        }
    }()

    log.Println("Flow lab is ready")

    ctx, stop := signal.NotifyContext(context.Background(),
        syscall.SIGINT, syscall.SIGTERM)
    defer stop()
    <-ctx.Done()
    shutdownWebInterface(web)
    log.Println("Flow lab finished")
}

// runTestFightUIV1 serves the combat animation variant showcase (the
// v1 idea set): no game connection and no bot, the web interface plays
// a looping hero versus enemy demo fight through every damage
// visualization idea (four enemy positions vertically, the variants
// horizontally with a scroll bar, the map tiles as the background)
// until the process is stopped. The user compares the numbered
// variants in the browser and picks the winner for the live map
// implementation.
func runTestFightUIV1(cfg config) {
    if cfg.webAddress == "" {
        log.Println("Fight test UI needs the web interface, " +
            "pass a -web address")

        return
    }

    server := webserver.NewFightServer(cfg.webAddress, log.Default())
    go func() {
        if err := server.ListenAndServe(); err != nil {
            log.Printf("Web interface stopped: %v", err)
        }
    }()

    log.Println("Fight test UI is ready")

    ctx, stop := signal.NotifyContext(context.Background(),
        syscall.SIGINT, syscall.SIGTERM)
    defer stop()
    <-ctx.Done()
    shutdownWebInterface(server)
    log.Println("Fight test finished")
}
