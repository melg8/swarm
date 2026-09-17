// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "context"
    "errors"
    "flag"
    "fmt"
    "io"
    "log"
    "net"
    "net/http"
    "os"
    "os/signal"
    "path/filepath"
    "strconv"
    "strings"
    "sync"
    "syscall"
    "time"

    "github.com/melg8/swarm/internal/swarm/acceptance"
    "github.com/melg8/swarm/internal/swarm/connection"
    "github.com/melg8/swarm/internal/swarm/hunt"
    "github.com/melg8/swarm/internal/swarm/huntaudit"
    "github.com/melg8/swarm/internal/swarm/memwatch"
    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
    "github.com/melg8/swarm/internal/swarm/proxy"
    "github.com/melg8/swarm/internal/swarm/session"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/melg8/swarm/internal/swarm/webserver"
    "github.com/melg8/swarm/internal/version"
)

// Default configuration values.
const (
    defaultLoginAddress = "127.0.0.1:2106"
    defaultAccount      = "test1"
    defaultPassword     = "test"
    defaultCharName     = "test1"
    defaultWebAddress   = "127.0.0.1:8080"
    connectTimeout      = 10 * time.Second
    defaultProxyLogPath = "proxy.log"
)

// Candidate geodata directories, checked in order when -geodata is empty:
// the relative server layouts first (the sandbox checkout next to the swarm
// root keeps the server tree at l2j_mobius/L2J_Mobius_C1_HarbingersOfWar,
// the reference Windows deployment runs the server straight from its
// L2J_Mobius_C1_HarbingersOfWar/game folder, see AGENTS.md), then the bot
// tree itself.
var defaultGeodataCandidates = []string{
    filepath.Join("data", "geodata"),
    filepath.Join("..", "l2j_mobius", "L2J_Mobius_C1_HarbingersOfWar",
        "dist", "game", "data", "geodata"),
    filepath.Join("L2J_Mobius_C1_HarbingersOfWar", "game", "data", "geodata"),
    filepath.Join("E:\\", "work", "lineage_workspace_fresh",
        "L2J_Mobius_C1_HarbingersOfWar", "game", "data", "geodata"),
}

// Candidate navmesh tile directories, checked in order when -navmesh
// is empty: the tiles are the offline output of cmd/navmesh-build and
// live in the bot tree (data/navmesh is the documented default of the
// build command), never in the server tree.
var defaultNavmeshCandidates = []string{
    filepath.Join("data", "navmesh"),
}

// Reconnect backoff of the 24/7 supervisor: a lost session is retried
// with a growing pause, a long lived session resets the pause so a drop
// after hours reconnects immediately.
const (
    reconnectMinDelay = 2 * time.Second
    reconnectMaxDelay = 30 * time.Second
    // A session shorter than this counts as a failed attempt and grows
    // the backoff; a longer one resets it.
    stableSessionTime = time.Minute
)

// Character creation constants for the elven fighter.
const (
    elfRaceID     = 1
    elfFighterID  = 18
    male          = 0
    defaultHair   = 0
    defaultFace   = 0
    characterSlot = 0
)

type config struct {
    loginAddress  string
    account       string
    password      string
    charName      string
    webAddress    string
    hunt          bool
    pathfindTest  bool
    testFightUI   bool
    testFightUIV1 bool
    geodataDir    string
    navmeshDir    string
    // navmeshShow is the -show-navmesh flag: the 3D mesh viewer mode
    // over the navmesh tile directory. A bare -show-navmesh loads
    // every tile stitched together, -show-navmesh=21_19 (comma
    // separated col_row keys) opens the named tiles only; the route
    // queries always run over the full directory mesh.
    navmeshShow navmeshShowFlag
    maxPassable uint
    proxy       bool
    proxyLogin  string
    proxyGame   string
    proxyLog    string
    bots        int
    // acceptanceRun selects the headless acceptance test mode: the
    // process launches no fleet bot supervisor, just the acceptance
    // manager and the requested scenario. "list" prints the available
    // scenario ids and exits; "all" runs every scenario in order, a
    // specific id runs just that one. The exit code reflects the
    // outcome (0 for a pass, 1 for a fail).
    acceptanceRun string
    // sessionDir is the directory of the persistent session journal
    // ("logs" by default, empty disables the journal entirely).
    sessionDir string
    // sessionReport renders the compact session report of a journal
    // file to stdout and exits (the post-mortem path: the run may be
    // long over, the journal file carries the story).
    sessionReport string
    // sessionQuery streams the records of a journal file through the
    // drill-down filters below and exits: the grep of the journal.
    sessionQuery string
    // sessionAnomalies scans a journal file for the ranked behavior
    // findings of a long run and exits: the "where to look" tool
    // every deep analysis starts from.
    sessionAnomalies string
    // queryFrom and queryTo bound the record window of the query and
    // the report: RFC3339 timestamps or bare 15:04 clocks resolved
    // against the first record date.
    queryFrom string
    queryTo   string
    // queryEvents restricts the query to a comma separated list of
    // journal event kinds (kill, death, story, ...).
    queryEvents string
    // queryMatch is a regular expression over the story text and the
    // reason fields of the query.
    queryMatch string
    // queryContext prints the records within this duration around
    // every query match.
    queryContext time.Duration
    // queryLimit caps the printed query records.
    queryLimit int
    // huntAudit runs the live hunting ground audit and exits: the
    // probe character visits every cell focus of the registry and
    // the JSON evidence file collects what it actually sees.
    huntAudit string
    // auditWait is the knownlist settle window of every audit visit.
    auditWait time.Duration
    // auditAccount names the probe account of the audit (the password
    // equals the account name, the character shares it).
    auditAccount string
    // auditAnchors optionally overrides the audited positions per
    // spot id (the verification pass of the regenerated geometry).
    auditAnchors string
    // auditFilter audits only the spots whose id contains one of the
    // comma separated substrings.
    auditFilter string
    // auditStride audits every Nth spot (0 or 1 audits every spot):
    // the stratified sampling of a verification pass.
    auditStride int
    // auditFresh drops the resume state of the audit evidence file.
    auditFresh bool
}

