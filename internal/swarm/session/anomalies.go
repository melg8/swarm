// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package session

import (
    "fmt"
    "io"
    "regexp"
    "sort"
    "strings"
)

// The anomaly scan thresholds. Every threshold trades noise against
// coverage: the counts below flag the patterns that wasted real minutes
// in the observed six hour fleet runs while ignoring the healthy
// background chatter of the world.
const (
    // patternMinCount flags a repeated decision line shape.
    patternMinCount = 12
    // clusterGapSec starts a new emergency logout cluster.
    clusterGapSec = int64(300)
    // logoutClusterMin flags a cluster of emergency logouts.
    logoutClusterMin = 3
    // fightOutlierSec flags one suspiciously long fight.
    fightOutlierSec = 300.0
    // tripLoopMin flags a repeated town trip end reason.
    tripLoopMin = 3
    // deathBurstSec and deathBurstMin flag a death streak window.
    deathBurstSec = int64(600)
    deathBurstMin = 5
    // stallReportSec flags one stagnation event worth reading.
    stallReportSec = 600.0
    // churnSessionSec flags a short-lived session of a reconnect loop.
    churnSessionSec = int64(120)
    // gapReportSec flags an offline stretch between samples.
    gapReportSec = int64(300)
)

// digitRun matches a run of digits (the pattern normalizer collapses
// the variable parts of a repeated line: object ids, coordinates,
// counters).
var digitRun = regexp.MustCompile(`[0-9]+`)

// patternStat accumulates one repeated story line shape.
type patternStat struct {
    sample string
    count  int
    first  int64
    last   int64
}

// reasonStat accumulates one repeated trip end reason.
type reasonStat struct {
    count int
    first int64
    last  int64
}

// mobAnomaly summarizes the fights against one mob species.
type mobAnomaly struct {
    name   string
    kills  int
    durMax float64
}

// logoutCluster is one burst of emergency logouts.
type logoutCluster struct {
    first int64
    last  int64
    count int
}

// anomalyBot is the streaming collector of one bot.
type anomalyBot struct {
    id       string
    first    int64
    last     int64
    patterns map[string]*patternStat
    // logoutAt collects the emergency logout moments (story markers and
    // the honest logout events of later builds).
    logoutAt []int64
    // closedLost counts the lost sessions whose reason names the
    // self-closed socket (the emergency logout signature of the builds
    // before the honest reason line).
    closedLost   int
    killOutliers []fightMark
    mobs         map[string]*mobAnomaly
    trips        int
    tripReasons  map[string]*reasonStat
    deaths       []deathMark
    stalls       []stallMark
    enteredAt    int64
    connects     int
    losts        int
    lifetimes    []int64
    lastSample   int64
    gaps         []gapMark
    storyMinutes map[int64]int
}

// newAnomalyBot builds the empty collector.
func newAnomalyBot(id string) *anomalyBot {
    return &anomalyBot{
        id:           id,
        first:        0,
        last:         0,
        patterns:     make(map[string]*patternStat),
        logoutAt:     nil,
        closedLost:   0,
        killOutliers: nil,
        mobs:         make(map[string]*mobAnomaly),
        trips:        0,
        tripReasons:  make(map[string]*reasonStat),
        deaths:       nil,
        stalls:       nil,
        enteredAt:    0,
        connects:     0,
        losts:        0,
        lifetimes:    nil,
        lastSample:   0,
        gaps:         nil,
        storyMinutes: make(map[int64]int),
    }
}

// anomalyScan is the whole-file collector.
type anomalyScan struct {
    bots  map[string]*anomalyBot
    order []string
    total int
    first int64
    last  int64
}

// RunAnomalies streams the journal once and prints the ranked findings
// of the long-run behavior: the tool answers "where should I look"
// without the agent reading the whole session. Every finding carries
// its own drill-down command line for -session-query.
func RunAnomalies(path string, out io.Writer) error {
    scan := &anomalyScan{
        bots:  make(map[string]*anomalyBot),
        order: nil,
        total: 0,
        first: 0,
        last:  0,
    }
    if err := scanJournal(path, scan.visit); err != nil {
        return err
    }
    render := &anomalyRender{scan: scan, path: path, out: out}
    render.write()

    return nil
}

// visit folds one record into the scan.
func (s *anomalyScan) visit(r record) error {
    s.total++
    if s.first == 0 {
        s.first = r.T
    }
    if r.T > s.last {
        s.last = r.T
    }
    if r.B == "" {
        return nil
    }
    bot, ok := s.bots[r.B]
    if !ok {
        bot = newAnomalyBot(r.B)
        s.bots[r.B] = bot
        s.order = append(s.order, r.B)
    }
    if bot.first == 0 {
        bot.first = r.T
    }
    if r.T > bot.last {
        bot.last = r.T
    }
    bot.apply(r)

    return nil
}

