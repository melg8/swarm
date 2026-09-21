// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package acceptance

import (
    "context"
    "errors"
    "fmt"
    "log"
    "strconv"
    "strings"
    "sync"
    "time"

    "github.com/melg8/swarm/internal/swarm/acceptance/botlog"
    "github.com/melg8/swarm/internal/swarm/pathfind"
    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
    "github.com/melg8/swarm/internal/swarm/proxy"
    "github.com/melg8/swarm/internal/swarm/state"
)

// Test statuses reported by the web UI.
const (
    StatusIdle    = "idle"
    StatusRunning = "running"
    StatusPassed  = "passed"
    StatusFailed  = "failed"
)

// Run modes of the run all action.
const (
    ModeSequential = "sequential"
    ModeParallel   = "parallel"
)

// logRingLimit bounds the per test event log the API serves: the
// lines carry the human readable progress of the scenario, the tail
// is what the user reads while the test runs.
const logRingLimit = 40

// restartWait bounds the wait for the previous run of a restarted
// test: the session stop is graceful but the server flush adds up to
// the combat stance delay, the new run proceeds alone after the cap.
const restartWait = 90 * time.Second

// parallelStartStagger spaces the simultaneous launch of the parallel
// run all: the login server flood protector drops the connections
// that arrive too close together, so the scenarios log in one after
// another while their runs still overlap.
const parallelStartStagger = 2 * time.Second

// checkLogLimit bounds how many checks one scenario may publish.
const checkLogLimit = 12

// huntLinePrefix marks the hunt loop decision lines of the session
// logger (the logger convention of the hunt package: every line the
// loop prints starts with it) so the run log tags them apart from
// the scenario narration.
const huntLinePrefix = "Hunt:"

// Check is one verified condition of a scenario: the short label the
// list shows and the detail line of the moment it completed.
type Check struct {
    ID     string `json:"id"`
    Label  string `json:"label"`
    Done   bool   `json:"done"`
    Detail string `json:"detail"`
}

// TestDef describes one acceptance scenario: the identity, the hover
// description (the essence, the start values and the success
// criteria), the temp account it owns and the timeout bounding the
// whole run.
type TestDef struct {
    ID          string
    Title       string
    Description string
    Account     string
    Timeout     time.Duration
    Scenario    ScenarioFunc
}

// TestView is the API payload of one test.
type TestView struct {
    ID          string    `json:"id"`
    Title       string    `json:"title"`
    Account     string    `json:"account"`
    Bots        []string  `json:"bots"`
    Description string    `json:"description"`
    TimeoutSec  int       `json:"timeoutSec"`
    Status      string    `json:"status"`
    StartedAt   time.Time `json:"startedAt"`
    FinishedAt  time.Time `json:"finishedAt"`
    FailReason  string    `json:"failReason"`
    Checks      []Check   `json:"checks"`
    Log         []string  `json:"log"`
}

// ScenarioFunc runs one scenario instance. It receives the run
// context (cancelled on restart and on shutdown) and the test state
// (the checks and the log). Returning nil marks the scenario passed,
// an error fails it; the log lines explain either.
type ScenarioFunc func(ctx context.Context, m *Manager, t *Test) error

// Test is the live state of one acceptance scenario: the definition,
// the current status, the check list and the rolling log. The
// scenarios and the API share one instance per test.
type Test struct {
    def TestDef

    mu         sync.Mutex
    status     string
    failReason string
    checks     []Check
    // checkSeen holds the last decided state of every check id: the
    // run log mirrors only the flips (a steady pending check
    // rewrites every monitor period).
    checkSeen  map[string]bool
    log        []string
    startedAt  time.Time
    finishedAt time.Time
    generation uint64
    cancel     context.CancelFunc
    done       chan struct{}
    // runLog is the per-run botlog file of the current generation
    // (nil between the runs and when the log directory is off): the
    // narration, the checks and the packet taps mirror into it.
    runLog *botlog.RunLog
}