func parseFlags() config {
    cfg := config{
        loginAddress:     "",
        account:          "",
        password:         "",
        charName:         "",
        webAddress:       "",
        hunt:             false,
        pathfindTest:     false,
        testFightUI:      false,
        testFightUIV1:    false,
        geodataDir:       "",
        navmeshDir:       "",
        navmeshShow:      navmeshShowFlag{tiles: nil, enabled: false},
        maxPassable:      uint(pathfind.DefaultMaxPassableHeight),
        proxy:            false,
        proxyLogin:       "",
        proxyGame:        "",
        proxyLog:         "",
        bots:             1,
        acceptanceRun:    "",
        sessionDir:       "",
        sessionReport:    "",
        sessionQuery:     "",
        sessionAnomalies: "",
        queryFrom:        "",
        queryTo:          "",
        queryEvents:      "",
        queryMatch:       "",
        queryContext:     0,
        queryLimit:       0,
        huntAudit:        "",
        auditWait:        0,
        auditAccount:     "",
        auditAnchors:     "",
        auditFilter:      "",
        auditStride:      0,
        auditFresh:       false,
    }
    flag.StringVar(&cfg.loginAddress, "login", defaultLoginAddress,
        "login server address")
    flag.StringVar(&cfg.account, "account", defaultAccount, "account name")
    flag.StringVar(&cfg.password, "password", defaultPassword, "password")
    flag.StringVar(&cfg.charName, "char", defaultCharName, "character name")
    flag.StringVar(&cfg.webAddress, "web", defaultWebAddress,
        "web interface address, empty disables it")
    flag.BoolVar(&cfg.hunt, "hunt", false,
        "auto hunt: attack, pick up loot and manage inventory")
    flag.BoolVar(&cfg.pathfindTest, "pathfind-test", false,
        "map pathfinding test UI instead of the bot: no game connection, "+
            "draggable start and end markers show the found path")
    flag.BoolVar(&cfg.testFightUI, "test-fight-ui", false,
        "fight FX comparison gallery instead of the bot: no game "+
            "connection, a horizontal grid of numbered damage "+
            "visualization variants, each shown with the enemy "+
            "above, below, left and right of the character")
    flag.BoolVar(&cfg.testFightUIV1, "test-fight-ui-v1", false,
        "combat animation variant showcase UI (v1 idea set) instead of the "+
            "bot: no game connection, a looping hero versus enemy demo fight "+
            "plays every damage visualization idea side by side for picking one")
    flag.StringVar(&cfg.geodataDir, "geodata", "",
        "geodata directory with X_Y.l2j region files for the pathfind "+
            "test (auto detected when empty)")
    flag.StringVar(&cfg.navmeshDir, "navmesh", "",
        "navmesh tile directory with X_Y.nm files (the "+
            "cmd/navmesh-build output); the hunt routes serve "+
            "from the mesh when the tiles exist, empty "+
            "autodetects data/navmesh")
    flag.Var(&cfg.navmeshShow, "show-navmesh",
        "serve the bot less 3D navmesh viewer instead of the bot: "+
            "a bare -show-navmesh loads every tile of the "+
            "-navmesh directory stitched together, "+
            "-show-navmesh=21_19 (comma separated) opens the named "+
            "tiles only; double click two mesh points in the browser "+
            "to run the corridor search with its construction timer")
    registerProxyFlags(&cfg)
    flag.UintVar(&cfg.maxPassable, "max-passable",
        uint(pathfind.DefaultMaxPassableHeight),
        "maximum walkable height difference between neighbouring cells")
    flag.IntVar(&cfg.bots, "bots", 1,
        "number of concurrent bot sessions to launch in one process. "+
            "When greater than 1, the bots share one web interface "+
            "(the sidebar lists every bot) and one proxy (the web UI "+
            "selects which bot a connecting C1 client attaches to). "+
            "The account and char name of -account/-char become the "+
            "base: bot 1 keeps them as-is, bot 2 appends '2', bot 3 "+
            "'3' and so on (test1, test2, test3...). All bots are "+
            "elven fighters, all share the same -login, -hunt, "+
            "-geodata and proxy settings. The server auto-creates "+
            "missing accounts, so the first run of -bots 3 makes "+
            "test1, test2, test3 on the fly.")
    flag.StringVar(&cfg.acceptanceRun, "acceptance", "",
        "run an acceptance scenario headless instead of the bot: "+
            "the value is a scenario id (soak, farm-readiness, "+
            "bot-lifetime, proxy-relay) or 'all' to run every "+
            "scenario in definition order, or 'list' to print "+
            "the available ids and exit. No fleet bot supervisor "+
            "runs; the acceptance manager launches the temp bot "+
            "of the scenario, runs it and exits. The soak scenario "+
            "reads SWARM_SOAK_MINUTES (default 10, the M1 proof "+
            "sets 480) for the window length. Pass an empty "+
            "-web to keep the UI off; the result prints to the "+
            "log. Exit code: 0 for a pass, 1 for a fail.")
    flag.StringVar(&cfg.sessionDir, "session-dir", "logs",
        "directory of the persistent session journal (the JSONL "+
            "record of every event since the application start, "+
            "rotated and gzipped; the web UI session dump button "+
            "renders its report). Empty disables the journal")
    flag.StringVar(&cfg.sessionReport, "session-report", "",
        "render the compact session report of a journal file to "+
            "stdout and exit (the post-mortem analysis of a "+
            "finished or crashed run; plain and gzipped journal "+
            "segments both parse). With -account set, only that "+
            "bot renders. -from/-to bound the records folded "+
            "into the report")
    flag.StringVar(&cfg.sessionQuery, "session-query", "",
        "stream the records of a journal file through the drill-"+
            "down filters and exit: the grep of the journal "+
            "without reading the whole session. Filters: "+
            "-account, -events, -match, -from, -to, -context, "+
            "-limit")
    flag.StringVar(&cfg.sessionAnomalies, "session-anomalies", "",
        "scan a journal file for the ranked behavior findings of "+
            "a long run and exit: emergency logout loops, "+
            "repeated decision lines, trip abort loops, fight "+
            "duration outliers, death streaks, stalls. Every "+
            "finding carries its own -session-query drill-down")
    flag.StringVar(&cfg.huntAudit, "hunt-audit", "",
        "run the live hunting ground audit instead of the bot: the probe "+
            "character is injected at every cell focus of the registry "+
            "(the database position rewrite), waits out the knownlist and "+
            "the JSON evidence file collects every attackable npc it sees "+
            "with the polygon leash verdict. The run resumes: cells "+
            "already measured in the file are skipped, so a long registry "+
            "audits across several foreground runs")
    flag.DurationVar(&cfg.auditWait, "audit-wait", 12*time.Second,
        "knownlist settle window of every cell visit of -hunt-audit")
    flag.StringVar(&cfg.auditAccount, "audit-account", "huntaudit",
        "probe account of -hunt-audit (the password equals the name, "+
            "the character shares it)")
    flag.StringVar(&cfg.auditAnchors, "audit-anchors", "",
        "JSON file with per cell position overrides of -hunt-audit "+
            "(the verification pass of the regenerated geometry)")
    flag.StringVar(&cfg.auditFilter, "audit-filter", "",
        "audit only the cells whose id contains one of the comma "+
            "separated substrings")
    flag.IntVar(&cfg.auditStride, "audit-stride", 0,
        "audit every Nth cell of -hunt-audit (0 or 1 audits every "+
            "cell): the stratified sampling of a verification pass")
    flag.BoolVar(&cfg.auditFresh, "audit-fresh", false,
        "re-measure every cell of -hunt-audit, ignoring the resume state")
    flag.StringVar(&cfg.queryFrom, "from", "",
        "window start of -session-query/-session-report: RFC3339 "+
            "or a bare 15:04 clock of the session day")
    flag.StringVar(&cfg.queryTo, "to", "",
        "window end of -session-query/-session-report: RFC3339 "+
            "or a bare 15:04 clock of the session day")
    flag.StringVar(&cfg.queryEvents, "events", "",
        "comma separated event kinds of -session-query (story, "+
            "kill, death, stall, trip-start, ...)")
    flag.StringVar(&cfg.queryMatch, "match", "",
        "regular expression over the text and reason fields of "+
            "-session-query")
    flag.DurationVar(&cfg.queryContext, "context", 0,
        "print the records within this duration around every "+
            "-session-query match")
    flag.IntVar(&cfg.queryLimit, "limit", 0,
        "cap on the printed -session-query records (0 keeps "+
            "everything)")
    flag.Parse()

    return cfg
}

