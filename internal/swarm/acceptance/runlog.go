// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// The acceptance run log wiring: the per-run file of the botlog
// package opened at every scenario launch, the environment block the
// header carries, the periodic character state sampler and the
// account-to-run resolution the session runner uses to install the
// packet taps.

package acceptance

import (
    "context"
    "os"
    "strconv"
    "strings"
    "time"

    "github.com/melg8/swarm/internal/swarm/acceptance/botlog"
    "github.com/melg8/swarm/internal/swarm/state"
    "github.com/melg8/swarm/internal/version"
)

// samplePeriod paces the state samples of the run log: ten seconds
// of resolution keeps a twenty minute run at ~120 sample lines while
// the position and phase drift stays visible between the packet
// lines.
const samplePeriod = 10 * time.Second

// openRunLog opens the botlog file of one scenario run (nil when the
// log directory is disabled). The header carries the test card and
// the environment so the file stands alone: the owner attaches it to
// a report without any other artifact of the run.
func (m *Manager) openRunLog(test *Test, generation uint64) *botlog.RunLog {
    if m.logDir == "" {
        return nil
    }
    head := botlog.Header{
        TestID:      test.def.ID,
        Title:       test.def.Title,
        Account:     test.def.Account,
        Generation:  generation,
        Timeout:     test.def.Timeout,
        Description: test.def.Description,
        Checks:      checkDescs(test),
        Environment: m.environment(),
        Args:        strings.Join(os.Args, " "),
    }
    log, err := botlog.Open(m.logDir, head)
    if err != nil {
        m.logger.Printf("Acceptance: the run log of %s is unavailable: %v",
            test.def.ID, err)

        return nil
    }
    m.logger.Printf("Acceptance: the run log of %s writes %s",
        test.def.ID, log.Path())

    return log
}

// checkDescs snapshots the check list of a test into the botlog
// shape.
func checkDescs(test *Test) []botlog.CheckDesc {
    test.mu.Lock()
    defer test.mu.Unlock()
    out := make([]botlog.CheckDesc, 0, len(test.checks))
    for i := range test.checks {
        out = append(out, botlog.CheckDesc{
            ID:     test.checks[i].ID,
            Label:  test.checks[i].Label,
            Done:   test.checks[i].Done,
            Detail: test.checks[i].Detail,
        })
    }

    return out
}

// environment collects the wiring lines of the header: the build
// identity, the login endpoint, the proxy, the database endpoint,
// the navigator and the process facts. The game endpoint resolves
// at the session start (the login server hands it over) and lands in
// the session events instead.
func (m *Manager) environment() []botlog.EnvLine {
    env := []botlog.EnvLine{
        {Key: "build", Value: version.Identity()},
        {Key: "login server", Value: m.login},
        {Key: "database", Value: m.dbConfig.Address + "/" +
            m.dbConfig.Database + " (user " + m.dbConfig.User + ")"},
    }
    if m.proxy != nil {
        env = append(env, botlog.EnvLine{
            Key: "proxy",
            Value: "login " + m.proxy.LoginAddr() + ", game " +
                m.proxy.GameAddr() + ", " +
                strconv.Itoa(m.proxy.ClientCount()) + " clients",
        })
    }
    navigator := "none (no geodata)"
    if m.engine != nil {
        navigator = "grid engine"
        if m.mesh != nil {
            navigator = "navmesh hybrid (grid + mesh)"
        }
    }
    env = append(env, botlog.EnvLine{
        Key: "navigator", Value: navigator,
    })
    env = append(env, botlog.EnvLine{
        Key:   "bots in registry",
        Value: strconv.Itoa(len(m.registry.Bots())),
    })

    return env
}

// runLogFor resolves the live run log of the test that owns the
// account (every scenario owns one temp account, the sessions of the
// account belong to its current run). The nil answer keeps the
// session wiring tapless.
func (m *Manager) runLogFor(account string) *botlog.RunLog {
    for _, test := range m.tests {
        if test.def.Account != account {
            continue
        }
        test.mu.Lock()
        log := test.runLog
        test.mu.Unlock()

        return log
    }

    return nil
}

// runLogSampler publishes the periodic character state of one run:
// the position, the vitals, the wallet, the phase and the target of
// the temp bot, the quick scan line between the packet detail.
type runLogSampler struct {
    tracker *state.Bot
    log     *botlog.RunLog
}

// Run samples until the context ends. The first sample lands at once
// so a run that dies early still carries its start state.
func (s *runLogSampler) Run(ctx context.Context) {
    if s.log == nil || s.tracker == nil {
        return
    }
    s.log.Sample(renderSample(s.tracker))
    ticker := time.NewTicker(samplePeriod)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            s.log.Sample(renderSample(s.tracker))
        }
    }
}

// renderSample reads the tracker into one sample line.
func renderSample(tracker *state.Bot) string {
    x, y, z, ok := tracker.SelfPosition()
    position := "unknown"
    if ok {
        position = strconv.Itoa(int(x)) + "," + strconv.Itoa(int(y)) +
            "," + strconv.Itoa(int(z))
    }
    target := "none"
    if targetID := tracker.SelfTargetID(); targetID != 0 {
        target = tracker.ObjectName(targetID) + " (" +
            strconv.Itoa(int(targetID)) + ")"
    }
    zone := "none"
    if hunting := tracker.Snapshot().HuntingZone; hunting != nil {
        zone = strconv.Itoa(int(hunting.CX)) + "," +
            strconv.Itoa(int(hunting.CY)) + " +-" +
            strconv.Itoa(int(hunting.Half))
    }

    return "state status=" + string(tracker.Status()) +
        " phase=" + tracker.Phase() +
        " pos=" + position +
        " hp=" + strconv.Itoa(int(tracker.SelfHealthPercent())) + "%" +
        " level=" + strconv.Itoa(int(tracker.SelfLevel())) +
        " adena=" + strconv.Itoa(int(
        tracker.InventoryStats().Adena)) +
        " sp=" + strconv.Itoa(int(tracker.SelfSp())) +
        " target=" + target +
        " zone=" + zone
}