// view snapshots the test state for the API.
func (t *Test) view() TestView {
    t.mu.Lock()
    defer t.mu.Unlock()
    checks := make([]Check, len(t.checks))
    copy(checks, t.checks)
    lines := make([]string, len(t.log))
    copy(lines, t.log)

    return TestView{
        ID:          t.def.ID,
        Title:       t.def.Title,
        Account:     t.def.Account,
        Bots:        []string{t.def.Account},
        Description: t.def.Description,
        TimeoutSec:  int(t.def.Timeout / time.Second),
        Status:      t.status,
        StartedAt:   t.startedAt,
        FinishedAt:  t.finishedAt,
        FailReason:  t.failReason,
        Checks:      checks,
        Log:         lines,
    }
}

// setRunning switches the test into the running state.
func (t *Test) setRunning() {
    t.mu.Lock()
    defer t.mu.Unlock()
    t.status = StatusRunning
    t.failReason = ""
    t.startedAt = time.Now()
    t.finishedAt = time.Time{}
}

// finish records the terminal state unless the run was replaced by a
// newer generation (a restart): the newest run owns the status.
func (t *Test) finish(generation uint64, err error) {
    t.mu.Lock()
    defer t.mu.Unlock()
    if t.generation != generation {
        return
    }
    t.finishedAt = time.Now()
    if err == nil {
        t.status = StatusPassed

        return
    }
    t.status = StatusFailed
    t.failReason = err.Error()
}

// setChecks publishes the condition list of the scenario.
func (t *Test) setChecks(checks []Check) {
    if len(checks) > checkLogLimit {
        checks = checks[:checkLogLimit]
    }
    t.mu.Lock()
    defer t.mu.Unlock()
    t.checks = checks
    t.checkSeen = make(map[string]bool, len(checks))
    if t.runLog != nil {
        descs := make([]botlog.CheckDesc, 0, len(checks))
        for i := range checks {
            descs = append(descs, botlog.CheckDesc{
                ID:     checks[i].ID,
                Label:  checks[i].Label,
                Done:   checks[i].Done,
                Detail: checks[i].Detail,
            })
        }
        t.runLog.Checks(t.def.Timeout, descs)
    }
}

// updateCheck rewrites one check by id. The run log mirrors the
// state flips: the moments the tester decided a condition holds (or
// slipped back after it once held).
func (t *Test) updateCheck(id string, done bool, detail string) {
    t.mu.Lock()
    defer t.mu.Unlock()
    for i := range t.checks {
        if t.checks[i].ID != id {
            continue
        }
        t.checks[i].Done = done
        t.checks[i].Detail = detail
        if t.runLog != nil {
            seen, known := t.checkSeen[id]
            if !known || seen != done {
                t.runLog.CheckTransition(id, t.checks[i].Label, done,
                    detail)
            }
            t.checkSeen[id] = done
        }

        return
    }
}

// appendLog adds one line to the rolling log and mirrors it into
// the run log file of the current run. The hunt loop decision lines
// (they all start with "Hunt:", the logger convention of the loop)
// ride their own tag so the file separates the bot decisions from
// the scenario narration.
func (t *Test) appendLog(line string) {
    t.mu.Lock()
    runLog := t.runLog
    t.mu.Unlock()
    if runLog != nil {
        if strings.HasPrefix(line, huntLinePrefix) {
            runLog.Hunt(line)
        } else {
            runLog.Event("%s", line)
        }
    }
    t.mu.Lock()
    defer t.mu.Unlock()
    t.log = append(t.log, time.Now().Format("15:04:05")+" "+line)
    if len(t.log) > logRingLimit {
        t.log = t.log[len(t.log)-logRingLimit:]
    }
}