// registerProxyFlags declares the client proxy flags. The listener
// defaults answer the hardcoded auth port 2106 of the classic C1 exe on
// both loopback addresses (see proxy.DefaultLoginAddresses).
func registerProxyFlags(cfg *config) {
    flag.BoolVar(&cfg.proxy, "proxy", false,
        "run the MITM proxy for real C1 clients: an emulated login "+
            "server answering 2106 and 2107 on 127.0.0.1 and 127.0.0.2 "+
            "(the classic C1 exe hardcodes the auth port 2106, the ini "+
            "[URL] Port line is ignored by it) and an emulated game "+
            "server on 127.0.0.1:7778 that attach a connecting client to "+
            "the live bot session (any login/password pair is accepted, "+
            "the char list shows the selected bot)")
    flag.StringVar(&cfg.proxyLogin, "proxy-login",
        strings.Join(proxy.DefaultLoginAddresses(), ","),
        "comma separated login listen addresses of the proxy (the first "+
            "is mandatory, the rest are optional fallbacks: 127.0.0.1:2106 "+
            "intercepts hardcoded-port clients, the 127.0.0.2 pair answers "+
            "ServerAddr=127.0.0.2 variants)")
    flag.StringVar(&cfg.proxyGame,
        "proxy-game", strings.Join(proxy.DefaultGameAddresses(), ","),
        "comma separated game listen addresses of the proxy")
    flag.StringVar(&cfg.proxyLog, "proxy-log", defaultProxyLogPath,
        "file the proxy writes its client connection log to")
}

// swarmDialer dials the login and game server connections with the
// connect timeout.
// Listing the deprecated Dialer fields explicitly would trip
// staticcheck SA1019, so the struct stays partial.
//
//nolint:exhaustruct_v5 // net.Dialer carries deprecated fields (see above)
var swarmDialer = &net.Dialer{Timeout: connectTimeout}

// connectLoginServer establishes the login server connection.
func connectLoginServer(address string) (net.Conn, error) {
    conn, err := swarmDialer.Dial("tcp", address)
    if err != nil {
        return nil, fmt.Errorf(
            "failed to connect to login server: %w", err)
    }
    log.Println("Connected to login server at " + address)

    return conn, nil
}