// apply folds one record of one bot.
func (a *anomalyBot) apply(r record) {
    switch r.E {
    case kindStory:
        a.applyStory(r)
    case kindLogout:
        a.logoutAt = append(a.logoutAt, r.T)
    case kindKill:
        a.applyKill(r)
    case kindDeath:
        a.deaths = append(a.deaths, deathMark{t: r.T, lv: r.Lv, x: r.X, y: r.Y})
    case kindStall:
        a.stalls = append(a.stalls,
            stallMark{t: r.T, sec: r.Dur, x: r.X, y: r.Y})
    case kindTripEnd:
        a.applyTripEnd(r)
    case kindConnect:
        if r.R == "entered" {
            a.connects++
            a.enteredAt = r.T
        }
    case kindLost:
        a.applyLost(r)
    case kindSample:
        a.applySample(r)
    }
}

// applyStory folds one story line: the pattern counter over the Hunt
// decision shapes and the emergency logout markers.
func (a *anomalyBot) applyStory(r record) {
    a.storyMinutes[r.T/60]++
    if strings.Contains(r.M, "emergency logout") {
        a.logoutAt = append(a.logoutAt, r.T)

        return
    }
    if !strings.HasPrefix(r.M, "Hunt: ") {
        return
    }
    key := digitRun.ReplaceAllString(r.M, "#")
    if len(key) > 96 {
        key = key[:96]
    }
    stat := a.patterns[key]
    if stat == nil {
        a.patterns[key] = &patternStat{
            sample: r.M, count: 1, first: r.T, last: r.T,
        }

        return
    }
    stat.count++
    stat.last = r.T
}

// applyKill folds one kill into the outlier and per-mob views.
func (a *anomalyBot) applyKill(r record) {
    mob := a.mobs[r.Mob]
    if mob == nil {
        mob = &mobAnomaly{name: r.Mob, kills: 0, durMax: 0}
        a.mobs[r.Mob] = mob
    }
    mob.kills++
    if r.Dur > mob.durMax {
        mob.durMax = r.Dur
    }
    if r.Dur >= fightOutlierSec {
        a.killOutliers = append(a.killOutliers, fightMark{
            t: r.T, mob: r.Mob, lvl: r.Lvl, dur: r.Dur,
        })
    }
}

// applyTripEnd folds one finished trip.
func (a *anomalyBot) applyTripEnd(r record) {
    a.trips++
    stat := a.tripReasons[r.R]
    if stat == nil {
        a.tripReasons[r.R] = &reasonStat{count: 1, first: r.T, last: r.T}

        return
    }
    stat.count++
    stat.last = r.T
}

// applyLost folds one lost session: the closed-socket signature of the
// emergency logout and the session lifetime against the last entry.
func (a *anomalyBot) applyLost(r record) {
    a.losts++
    if strings.Contains(r.R, "use of closed network connection") {
        a.closedLost++
    }
    if a.enteredAt != 0 {
        a.lifetimes = append(a.lifetimes, r.T-a.enteredAt)
        a.enteredAt = 0
    }
}

// applySample folds one sample for the offline gap detection.
func (a *anomalyBot) applySample(r record) {
    if a.lastSample != 0 && r.T-a.lastSample >= gapReportSec {
        gap := r.T - a.lastSample
        mark := gapMark{t: a.lastSample, sec: float64(gap)}
        a.gaps = append(a.gaps, mark)
    }
    a.lastSample = r.T
}

// clusters splits the logout moments into bursts separated by gaps.
func (a *anomalyBot) clusters() []logoutCluster {
    var clusters []logoutCluster
    for _, at := range a.logoutAt {
        if n := len(clusters); n > 0 && at-clusters[n-1].last <= clusterGapSec {
            clusters[n-1].last = at
            clusters[n-1].count++

            continue
        }
        clusters = append(clusters,
            logoutCluster{first: at, last: at, count: 1})
    }

    return clusters
}

// medianLifetime returns the median session lifetime in seconds.
func (a *anomalyBot) medianLifetime() int64 {
    if len(a.lifetimes) == 0 {
        return 0
    }
    live := append([]int64(nil), a.lifetimes...)
    sort.Slice(live, func(i, j int) bool { return live[i] < live[j] })

    return live[len(live)/2]
}