// appendDB mirrors one database statement of the character
// injection into the run log under its own tag.
func (t *Test) appendDB(statement string) {
    t.mu.Lock()
    runLog := t.runLog
    t.mu.Unlock()
    if runLog != nil {
        runLog.DB(statement)
    }
}

// Manager owns the acceptance scenarios of the process: the shared
// registry of the temp bots, the database injection channel and the
// run lifecycle of every test.
type Manager struct {
    registry *state.Registry
    login    string
    engine   *pathfind.Engine
    mesh     *navmesh.Mesh
    proxy    *proxy.Server
    logger   *log.Logger
    dbConfig DBConfig
    // logDir is the directory of the per-run botlog files (empty
    // disables the run logs entirely, see -acceptance-log-dir).
    logDir string

    tests []*Test

    dbMu sync.Mutex
    db   *DB
}

// ManagerDeps carries the process level dependencies of the manager.
type ManagerDeps struct {
    Registry *state.Registry
    Login    string
    Engine   *pathfind.Engine
    // Mesh is the navigation mesh of the live integration: the
    // scenarios plan through the same mesh navigator the fleet bot
    // runs (the church entry round proved the acceptance suite must
    // exercise the planner the user's bot serves, the pure grid
    // navigator answers a different walk).
    Mesh     *navmesh.Mesh
    Proxy    *proxy.Server
    Logger   *log.Logger
    DBConfig DBConfig
    // LogDir is the directory of the per-run botlog files (empty
    // disables the run logs, the default derives logs/acceptance
    // from the session directory of the process).
    LogDir string
}

// NewManager builds the manager with the given scenario definitions
// registered in definition order. Every test owns its tracker from
// construction on, so the temp bots appear in the web UI bot list
// immediately (offline until their first run).
func NewManager(deps ManagerDeps, defs []TestDef) *Manager {
    manager := &Manager{
        registry: deps.Registry,
        login:    deps.Login,
        engine:   deps.Engine,
        mesh:     deps.Mesh,
        proxy:    deps.Proxy,
        logger:   deps.Logger,
        dbConfig: deps.DBConfig,
        logDir:   deps.LogDir,
        tests:    nil,
        dbMu:     sync.Mutex{},
        db:       nil,
    }
    for i := range defs {
        def := defs[i]
        tracker := state.NewBot(def.Account)
        tracker.SetKind(state.KindAcceptance)
        tracker.SetOffline()
        deps.Registry.Add(tracker)
        manager.tests = append(manager.tests, &Test{
            def:        def,
            mu:         sync.Mutex{},
            status:     StatusIdle,
            failReason: "",
            checks:     nil,
            checkSeen:  nil,
            log:        nil,
            startedAt:  time.Time{},
            finishedAt: time.Time{},
            generation: 0,
            cancel:     nil,
            done:       nil,
            runLog:     nil,
        })
    }

    return manager
}

// Tests snapshots the state of every scenario.
func (m *Manager) Tests() []TestView {
    views := make([]TestView, 0, len(m.tests))
    for _, test := range m.tests {
        views = append(views, test.view())
    }

    return views
}

// tracker returns the bot state tracker of a test.
func (m *Manager) tracker(t *Test) *state.Bot {
    bot, _ := m.registry.Get(t.def.Account)

    return bot
}

// Start launches (or relaunches) one scenario: a running test is
// cancelled first, the new run waits for the old session to unwind
// so the character reset never races the server side flush. The
// button semantics of the user: pressing run again recreates the bot
// with the same name and the same scenario.
func (m *Manager) Start(id string) error {
    for _, test := range m.tests {
        if test.def.ID != id {
            continue
        }
        if test.def.Scenario == nil {
            return fmt.Errorf("test %s has no scenario", id)
        }
        m.launch(test)

        return nil
    }

    return fmt.Errorf("unknown test %q", id)
}