// connectGameServer establishes the game server connection from the auth
// result.
func connectGameServer(auth *connection.AuthResult) (net.Conn, error) {
    address := fmt.Sprintf("%d.%d.%d.%d:%d",
        auth.ServerIP[0], auth.ServerIP[1], auth.ServerIP[2], auth.ServerIP[3],
        auth.ServerPort)
    conn, err := swarmDialer.Dial("tcp", address)
    if err != nil {
        return nil, fmt.Errorf(
            "failed to connect to game server: %w", err)
    }
    log.Println("Connected to game server at " + address)

    return conn, nil
}

// runBot performs one bot session: login, game handshake, authentication,
// character creation, entering the world and staying in it until the
// context is done or the session fails. The hunt loop of the session is
// bound to a derived context so it stops with the session. The optional
// geodata engine serves the town trips of the hunt loop.
func runBot( //nolint:funlen // linear session script
    ctx context.Context, cfg config, tracker *state.Bot,
    engine *pathfind.Engine, mesh *navmesh.Mesh,
    proxyServer *proxy.Server, journal *session.Journal,
) error {
    sessionCtx, cancelSession := context.WithCancel(ctx)
    defer cancelSession()

    tracker.ResetSession()

    loginConn, err := connectLoginServer(cfg.loginAddress)
    if err != nil {
        return err
    }

    auth, err := connection.Authenticate(loginConn, cfg.account, cfg.password)
    if err != nil {
        return fmt.Errorf("failed to authenticate: %w", err)
    }

    // The emulated login server of the proxy mirrors the Init packet of
    // the real one, so its scrambled RSA modulus is published here.
    if proxyServer != nil {
        proxyServer.SetRsaModulus(auth.RsaPublicKey)
    }

    gameConn, err := connectGameServer(auth)
    if err != nil {
        return err
    }

    game, err := connection.NewGameClient(gameConn)
    if err != nil {
        return fmt.Errorf("game handshake failed: %w", err)
    }
    game.SetTracker(tracker)

    // The proxy observes the whole session (the recorder replays it to
    // connecting C1 clients) and forwards their packets through the
    // shared outbound cipher of the session. The registration must
    // happen before the session starts reading so nothing is missed.
    var sessionRecorder *proxy.Recorder
    if proxyServer != nil {
        sessionRecorder = proxyServer.RegisterSession(cfg.account, game, tracker)
        game.SetTap(sessionRecorder.Record)
        defer proxyServer.UnregisterSession(cfg.account, sessionRecorder)
    }

    charList, err := game.Authenticate(connection.GameSessionParams{
        Account:    auth.Account,
        LoginOkID1: auth.LoginOkID1,
        LoginOkID2: auth.LoginOkID2,
        PlayOkID1:  auth.PlayOkID1,
        PlayOkID2:  auth.PlayOkID2,
    })
    if err != nil {
        return fmt.Errorf("game authentication failed: %w", err)
    }

    charList, err = game.EnsureCharacter(connection.CharacterParams{
        Name:      cfg.charName,
        Race:      elfRaceID,
        Female:    male,
        ClassID:   elfFighterID,
        HairStyle: defaultHair,
        HairColor: defaultHair,
        Face:      defaultFace,
    }, charList)
    if err != nil {
        return fmt.Errorf("failed to prepare character: %w", err)
    }

    slot, charInfo, found := charList.FindCharacterByName(cfg.charName)
    if !found {
        return fmt.Errorf("character %s not found", cfg.charName)
    }
    log.Printf("Playing character %s of level %d",
        charInfo.Name, charInfo.Level)

    if err := game.EnterWorld(int32(slot)); err != nil {
        return fmt.Errorf("failed to enter world: %w", err)
    }
    log.Println("Character " + cfg.charName + " entered the world")
    if journal != nil {
        journal.Connect(cfg.account, "entered",
            "level "+strconv.Itoa(int(charInfo.Level)))
    }

    // The loop always runs: with -hunt it hunts autonomously, without
    // it stays in the manual mode and only executes the commands of the
    // web UI (map clicks, equipment drags) so the interface stays
    // interactive in both launch modes.
    loop := hunt.NewLoop(game, tracker)
    // The hunt decisions mirror into the tracker event log: the web UI
    // log tab and the state dump button then carry the reasoning of
    // the loop (zone switches, escapes, stuck re-paths) next to the
    // raw game events - the debugging material of the live sessions.
    loop.SetLogger(huntEventLogger(tracker))
    loop.SetJournal(journal)
    installNavigator(loop, engine, mesh, cfg.hunt)
    if cfg.hunt {
        loop.SetHuntingZoneRegion("elven")
    } else {
        loop.SetAutonomy(false)
    }
    go loop.Run(sessionCtx)

    return game.Run(sessionCtx, cfg.charName)
}

// installNavigator binds the path planning behind the Navigator
// seam of the hunt loop: the navmesh hybrid when the tile mesh
// exists (the live integration round of docs/navmesh.md - the long
// routes serve from the mesh corridor search, the grid engine stays
// the click validation and the fallback authority), the pure grid
// engine navigator otherwise. Without geodata the loop hunts without
// the town trips at all.
func installNavigator(
    loop *hunt.Loop, engine *pathfind.Engine, mesh *navmesh.Mesh,
    huntMode bool,
) {
    switch {
    case engine != nil && mesh != nil:
        loop.SetNavigator(hunt.NewNavmeshNavigator(engine, mesh))
    case engine != nil:
        loop.SetNavigator(hunt.NewNavigator(engine))
    case huntMode:
        log.Println("Hunt runs without town trips: no geodata available")
    }
}

