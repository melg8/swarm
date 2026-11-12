// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "flag"
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "time"

    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/proxy"
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
}

// Candidate navmesh tile directories, checked in order when -navmesh
// is empty: the tiles are the offline output of cmd/navmesh-build and
// live in the bot tree (data/navmesh is the documented default of the
// build command), never in the server tree.
var defaultNavmeshCandidates = []string{
    filepath.Join("data", "navmesh"),
}

type config struct {
    loginAddress  string
    account       string
    password      string
    charName      string
    webAddress    string
    hunt          bool
    pathfindTest  bool
    testFightUI   bool
    testPathUI    bool
    testPathV2UI  bool
    testPathV3UI  bool
    testFightUIV1 bool
    geodataDir    string
    navmeshDir    string
    // navmeshCompareDir is the -navmesh-compare flag: the second
    // tile pack of the dual pack view (issue #59). The -show-navmesh
    // viewer renders it in pane B beside the -navmesh pack of pane
    // A, with the per tile poly diff and the compare pathfind.
    navmeshCompareDir string
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
    // configPath is the -config flag: the launch configuration file
    // driving the shared parameters and the fleet composition (see
    // launch_config.go). Empty keeps the plain flag defaults.
    configPath string
    // botPlans is the expanded fleet composition (classicFleetPlan /
    // launchConfig.expand): the account and the bot type of every
    // fleet slot, whatever the origin of the composition (the
    // -config file or the plain -bots count). Nil until the launch
    // normalization fills it.
    botPlans []botPlan
    // botType is the type of the bot this configuration launches
    // ("fighter" today): the fleet goroutine and the single bot path
    // set it from their plan slot, the future per-type behaviors (the
    // archer of issue #13) branch on it.
    botType string
    // kite is the per-plan kite fight override (the "kite" section
    // of the bot spec, owner issue #29): nil keeps the shipped
    // tuning of DefaultKiteParams, a set section overrides the
    // named knobs for this bot whole - the standing-archer baseline
    // and the tuned profile switch by config, not by rebuild. The
    // fleet goroutine and the single bot path set it from their plan
    // slot next to the bot type.
    kite *botKiteSpec
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
    // acceptanceLogDir is the directory of the per-run acceptance
    // bot logs (the hyper-detailed trail of every scenario run: the
    // test contract, the environment, every packet, every monitor
    // verdict). Empty derives "<session-dir>/acceptance" (or
    // "logs/acceptance" when the journal is off), "off" disables
    // the run logs.
    acceptanceLogDir string
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

//nolint:funlen // the flag surface is one linear block by design
func parseFlags() config {
    cfg := config{
        loginAddress:      "",
        account:           "",
        password:          "",
        charName:          "",
        webAddress:        "",
        hunt:              false,
        pathfindTest:      false,
        testFightUI:       false,
        testPathUI:        false,
        testPathV2UI:      false,
        testPathV3UI:      false,
        testFightUIV1:     false,
        geodataDir:        "",
        navmeshDir:        "",
        navmeshCompareDir: "",
        navmeshShow:       navmeshShowFlag{tiles: nil, enabled: false},
        maxPassable:       uint(pathfind.DefaultMaxPassableHeight),
        proxy:             false,
        proxyLogin:        "",
        proxyGame:         "",
        proxyLog:          "",
        bots:              1,
        configPath:        "",
        botPlans:          nil,
        botType:           "",
        kite:              nil,
        acceptanceRun:     "",
        sessionDir:        "",
        acceptanceLogDir:  "",
        sessionReport:     "",
        sessionQuery:      "",
        sessionAnomalies:  "",
        queryFrom:         "",
        queryTo:           "",
        queryEvents:       "",
        queryMatch:        "",
        queryContext:      0,
        queryLimit:        0,
        huntAudit:         "",
        auditWait:         0,
        auditAccount:      "",
        auditAnchors:      "",
        auditFilter:       "",
        auditStride:       0,
        auditFresh:        false,
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
    flag.BoolVar(&cfg.testPathUI, "test-path-ui", false,
        "walk plan visualization gallery instead of the bot: no game "+
            "connection, a grid of twenty numbered animated ways to "+
            "draw the planned walk of the bot on the map, every cell "+
            "walking the same route with five stops and a destination")
    flag.BoolVar(&cfg.testPathV2UI, "test-path-ui-v2", false,
        "smooth curve animation lab instead of the bot: no game "+
            "connection, a grid of thirty numbered animated ways to "+
            "draw the same Catmull-Rom walk, motion first, then color "+
            "and stroke approach")
    flag.BoolVar(&cfg.testPathV3UI, "test-path-ui-v3", false,
        "flow and snake speed lab instead of the bot: no game "+
            "connection, a grid of forty numbered cards, twenty "+
            "rainbow flow variants in five palettes at four hue "+
            "speeds and twenty pulse snake variants in five crest "+
            "schemes at four wave periods; the flow hue bands travel "+
            "from the start of the walk to the destination")
    flag.BoolVar(&cfg.testFightUIV1, "test-fight-ui-v1", false,
        "combat animation variant showcase UI (v1 idea set) instead of the "+
            "bot: no game connection, a looping hero versus enemy demo fight "+
            "plays every damage visualization idea side by side "+
            "for picking one")
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
    flag.StringVar(&cfg.navmeshCompareDir, "navmesh-compare", "",
        "second navmesh tile directory for the -show-navmesh dual "+
            "pack view (issue #59): pane B renders this pack beside "+
            "the -navmesh one of pane A, the per tile poly diff "+
            "tints the changed regions and every route pair runs "+
            "through both packs (the old and the reduced "+
            "decomposition of the same geodata)")
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
    flag.StringVar(&cfg.configPath, "config", "",
        "launch configuration file (JSON): the shared launch parameters "+
            "and the fleet composition - the bot types with their counts "+
            "(see configs/swarm.json and docs/launch_config.md). An "+
            "explicitly set flag overrides the matching file value, an "+
            "omitted file field keeps the flag default, and without the "+
            "file the plain flag defaults apply")
    flag.StringVar(&cfg.acceptanceRun, "acceptance", "",
        "run an acceptance scenario headless instead of the bot: "+
            "the value is a scenario id (soak, farm-readiness, "+
            "bot-lifetime, proxy-relay) or 'all' to run every "+
            "scenario in definition order, 'all-parallel' to launch "+
            "them at once (the account partition keeps the runs "+
            "apart, the wall time drops to the slowest scenario) or "+
            "'list' to print the available ids and exit. No fleet bot "+
            "supervisor runs; the acceptance manager launches the temp bot "+
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
    flag.StringVar(&cfg.acceptanceLogDir, "acceptance-log-dir", "",
        "directory of the per-run acceptance bot logs - one "+
            "hyper-detailed file per scenario run (the test "+
            "contract, the environment, every packet the temp bot "+
            "receives and sends, every monitor check verdict and "+
            "the outcome block). Empty derives it from -session-dir "+
            "(<session-dir>/acceptance, or logs/acceptance when the "+
            "journal is off); 'off' disables the run logs")
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

    // The launch configuration file folds in after the flag parse:
    // an explicitly set flag wins over the file (flag.Visit names
    // them), an omitted file field keeps its flag default, and a
    // broken file refuses to launch (a typo'd key silently falling
    // back to a default is the worst failure mode a config can have).
    if cfg.configPath != "" {
        lc, err := loadLaunchConfig(cfg.configPath)
        if err != nil {
            fmt.Fprintf(os.Stderr, "swarm: %v\n", err)
            os.Exit(1)
        }
        explicit := map[string]bool{}
        flag.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
        applyLaunchConfig(&cfg, lc, explicit)
    }

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