// Run launches one scenario and blocks until it reaches the terminal
// state (passed or failed). Returns nil when the scenario passed, an
// error wrapping the fail reason otherwise. The CLI path uses this
// to run a scenario without the web UI: the launch path is the same
// as Start, the manager just waits for the run done channel before
// returning. The wait is bounded by the scenario timeout plus the
// restart window so a stuck run still unwinds.
func (m *Manager) Run(ctx context.Context, id string) error {
    test, err := m.findTest(id)
    if err != nil {
        return err
    }
    if err := m.Start(id); err != nil {
        return err
    }
    // The done channel of the just-armed run: launch writes it under
    // the test lock right before going off, so reading it back here is
    // the run we wait on (a concurrent restart would reassign the
    // field, but the channel we hold stays the channel launch created).
    test.mu.Lock()
    done := test.done
    test.mu.Unlock()
    if done == nil {
        return fmt.Errorf("test %q did not arm a run", id)
    }
    timeoutCtx, cancel := context.WithTimeout(ctx,
        test.def.Timeout+restartWait)
    defer cancel()
    select {
    case <-done:
    case <-timeoutCtx.Done():
        return fmt.Errorf("test %q did not finish: %w",
            id, timeoutCtx.Err())
    }
    test.mu.Lock()
    status := test.status
    failReason := test.failReason
    test.mu.Unlock()
    switch status {
    case StatusPassed:
        return nil
    case StatusFailed:
        return fmt.Errorf("test %q failed: %s", id, failReason)
    default:
        return fmt.Errorf("test %q ended in status %q", id, status)
    }
}

// RunAll launches every scenario one after another and blocks until
// they all reach their terminal state. The first failing scenario
// stops the run and its error is returned; the rest stay unrun. The
// CLI uses this for `-acceptance all` so an agent can drive the full
// suite headless.
func (m *Manager) RunAll(ctx context.Context) error {
    for _, test := range m.tests {
        if err := m.Run(ctx, test.def.ID); err != nil {
            return err
        }
    }

    return nil
}

// RunAllParallel launches every scenario at once and blocks until
// they all reach their terminal state. The account partition gives
// every test its own temp bot, so the parallel runs never collide on
// the character side; the launch staggers the logins off the flood
// protector burst path the same way the parallel StartAll does. Unlike
// the sequential RunAll, one failing scenario does not stop the
// others: every failure is collected and the combined error names
// them all, so a single red scenario cannot hide the verdict of the
// rest (the headless agent run pays the wall time of the slowest
// scenario instead of the sum of all of them).
func (m *Manager) RunAllParallel(ctx context.Context) error {
    type outcome struct {
        id  string
        err error
    }
    outcomes := make(chan outcome, len(m.tests))
    var wg sync.WaitGroup
    for i, test := range m.tests {
        if test.def.Scenario == nil {
            outcomes <- outcome{
                id:  test.def.ID,
                err: fmt.Errorf("test %s has no scenario", test.def.ID),
            }

            continue
        }
        wg.Add(1)
        go func(index int, t *Test) {
            defer wg.Done()
            if index > 0 {
                time.Sleep(time.Duration(index) * parallelStartStagger)
            }
            outcomes <- outcome{
                id:  t.def.ID,
                err: m.Run(ctx, t.def.ID),
            }
        }(i, test)
    }
    wg.Wait()
    close(outcomes)
    failures := make([]string, 0, len(m.tests))
    for o := range outcomes {
        if o.err != nil {
            failures = append(failures, o.id+": "+o.err.Error())
        }
    }
    if len(failures) > 0 {
        return fmt.Errorf("%d of %d scenarios failed: %s",
            len(failures), len(m.tests), strings.Join(failures, "; "))
    }

    return nil
}

// findTest resolves the test by id.
func (m *Manager) findTest(id string) (*Test, error) {
    for _, test := range m.tests {
        if test.def.ID == id {
            return test, nil
        }
    }

    return nil, fmt.Errorf("unknown test %q", id)
}