// huntEventLogger builds the logger of the hunt loop: the console copy
// keeps the standard format, and every hunt decision line mirrors into
// the tracker event log (stripped of the timestamp prefix the console
// format adds). The mirrored lines then stream to the web UI log tab
// and travel inside the state dump.
func huntEventLogger(tracker *state.Bot) *log.Logger {
    mirror := huntEventMirror{tracker: tracker}

    return log.New(io.MultiWriter(os.Stdout, mirror), "", log.LstdFlags)
}

// openSessionJournal opens the persistent session journal when the
// mode is on (-session-dir defaults to "logs", the empty value
// disables it). The journal carries the process identity line and
// logs its path so the user knows what to attach to a report.
func openSessionJournal(cfg config) *session.Journal {
    if cfg.sessionDir == "" {
        return nil
    }
    journal, err := session.NewJournal(cfg.sessionDir, log.Default())
    if err != nil {
        log.Printf("Session journal unavailable (continuing without): %v",
            err)

        return nil
    }
    journal.Build(version.Identity())
    log.Printf("Session journal: %s", journal.Path())

    return journal
}

// wireSessionBot connects one tracker to the journal: the event story
// mirror (every recorded tracker event lands in the journal file) and
// the periodic state sampler of the quantitative trail.
func wireSessionBot(
    ctx context.Context, journal *session.Journal, tracker *state.Bot,
) {
    if journal == nil {
        return
    }
    botID := tracker.ID()
    tracker.SetEventSink(func(at time.Time, message string) {
        journal.Story(botID, message, at)
    })
    go session.NewSampler(botID, tracker, journal).Run(ctx)
}

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

// explicitAccount reports whether the -account flag was passed on the
// command line: the offline journal tools treat the bot filter as
// opt-in, so the default login account of the bot modes never filters
// a journal whose bots carry other names.
func explicitAccount() bool {
    set := false
    flag.Visit(func(f *flag.Flag) {
        if f.Name == "account" {
            set = true
        }
    })

    return set
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
    if err := session.RunQuery(cfg.sessionQuery, filter, os.Stdout); err != nil {
        log.Fatalf("Session query: %v", err)
    }
}

// runSessionAnomaliesCLI scans a journal file and prints the ranked
// findings: the first tool of every post-mortem, it names the windows
// the drill-down should isolate.
func runSessionAnomaliesCLI(cfg config) {
    if err := session.RunAnomalies(cfg.sessionAnomalies, os.Stdout); err != nil {
        log.Fatalf("Session anomalies: %v", err)
    }
}

// huntEventMirror writes hunt log lines into the bot event log.
type huntEventMirror struct {
    tracker *state.Bot
}

// Write implements io.Writer for the log package: one call carries one
// complete line.
func (m huntEventMirror) Write(p []byte) (int, error) {
    line := strings.TrimSpace(string(p))
    // The console prefix (date time) stays console only: the event log
    // carries its own timestamps.
    if at := strings.Index(line, "Hunt: "); at >= 0 {
        line = line[at:]
    }
    if line != "" {
        m.tracker.RecordEvent(line)
    }

    return len(p), nil
}

// startMemoryWatch runs the process memory logger (one footprint
// line a minute, see internal/swarm/memwatch) until the context ends:
// the long unattended runs demonstrate the memory behavior from the
// log alone, not only from the web statistics chart.
func startMemoryWatch(ctx context.Context) {
    go memwatch.Watch(ctx, log.Default(), memwatch.DefaultPeriod)
}

// runBotForever keeps the bot in the world around the clock: a lost
// session (server restart, kicked connection, network failure) is logged
// and the whole login flow is retried with a growing backoff until the
// user stops the process with SIGINT/SIGTERM. The process never exits on
// its own. The geodata engine and the navmesh mesh survive the
// reconnects.
func runBotForever(
    ctx context.Context, cfg config, tracker *state.Bot, engine *pathfind.Engine,
    mesh *navmesh.Mesh, proxyServer *proxy.Server, journal *session.Journal,
) {
    delay := reconnectMinDelay
    for {
        started := time.Now()
        err := runBot(ctx, cfg, tracker, engine, mesh, proxyServer, journal)
        if ctx.Err() != nil {
            return
        }
        if err != nil {
            // An emergency logout closes the socket itself, so
            // the session error names the close ("use of closed
            // network connection") instead of the pile up that
            // drove it: the tracker carries the honest reason
            // over the boundary and the lost record names it
            // (once - later network drops of the same
            // supervisor report themselves).
            reason := err.Error()
            if honest, ok := tracker.ConsumeEmergencyLogout(); ok {
                reason = "emergency logout: " + honest
            }
            log.Println("Bot failed: " + reason)
            if journal != nil {
                journal.Lost(cfg.account, reason)
            }
        }
        // An emergency logout of the hunt loop armed a login
        // cooldown: honor it on top of the reconnect backoff so the
        // next session starts after the danger window (the mobs
        // reset, the character regenerates) instead of the seconds
        // of the backoff.
        if cooldown := tracker.LoginCooldownRemaining(); cooldown > delay {
            log.Printf("Login cooldown %s holds the reconnect back",
                cooldown)
            delay = cooldown
        }
        if time.Since(started) >= stableSessionTime {
            delay = reconnectMinDelay
        }
        log.Printf("Reconnecting in %s", delay)
        if journal != nil {
            journal.Connect(cfg.account, "reconnect-wait",
                delay.String())
        }
        select {
        case <-ctx.Done():
            return
        case <-time.After(delay):
        }
        delay = min(delay*2, reconnectMaxDelay)
    }
}

