// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "log"
    "os"
    "strings"

    "github.com/melg8/swarm/internal/swarm/session"
)

// runSessionReportCLI renders the session report of a journal file to
// stdout: the post-mortem path of a finished or crashed run. With
// -account explicitly set, only that bot renders; otherwise every bot
// of the file renders in first-seen order (the default account of the
// bot modes is a login name, not a journal filter). The -from/-to
// window bounds the records folded into the aggregates (the time-boxed
// report of one interesting stretch of a long session).
func runSessionReportCLI(cfg config) {
    parsed, err := session.ParseJournalFileWindow(
        cfg.sessionReport, cfg.queryFrom, cfg.queryTo)
    if err != nil {
        log.Fatalf("Session report: %v", err)
    }
    bots := parsed.Order
    if explicitAccount() {
        bots = []string{cfg.account}
    }
    for _, bot := range bots {
        report, err := parsed.Report(bot)
        if err != nil {
            log.Fatalf("Session report: %v", err)
        }
        if _, err := os.Stdout.WriteString(report); err != nil {
            log.Fatalf("Session report write: %v", err)
        }
    }
}

// runSessionQueryCLI streams the filtered records of a journal file to
// stdout: the drill-down tool every anomaly finding points at.
func runSessionQueryCLI(cfg config) {
    filter := session.QueryFilter{
        Bot:     "",
        Events:  strings.Split(cfg.queryEvents, ","),
        Match:   cfg.queryMatch,
        From:    cfg.queryFrom,
        To:      cfg.queryTo,
        Context: cfg.queryContext,
        Limit:   cfg.queryLimit,
    }
    if cfg.queryEvents == "" {
        filter.Events = nil
    }
    if explicitAccount() {
        filter.Bot = cfg.account
    }
    if err := session.RunQuery(cfg.sessionQuery, filter,
        os.Stdout); err != nil {
        log.Fatalf("Session query: %v", err)
    }
}

// runSessionAnomaliesCLI scans a journal file and prints the ranked
// findings: the first tool of every post-mortem, it names the windows
// the drill-down should isolate.
func runSessionAnomaliesCLI(cfg config) {
    if err := session.RunAnomalies(cfg.sessionAnomalies,
        os.Stdout); err != nil {
        log.Fatalf("Session anomalies: %v", err)
    }
}