// IDs returns the list of registered scenario ids in definition order.
// The CLI prints this for `-acceptance list` so an agent discovers the
// available scenarios without reading the web UI.
func (m *Manager) IDs() []string {
    ids := make([]string, 0, len(m.tests))
    for _, test := range m.tests {
        ids = append(ids, test.def.ID)
    }

    return ids
}

// launch arms one run of the test: it bumps the generation, cancels
// the previous run and schedules the new one behind it.
func (m *Manager) launch(test *Test) {
    test.mu.Lock()
    previousDone := test.done
    previousCancel := test.cancel
    test.generation++
    generation := test.generation
    test.mu.Unlock()

    if previousCancel != nil {
        previousCancel()
    }

    ctx, cancel := context.WithCancel(context.Background())
    done := make(chan struct{})
    test.mu.Lock()
    test.cancel = cancel
    test.done = done
    test.mu.Unlock()

    go func() {
        if previousDone != nil {
            select {
            case <-previousDone:
            case <-time.After(restartWait):
                test.appendLog("acceptance: the previous run still " +
                    "winds down, starting anyway")
            }
        }
        m.execute(ctx, test, cancel, done, generation)
    }()
}

// execute runs one scenario instance bounded by the test timeout and
// records the terminal status. The run owns its botlog file from the
// first line to the verdict: the header (the test card, the pass/fail
// checks, the environment) is written before the scenario starts and
// the outcome block after it ends, so even a crashed run leaves the
// complete story on disk.
func (m *Manager) execute(
    ctx context.Context, test *Test, cancel context.CancelFunc,
    done chan struct{}, generation uint64,
) {
    // The run owns exactly the done channel launch handed it: a
    // restart reassigns test.done while the previous run still winds
    // down, closing the field here would kill the new run's channel.
    defer func() {
        cancel()
        close(done)
    }()
    runLog := m.openRunLog(test, generation)
    test.setRunLog(runLog)
    test.setRunning()
    test.appendLog("acceptance: run started")

    samplerCtx, stopSampler := context.WithCancel(ctx)
    sampler := &runLogSampler{tracker: m.tracker(test), log: runLog}
    go sampler.Run(samplerCtx)

    timeoutCtx, timeoutCancel := context.WithTimeout(ctx, test.def.Timeout)
    defer timeoutCancel()

    err := test.def.Scenario(timeoutCtx, m, test)
    switch {
    case err != nil:
        test.finish(generation, err)
    case ctx.Err() != nil:
        // The run context (not the timeout) ended: the process shuts
        // down or the test was replaced mid finish. A replacement is
        // filtered by the generation check inside finish.
        test.finish(generation, errors.New("stopped before completing"))
    default:
        test.finish(generation, nil)
        test.appendLog("acceptance: scenario passed")
    }
    stopSampler()
    m.writeVerdict(test, generation, err)

    // The run log closes after the verdict; a session goroutine that
    // still winds down may emit a few more lines - they drop.
    test.setRunLog(nil)
    runLog.Close()
}

// writeVerdict renders the outcome block of the finished run. The
// status and the fail reason of the test already carry the verdict
// of the generation owner (finish filters the stale generations), so
// the block mirrors them with the final check list.
func (m *Manager) writeVerdict(
    test *Test, generation uint64, err error,
) {
    test.mu.Lock()
    if test.generation != generation || test.runLog == nil {
        test.mu.Unlock()

        return
    }
    checks := make([]botlog.CheckDesc, 0, len(test.checks))
    for i := range test.checks {
        checks = append(checks, botlog.CheckDesc{
            ID:     test.checks[i].ID,
            Label:  test.checks[i].Label,
            Done:   test.checks[i].Done,
            Detail: test.checks[i].Detail,
        })
    }
    passed := test.status == StatusPassed
    reason := test.failReason
    runLog := test.runLog
    test.mu.Unlock()
    runLog.Verdict(passed, reason, checks, err)
}