func main() {
    cfg := parseFlags()
    log.SetOutput(os.Stdout)

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

    if cfg.bots > 1 {
        runFleet(cfg)

        return
    }

    log.Println("Starting swarm bot for account " + cfg.account)
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
    wireSessionBot(ctx, journal, tracker)
    startMemoryWatch(ctx)

    var proxyServer *proxy.Server
    if cfg.proxy {
        proxyServer = startProxy(cfg)
    }

    // The geodata engine serves the town trips of the hunt and the
    // long manual walks of the web UI (the server side pathfinder
    // refuses far targets), so it loads in every mode.
    var engine *pathfind.Engine
    dir := cfg.geodataDir
    if dir == "" {
        dir = detectGeodataDir()
    }
    engine = pathfind.NewEngine(dir)
    engine.SetMaxPassableHeight(uint16(cfg.maxPassable))
    // The capsule clearance keeps every planned waypoint and leg away
    // from the walls: the server movement validation is cell level and
    // never checks the character capsule (the elven fighter template
    // radius 7.5), so the planner owns the clearance
    // (docs/pathfinding.md).
    engine.SetCapsuleClearance(pathfind.DefaultCollisionRadius)
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
    attachAcceptance(web, registry, cfg, engine, proxyServer)
    if web != nil && journal != nil {
        web.SetSessionJournal(journal)
    }

    runBotForever(ctx, cfg, tracker, engine, mesh, proxyServer, journal)
    stop()
    shutdownWebInterface(web)
    shutdownProxy(proxyServer)
    if journal != nil {
        journal.Shutdown("process finished")
        journal.Close()
    }
    log.Println("Bot finished")
}

// runFleet launches multiple bot sessions in one process: each bot gets
// its own account (the base -account/-char name plus the 1-based index,
// so -account test1 -bots 3 makes test1, test2, test3), its own tracker
// in a shared registry, and its own runBotForever goroutine. All bots
// share one web interface (the sidebar lists every bot, clicking
// switches the observed one), one proxy (the web UI selects which bot a
// connecting C1 client attaches to) and one geodata engine. The process
// stays alive until every bot supervisor returns (SIGINT/SIGTERM stops
// them all through the shared context).
func runFleet(cfg config) {
    log.Printf("Starting swarm fleet of %d bots", cfg.bots)
    log.Printf("Build: %s", version.Identity())

    registry := state.NewRegistry()
    trackers := make([]*state.Bot, 0, cfg.bots)
    for i := range cfg.bots {
        account := fleetAccountName(cfg.account, i)
        tracker := state.NewBot(account)
        tracker.SetKind(state.KindLongRunning)
        registry.Add(tracker)
        trackers = append(trackers, tracker)
    }
    log.Printf("Fleet accounts: %s", fleetAccountList(cfg.account, cfg.bots))

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
    var engine *pathfind.Engine
    dir := cfg.geodataDir
    if dir == "" {
        dir = detectGeodataDir()
    }
    engine = pathfind.NewEngine(dir)
    engine.SetMaxPassableHeight(uint16(cfg.maxPassable))
    // The capsule clearance of the bot fleet (docs/pathfinding.md).
    engine.SetCapsuleClearance(pathfind.DefaultCollisionRadius)
    stats := engine.Stats()
    if stats.HasData {
        log.Printf("Geodata ready: %d region files in %s, town trips "+
            "and manual long walks enabled", stats.RegionFiles, stats.Dir)
    } else {
        log.Println("No geodata files found in " + stats.Dir +
            ", the bot hunts without town trips")
    }

    web := startWebInterface(cfg, registry, nil, proxyServer)
    attachAcceptance(web, registry, cfg, engine, proxyServer)
    if web != nil && journal != nil {
        web.SetSessionJournal(journal)
    }

    ctx, stop := signal.NotifyContext(context.Background(),
        syscall.SIGINT, syscall.SIGTERM)
    startMemoryWatch(ctx)

    for _, tracker := range trackers {
        wireSessionBot(ctx, journal, tracker)
    }

    // Launch every bot supervisor in its own goroutine. A per-bot
    // config carries the derived account and char name; the rest of
    // the flags (login, hunt, geodata, navmesh, proxy) stay shared.
    mesh := loadNavmesh(cfg)
    var wg sync.WaitGroup
    for i, tracker := range trackers {
        botCfg := cfg
        botCfg.account = fleetAccountName(cfg.account, i)
        botCfg.charName = botCfg.account
        wg.Add(1)
        go func(c config, t *state.Bot) {
            defer wg.Done()
            runBotForever(ctx, c, t, engine, mesh, proxyServer, journal)
        }(botCfg, tracker)
    }
    wg.Wait()
    stop()
    shutdownWebInterface(web)
    shutdownProxy(proxyServer)
    if journal != nil {
        journal.Shutdown("fleet finished")
        journal.Close()
    }
    log.Println("Fleet finished")
}