// churnCount returns the sessions that lived shorter than the churn
// threshold.
func (a *anomalyBot) churnCount() int {
    churn := 0
    for _, life := range a.lifetimes {
        if life <= churnSessionSec {
            churn++
        }
    }

    return churn
}

// mutedCount replicates the story cap of the live journal over the
// per-minute counters: a healthy bot stays under the cap, a flooded one
// mutes the difference.
func (a *anomalyBot) mutedCount() int {
    muted := 0
    for _, count := range a.storyMinutes {
        if count > storyCapPerMin {
            muted += count - storyCapPerMin
        }
    }

    return muted
}

// deathBursts returns the windows that hold a death streak.
func (a *anomalyBot) deathBursts() []logoutCluster {
    var bursts []logoutCluster
    left := 0
    for right := range a.deaths {
        for a.deaths[right].t-a.deaths[left].t > deathBurstSec {
            left++
        }
        count := right - left + 1
        if count >= deathBurstMin {
            n := len(bursts)
            if n > 0 && bursts[n-1].last == a.deaths[right-1].t {
                bursts[n-1].last = a.deaths[right].t
                bursts[n-1].count = count

                continue
            }
            bursts = append(bursts, logoutCluster{
                first: a.deaths[left].t, last: a.deaths[right].t, count: count,
            })
        }
    }

    return bursts
}

// topPatterns returns the repeated decision shapes ranked by count.
func (a *anomalyBot) topPatterns() []*patternStat {
    var stats []*patternStat
    for _, stat := range a.patterns {
        if stat.count >= patternMinCount {
            stats = append(stats, stat)
        }
    }
    sort.Slice(stats, func(i, j int) bool {
        return stats[i].count > stats[j].count
    })

    return stats
}

// slowMobs returns the mob species whose longest fight crossed the
// outlier threshold.
func (a *anomalyBot) slowMobs() []*mobAnomaly {
    var mobs []*mobAnomaly
    for _, mob := range a.mobs {
        if mob.durMax >= fightOutlierSec {
            mobs = append(mobs, mob)
        }
    }
    sort.Slice(mobs, func(i, j int) bool {
        return mobs[i].durMax > mobs[j].durMax
    })

    return mobs
}

// tripLoop is one repeated town trip end reason.
type tripLoop struct {
    reason string
    stat   *reasonStat
}

// tripLoops returns the FAILED trip end reasons that repeated enough
// to count as a loop, ranked by count. The failure vocabulary: the
// aborted trips, the timeouts, the refused walks - the successful
// endings ("back at the farm spot", the combat handover) never loop
// anything.
func (a *anomalyBot) tripLoops() []tripLoop {
    var loops []tripLoop
    for reason, stat := range a.tripReasons {
        if stat.count >= tripLoopMin && tripFailed(reason) {
            loops = append(loops, tripLoop{reason: reason, stat: stat})
        }
    }
    sort.Slice(loops, func(i, j int) bool {
        return loops[i].stat.count > loops[j].stat.count
    })

    return loops
}

// tripFailed reports whether a trip end reason names a failure: the
// markers of the aborted, timed out and refused endings of the town
// trip machinery.
func tripFailed(reason string) bool {
    for _, marker := range [...]string{
        "abort", "fail", "timeout", "timed out", "no walkable",
        "stuck", "refus", "would swim", "could not", "never",
    } {
        if strings.Contains(reason, marker) {
            return true
        }
    }

    return false
}

// The finding kinds of the ranked report.
const (
    sevHigh   = "HIGH"
    sevMedium = "MEDIUM"
    sevLow    = "LOW"
)

// finding is one rendered anomaly: a severity, a headline, detail lines
// and the drill-down command line that isolates it.
type finding struct {
    severity string
    score    float64
    title    string
    details  []string
    drill    string
}

// anomalyRender renders the scan into the ranked plain text report.
type anomalyRender struct {
    scan *anomalyScan
    path string
    out  io.Writer
}

// write collects and prints the findings, the highest waste first.
func (r *anomalyRender) write() {
    findings := r.collect()
    sort.SliceStable(findings, func(i, j int) bool {
        return findings[i].score > findings[j].score
    })
    fmt.Fprintf(r.out, "swarm session anomalies\n")
    fmt.Fprintf(r.out, "journal: %s (%d records, %s .. %s)\n\n",
        r.path, r.scan.total, hhmm(r.scan.first), hhmm(r.scan.last))
    if len(findings) == 0 {
        fmt.Fprintf(r.out, "no anomalies above the thresholds\n")

        return
    }
    for i, f := range findings {
        fmt.Fprintf(r.out, "[%d] %s %s\n", i+1, f.severity, f.title)
        for _, detail := range f.details {
            fmt.Fprintf(r.out, "    %s\n", detail)
        }
        if f.drill != "" {
            fmt.Fprintf(r.out, "    drill: %s\n", f.drill)
        }
        fmt.Fprintln(r.out)
    }
}