// setRunLog installs (or removes) the run log of the test under the
// lock: the sessions resolve it through the account lookup.
func (t *Test) setRunLog(log *botlog.RunLog) {
    t.mu.Lock()
    defer t.mu.Unlock()
    t.runLog = log
}

// StartAll runs every scenario the requested way: sequential runs
// them one after another (each waits for the previous to finish),
// parallel starts them all at once - every test on its own temp
// account, so the bots never collide.
//
//nolint:gocognit // the launcher interleaves the account and client phases
func (m *Manager) StartAll(mode string) error {
    switch mode {
    case ModeSequential, ModeParallel:
    default:
        return fmt.Errorf("unknown run mode %q", mode)
    }
    if mode == ModeParallel {
        go func() {
            for i, test := range m.tests {
                if i > 0 {
                    // The Mobius login flood protector silently
                    // drops the connections of one address that
                    // arrive within the 350ms window (and the
                    // bursts past 15 quick connects): the stagger
                    // keeps the simultaneous launch of the
                    // scenarios off the burst path - the runs
                    // still overlap, the temp bots just log in
                    // one after another.
                    time.Sleep(parallelStartStagger)
                }
                if err := m.Start(test.def.ID); err != nil {
                    m.logger.Printf("Acceptance: %v", err)
                }
            }
        }()

        return nil
    }
    go func() {
        for _, test := range m.tests {
            test.mu.Lock()
            done := test.done
            status := test.status
            test.mu.Unlock()
            if status == StatusRunning && done != nil {
                // Already running: let it finish first.
                select {
                case <-done:
                case <-time.After(test.def.Timeout + restartWait):
                    continue
                }
                // The finished run may have been replaced already;
                // launch starts a fresh one either way.
            }
            if err := m.Start(test.def.ID); err != nil {
                m.logger.Printf("Acceptance: %v", err)

                continue
            }
            test.mu.Lock()
            done = test.done
            test.mu.Unlock()
            if done != nil {
                <-done
            }
        }
    }()

    return nil
}

// dbConnect lazily opens (and reopens) the shared database channel.
func (m *Manager) dbConnect() (*DB, error) {
    m.dbMu.Lock()
    defer m.dbMu.Unlock()
    if m.db != nil {
        // A dead channel (the stack restarted) reconnects on the next
        // call after the query error path closed it.
        if _, err := m.db.Query("SELECT 1"); err != nil {
            _ = m.db.Close()
            m.db = nil
        }
    }
    if m.db != nil {
        return m.db, nil
    }
    db, err := ConnectDB(m.dbConfig)
    if err != nil {
        return nil, err
    }
    m.db = db

    return db, nil
}

// dbClose drops the shared database channel (the stack went away).
func (m *Manager) dbClose() {
    m.dbMu.Lock()
    defer m.dbMu.Unlock()
    if m.db != nil {
        _ = m.db.Close()
        m.db = nil
    }
}

// injectReset opens the database channel, resets the temp character
// and reports the injection through the test log. Every SQL
// statement of the reset mirrors into the run log (the server side
// preparation trail of the run: the wipes, the injected stacks and
// the vitals rewrite).
func (m *Manager) injectReset(reset characterReset, test *Test) error {
    db, err := m.dbConnect()
    if err != nil {
        return err
    }
    db.SetQueryLog(func(statement string) {
        test.appendDB(statement)
    })
    defer db.SetQueryLog(nil)
    if err := resetCharacter(db, reset, func(line string) {
        test.appendLog(line)
    }); err != nil {
        m.dbClose()

        return err
    }
    test.appendLog("acceptance: character " + reset.Char +
        " reset to level " + itoa(int(reset.Level)) + ", " +
        itoa(int(reset.SP)) + " sp, " + itoa(int(reset.Adena)) + " adena")

    return nil
}

// itoa renders the decimal form of an int.
func itoa(value int) string {
    return strconv.Itoa(value)
}