// fleetAccountName derives the account name of bot i from the base
// name. The base name (cfg.account, default "test1") is used as-is for
// the first bot; subsequent bots get the base stripped of its trailing
// digits plus the 1-based index (test1 -> test2, test3, ...). A base
// without a trailing digit just appends the index (bot -> bot2, bot3).
func fleetAccountName(base string, i int) string {
    if i == 0 {
        return base
    }
    stripped := strings.TrimRight(base, "0123456789")

    return stripped + strconv.Itoa(i+1)
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

    logger.Printf("proxy started, login bound to %v, game bound to %v, log %s",
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

// splitAddresses splits a comma separated flag value.
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

// shutdownProxy stops the client proxy.
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

// runPathfindTest serves the bot less map pathfinding test UI: the geodata
// engine is loaded from the configured or auto detected directory and the
// web interface answers path requests until the process is stopped.
func runPathfindTest(cfg config) {
    dir := cfg.geodataDir
    if dir == "" {
        dir = detectGeodataDir()
    }
    engine := pathfind.NewEngine(dir)
    engine.SetMaxPassableHeight(uint16(cfg.maxPassable))
    // The capsule clearance keeps the test UI routes away from the
    // walls like the live bot plans them (docs/pathfinding.md).
    engine.SetCapsuleClearance(pathfind.DefaultCollisionRadius)

    stats := engine.Stats()
    if stats.HasData {
        log.Printf("Pathfind test: %d geodata region files in %s",
            stats.RegionFiles, stats.Dir)
    } else {
        log.Println("Pathfind test: no geodata files found in " + stats.Dir +
            ", pass -geodata with the game server data/geodata directory")
    }

    web := startWebInterface(cfg, nil, engine, nil)
    if web == nil {
        log.Println("Pathfind test needs the web interface, " +
            "pass a -web address")

        return
    }

    log.Println("Pathfind test UI is ready")

    ctx, stop := signal.NotifyContext(context.Background(),
        syscall.SIGINT, syscall.SIGTERM)
    defer stop()
    <-ctx.Done()
    shutdownWebInterface(web)
    log.Println("Pathfind test finished")
}

// navmeshShowFlag is the -show-navmesh flag value. It implements
// IsBoolFlag so the bare -show-navmesh works (the every tile stitched
// world), while -show-navmesh=21_19,22_19 names the tiles to open.
type navmeshShowFlag struct {
    tiles   []navmesh.RegionKey
    enabled bool
}

// String renders the flag state for the -help listing.
func (f *navmeshShowFlag) String() string {
    if !f.enabled {
        return "false"
    }
    if len(f.tiles) == 0 {
        return "true"
    }
    keys := make([]string, 0, len(f.tiles))
    for _, key := range f.tiles {
        keys = append(keys, strconv.Itoa(int(key.Col))+"_"+
            strconv.Itoa(int(key.Row)))
    }

    return strings.Join(keys, ",")
}

// Set parses the flag value: true/false toggle the mode, anything
// else is the comma separated col_row tile list.
func (f *navmeshShowFlag) Set(value string) error {
    switch value {
    case "true", "":
        f.enabled = true
        f.tiles = nil

        return nil
    case "false":
        f.enabled = false
        f.tiles = nil

        return nil
    }
    specs := strings.Split(value, ",")
    tiles := make([]navmesh.RegionKey, 0, len(specs))
    for _, spec := range specs {
        colText, rowText, found := strings.Cut(strings.TrimSpace(spec), "_")
        if !found {
            return fmt.Errorf("bad tile %q, expected col_row like 21_19", spec)
        }
        col, err := strconv.Atoi(colText)
        if err != nil || col < -32768 || col > 32767 {
            return fmt.Errorf("bad tile column %q: %w", colText, err)
        }
        row, err := strconv.Atoi(rowText)
        if err != nil || row < -32768 || row > 32767 {
            return fmt.Errorf("bad tile row %q: %w", rowText, err)
        }
        // The bounds checks above pin the int16 conversion range.
        col16, row16 := int16(col), int16(row) //nolint:gosec // guarded
        tiles = append(tiles, navmesh.RegionKey{Col: col16, Row: row16})
    }
    f.enabled = true
    f.tiles = tiles

    return nil
}

// IsBoolFlag lets flag.Parse accept the bare -show-navmesh form.
func (f *navmeshShowFlag) IsBoolFlag() bool {
    return true
}

// runNavmeshViewer serves the bot less 3D navmesh viewer (the
// -show-navmesh mode): the mesh tiles of the configured or auto
// detected directory render in the browser, a double click pair runs
// the real corridor search with its construction timer, and the
// process keeps serving until it is stopped. The flag selection
// bounds the initially visible tiles (every tile when empty), the
// route queries always run over the full directory mesh.
func runNavmeshViewer(cfg config) {
    if cfg.webAddress == "" {
        log.Println("Navmesh viewer needs the web interface, " +
            "pass a -web address")

        return
    }
    dir := cfg.navmeshDir
    if dir == "" {
        dir = detectNavmeshDir()
    }
    if dir == "" {
        log.Println("Navmesh viewer found no tile directory, pass " +
            "-navmesh with the cmd/navmesh-build output")

        return
    }
    mesh := navmesh.NewMesh(dir)
    stats := mesh.Stats()
    if stats.TileFiles == 0 {
        log.Println("Navmesh viewer found no tiles in " + dir +
            ", build them with cmd/navmesh-build first")

        return
    }

    // The geodata engine serves the original geometry variant and the
    // capsule clearance of the route answers (the same armed radius
    // the bot runs with).
    geoDir := cfg.geodataDir
    if geoDir == "" {
        geoDir = detectGeodataDir()
    }
    engine := pathfind.NewEngine(geoDir)
    engine.SetMaxPassableHeight(uint16(cfg.maxPassable))
    engine.SetCapsuleClearance(pathfind.DefaultCollisionRadius)

    initial := cfg.navmeshShow.tiles
    if len(initial) > 0 {
        initial = intersectTiles(mesh, initial)
        if len(initial) == 0 {
            log.Println("Navmesh viewer: none of the requested tiles " +
                "exist in " + dir + ", opening every tile instead")
            initial = nil
        }
    }
    if len(initial) == 0 {
        log.Printf("Navmesh viewer: %d tiles stitched in %s",
            stats.TileFiles, stats.Dir)
    } else {
        log.Printf("Navmesh viewer: %d of %d tiles selected in %s",
            len(initial), stats.TileFiles, stats.Dir)
    }

    server := webserver.NewNavmeshServer(mesh, cfg.webAddress,
        log.Default(), webserver.NavmeshOptions{
            InitialTiles: initial,
            Engine:       engine,
        })
    go func() {
        if err := server.ListenAndServe(); err != nil {
            if !errors.Is(err, http.ErrServerClosed) {
                log.Printf("Web interface stopped: %v", err)
            }
        }
    }()

    log.Println("Navmesh viewer UI is ready on http://" +
        server.Address())

    ctx, stop := signal.NotifyContext(context.Background(),
        syscall.SIGINT, syscall.SIGTERM)
    defer stop()
    <-ctx.Done()
    if err := server.Shutdown(context.Background()); err != nil {
        log.Printf("Web interface shutdown: %v", err)
    }
    log.Println("Navmesh viewer finished")
}

// intersectTiles keeps the requested tile keys that exist in the mesh
// directory (the flag may name tiles of another pack).
func intersectTiles(mesh *navmesh.Mesh, requested []navmesh.RegionKey,
) []navmesh.RegionKey {
    existing := make(map[navmesh.RegionKey]struct{})
    for _, key := range mesh.TileFiles() {
        existing[key] = struct{}{}
    }
    found := make([]navmesh.RegionKey, 0, len(requested))
    for _, key := range requested {
        if _, ok := existing[key]; ok {
            found = append(found, key)
        }
    }

    return found
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

// detectGeodataDir picks the first candidate directory that exists.
func detectGeodataDir() string {
    for _, candidate := range defaultGeodataCandidates {
        if info, err := os.Stat(candidate); err == nil && info.IsDir() {
            return candidate
        }
    }
    log.Println("No geodata directory found, pass -geodata explicitly")

    return defaultGeodataCandidates[0]
}

// detectNavmeshDir picks the first candidate directory that holds at
// least one tile file, "" when none does: the mesh is an accelerator
// over the geodata engine, not a dependency - an absent tile set
// keeps the navigator on the pure grid engine without a word of
// noise.
func detectNavmeshDir() string {
    for _, candidate := range defaultNavmeshCandidates {
        entries, err := os.ReadDir(candidate)
        if err != nil {
            continue
        }
        for _, entry := range entries {
            if strings.HasSuffix(entry.Name(), ".nm") {
                return candidate
            }
        }
    }

    return ""
}

// loadNavmesh builds the mesh over the navmesh tile directory when
// the tiles exist: the explicit -navmesh value wins, the autodetected
// data/navmesh follows, and a directory without tiles answers nil so
// the hunt loop installs the plain engine navigator.
func loadNavmesh(cfg config) *navmesh.Mesh {
    dir := cfg.navmeshDir
    if dir == "" {
        dir = detectNavmeshDir()
    }
    if dir == "" {
        return nil
    }
    mesh := navmesh.NewMesh(dir)
    stats := mesh.Stats()
    if stats.TileFiles == 0 {
        log.Println("No navmesh tiles found in " + dir +
            ", the hunt routes stay on the grid engine")

        return nil
    }
    log.Printf("Navmesh ready: %d tiles in %s, the long hunt routes "+
        "serve from the mesh", stats.TileFiles, stats.Dir)

    return mesh
}

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

// attachAcceptance wires the acceptance test manager into the web
// interface: the temp test bots (temp1..temp3) register in the shared
// bot registry, so they appear in the sidebar bot list and stay
// connectable through the client proxy exactly like the fleet bots.
func attachAcceptance(
    web *webserver.Server, registry *state.Registry, cfg config,
    engine *pathfind.Engine, proxyServer *proxy.Server,
) {
    if web == nil {
        return
    }
    manager := newAcceptanceManager(registry, cfg, engine, proxyServer)
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
    engine *pathfind.Engine, proxyServer *proxy.Server,
) *acceptance.Manager {
    return acceptance.NewManager(acceptance.ManagerDeps{
        Registry: registry,
        Login:    cfg.loginAddress,
        Engine:   engine,
        Proxy:    proxyServer,
        Logger:   log.Default(),
        DBConfig: acceptance.DefaultDBConfig(),
    }, acceptance.Definitions())
}

// runHuntAuditCLI measures the live hunting ground geometry: the
// probe account visits every cell focus of the registry through the
// database position injection and the evidence file collects the
// attackable npc population of every ground with the polygon leash
// verdicts. The stack must be up (login 2106, game 7777, MariaDB
// 3306) - the audit is the live measurement the registry
// regeneration builds on (tools/generate_hunt_cells.py). The run
// resumes from the evidence file, so an interrupted audit continues
// with the next unaudited cell; -audit-fresh starts over. The exit
// code reflects
// the audit completion (0 when every spot of the filter measured).
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

// runAcceptanceCLI drives the acceptance scenarios headless: the
// process launches no fleet bot supervisor, only the acceptance
// manager. The "list" value prints the available scenario ids and
// exits; "all" runs every scenario in definition order; a specific id
// runs just that one. The exit code reflects the outcome (0 for a
// pass, 1 for a fail) so an agent or a CI gate can drive the suite
// without the web UI.
//
// The web interface stays optional: pass `-web 127.0.0.1:8080` to
// watch the run from the UI, or `-web ""` to keep the process silent.
// The proxy and the geodata engine load only when the scenarios need
// them (the proxy relay scenario arms its own ephemeral proxy through
// the manager, so the main -proxy flag stays off here).
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

    manager := newAcceptanceManager(registry, cfg, engine, proxyServer)
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

// shutdownWebInterface gracefully stops the web server.
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