// collect builds every finding the scan supports.
func (r *anomalyRender) collect() []finding {
    findings := make([]finding, 0, 16)
    for _, bot := range r.scan.order {
        agg := r.scan.bots[bot]
        findings = append(findings, r.logoutFindings(agg)...)
        findings = append(findings, r.patternFindings(agg)...)
        findings = append(findings, r.tripFindings(agg)...)
        findings = append(findings, r.fightFindings(agg)...)
        findings = append(findings, r.deathFindings(agg)...)
        findings = append(findings, r.stallFindings(agg)...)
        findings = append(findings, r.gapFindings(agg)...)
    }

    return findings
}

// drillLine builds the ready-made -session-query command of a window.
func (r *anomalyRender) drillLine(bot string, from int64, to int64) string {
    return fmt.Sprintf("swarm -session-query %s -account %s -from %s -to %s",
        r.path, bot, hhmm(from), hhmm(to))
}

// logoutFindings flags the emergency logout loops: the bot that keeps
// re-entering the ground that keeps kicking it out.
func (r *anomalyRender) logoutFindings(a *anomalyBot) []finding {
    findings := make([]finding, 0, 1)
    logouts := len(a.logoutAt)
    if logouts == 0 && a.closedLost == 0 {
        return findings
    }
    clusters := a.clusters()
    hot := 0
    for _, cluster := range clusters {
        if cluster.count >= logoutClusterMin {
            hot++
        }
    }
    severity := sevMedium
    if logouts >= 10 || a.churnCount() >= 10 {
        severity = sevHigh
    }
    details := []string{
        fmt.Sprintf("%s: %d emergency logouts, %d closed-socket reconnects",
            a.id, logouts, a.closedLost),
        fmt.Sprintf("median session %s of %d, %d sessions under %ds",
            durText(float64(a.medianLifetime())), len(a.lifetimes),
            a.churnCount(), churnSessionSec),
    }
    for _, cluster := range clusters {
        if cluster.count < logoutClusterMin {
            continue
        }
        details = append(details, fmt.Sprintf("burst %s..%s, %d logouts",
            hhmm(cluster.first), hhmm(cluster.last), cluster.count))
    }
    last := a.last
    drillFrom := a.first
    if logouts > 0 {
        drillFrom = a.logoutAt[0]
        last = a.logoutAt[len(a.logoutAt)-1]
    }
    findings = append(findings, finding{
        severity: severity,
        score:    float64(logouts+a.closedLost) * 90,
        title:    "emergency logout loop (re-entering too-hot ground)",
        details:  details,
        drill:    r.drillLine(a.id, drillFrom, last),
    })

    return findings
}

// patternFindings flags the repeated decision lines: a loop that
// re-issues the same decision shape every tick wasted the whole window.
func (r *anomalyRender) patternFindings(a *anomalyBot) []finding {
    patterns := a.topPatterns()
    findings := make([]finding, 0, len(patterns))
    for _, stat := range patterns {
        severity := sevMedium
        if stat.count >= 100 {
            severity = sevHigh
        }
        sample := stat.sample
        if len(sample) > 76 {
            sample = sample[:76] + "..."
        }
        findings = append(findings, finding{
            severity: severity,
            score:    float64(stat.count),
            title:    "repeated decision line",
            details: []string{
                fmt.Sprintf("%s: %dx %s..%s", a.id, stat.count,
                    hhmm(stat.first), hhmm(stat.last)),
                "  " + sample,
            },
            drill: r.drillLine(a.id, stat.first, stat.last),
        })
    }

    return findings
}

// tripFindings flags the town trip loops: one FAILED trip reason
// repeating over the session burned its cooldowns without progress.
// The successful endings (the farm spot return, the combat handover)
// never count - a bot that keeps finishing its trips is healthy
// whatever the reason distribution looks like.
func (r *anomalyRender) tripFindings(a *anomalyBot) []finding {
    loops := a.tripLoops()
    findings := make([]finding, 0, len(loops))
    for _, loop := range loops {
        severity := sevMedium
        if loop.stat.count >= 10 {
            severity = sevHigh
        }
        findings = append(findings, finding{
            severity: severity,
            score:    float64(loop.stat.count) * 300,
            title:    "town trip loop",
            details: []string{
                fmt.Sprintf("%s: %d of %d trips ended %q",
                    a.id, loop.stat.count, a.trips, loop.reason),
                fmt.Sprintf("first %s, last %s",
                    hhmm(loop.stat.first), hhmm(loop.stat.last)),
            },
            drill: r.drillLine(a.id, loop.stat.first, loop.stat.last),
        })
    }

    return findings
}

// fightFindings flags the fight duration outliers: one mob species the
// journal credits with fights of many minutes names either a genuinely
// stalled engage or a fight clock that never reset between the kills of
// a respawned (id-recycled) spawn point.
func (r *anomalyRender) fightFindings(a *anomalyBot) []finding {
    slow := a.slowMobs()
    findings := make([]finding, 0, len(slow)+1)
    if len(slow) == 0 {
        return findings
    }
    for _, mob := range slow {
        severity := sevMedium
        if mob.durMax >= 1800 {
            severity = sevHigh
        }
        findings = append(findings, finding{
            severity: severity,
            score:    mob.durMax,
            title:    "fight duration outlier",
            details: []string{
                fmt.Sprintf("%s: %s, %d kills, longest %s",
                    a.id, mob.name, mob.kills, durText(mob.durMax)),
                "suspect the respawn clock: the server recycles object ids",
            },
            drill: "",
        })
    }
    for _, mark := range a.killOutliers[:min(len(a.killOutliers), 5)] {
        findings = append(findings, finding{
            severity: sevLow,
            score:    mark.dur,
            title:    "longest fights",
            details: []string{
                fmt.Sprintf("%s: %s (lvl %d) in %s at %s",
                    a.id, mark.mob, mark.lvl, durText(mark.dur), hhmm(mark.t)),
            },
            drill: "",
        })
    }

    return findings
}

// deathFindings flags the death streaks: a burst of deaths in one
// window is either a delevel run or a death loop, the positions tell
// them apart.
func (r *anomalyRender) deathFindings(a *anomalyBot) []finding {
    bursts := a.deathBursts()
    findings := make([]finding, 0, len(bursts))
    for _, burst := range bursts {
        spots := make(map[[2]int32]int)
        for _, death := range a.deaths {
            if death.t >= burst.first && death.t <= burst.last {
                spots[[2]int32{death.x, death.y}]++
            }
        }
        details := []string{
            fmt.Sprintf("%s: %d deaths in %s..%s", a.id, burst.count,
                hhmm(burst.first), hhmm(burst.last)),
            fmt.Sprintf("%d distinct positions", len(spots)),
        }
        findings = append(findings, finding{
            severity: sevHigh,
            score:    float64(burst.count) * 60,
            title:    "death streak (delevel run or death loop)",
            details:  details,
            drill:    r.drillLine(a.id, burst.first, burst.last),
        })
    }

    return findings
}

// stallFindings flags the stagnation events an agent should read.
func (r *anomalyRender) stallFindings(a *anomalyBot) []finding {
    findings := make([]finding, 0, 4)
    for _, stall := range a.stalls {
        if stall.sec < stallReportSec {
            continue
        }
        findings = append(findings, finding{
            severity: sevMedium,
            score:    stall.sec,
            title:    "stagnation stall",
            details: []string{
                fmt.Sprintf("%s: %s stall held %s at %d %d",
                    a.id, "xp/position", durText(stall.sec), stall.x, stall.y),
            },
            drill: r.drillLine(a.id, stall.t-600, stall.t+600),
        })
    }

    return findings
}

// gapFindings flags the offline stretches and the flood mute: the
// mute count names how much of the story the cap swallowed.
func (r *anomalyRender) gapFindings(a *anomalyBot) []finding {
    findings := make([]finding, 0, len(a.gaps)+1)
    for _, gap := range a.gaps {
        findings = append(findings, finding{
            severity: sevLow,
            score:    gap.sec,
            title:    "offline gap",
            details: []string{
                fmt.Sprintf("%s: %s of silence from %s",
                    a.id, durText(gap.sec), hhmm(gap.t)),
            },
            drill: "",
        })
    }
    if muted := a.mutedCount(); muted > 0 {
        findings = append(findings, finding{
            severity: sevLow,
            score:    float64(muted) / 10,
            title:    "story flood mute",
            details: []string{
                fmt.Sprintf("%s: %d story lines over the "+
                    "per-minute cap were muted",
                    a.id, muted),
            },
            drill: "",
        })
    }

    return findings
}
