// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package session

import (
        "errors"
        "fmt"
        "io"
        "regexp"
        "strings"
        "time"
)

// QueryFilter carries the command line filters of the -session-query
// drill-down: the tool answers "what exactly happened around X" without
// the agent reading the whole session file by hand. Every filter is
// optional; the empty filter dumps the head of the journal. The From
// and To fields stay raw strings on purpose: a bare clock argument
// resolves against the date of the first journal record (a session that
// crosses midnight binds 01:30 to the morning after the 22:51 start).
type QueryFilter struct {
        // Bot restricts the output to one bot id (empty keeps every bot).
        Bot string
        // Events restricts the output to these event kinds (empty keeps
        // every kind). The names are the wire "e" values.
        Events []string
        // Match is a regular expression over the story text and the reason
        // fields (empty keeps everything).
        Match string
        // From and To bound the time window: RFC3339 timestamps or bare
        // 15:04 / 15:04:05 clocks resolved on the first record date.
        From string
        To   string
        // Context prints the records within this duration around every
        // match as dimmed context lines (0 disables the context mode).
        Context time.Duration
        // Limit caps the printed records (0 prints everything).
        Limit int
}

// The query errors.
var (
        // errQueryBadTime marks an unparsable -from/-to argument.
        errQueryBadTime = errors.New("bad time (want 15:04, 15:04:05 or RFC3339)")
        // errQueryBadRegex marks an unparsable -match expression.
        errQueryBadRegex = errors.New("bad match expression")
)

// ParseQueryTime resolves one -from/-to argument: a full RFC3339
// timestamp passes through, a bare 15:04 or 15:04:05 clock lands on the
// day of the base time that sits closest to it (a session crossing
// midnight binds 01:30 to the morning after the 22:51 start, not the
// 21 hours before it).
func ParseQueryTime(value string, base time.Time) (time.Time, error) {
        for _, layout := range [...]string{"15:04", "15:04:05"} {
                if len(value) != len(layout) {
                        continue
                }
                parsed, err := time.ParseInLocation(layout, value, base.Location())
                if err != nil {
                        return time.Time{}, errQueryBadTime
                }
                // Land the bare clock on the base date first, then pick the
                // neighboring day when it sits closer (the overnight session).
                parsed = parsed.AddDate(base.Year(), int(base.Month())-1, base.Day()-1)

                return nearestDay(parsed, base), nil
        }
        parsed, err := time.Parse(time.RFC3339, value)
        if err != nil {
                return time.Time{}, errQueryBadTime
        }

        return parsed, nil
}

// nearestDay shifts a bare clock time onto the base day or one of its
// neighbors, whichever lands closest to the base moment.
func nearestDay(clock time.Time, base time.Time) time.Time {
        best := clock
        bestGap := absDuration(clock.Sub(base))
        for _, shift := range [...]int{-1, 1} {
                candidate := clock.AddDate(0, 0, shift)
                if gap := absDuration(candidate.Sub(base)); gap < bestGap {
                        best, bestGap = candidate, gap
                }
        }

        return best
}

// absDuration returns the absolute value of a duration.
func absDuration(d time.Duration) time.Duration {
        if d < 0 {
                return -d
        }

        return d
}

// queryMatcher holds the compiled filters of one run.
type queryMatcher struct {
        events map[string]bool
        match  *regexp.Regexp
        filter QueryFilter
        // from and to are the resolved window bounds (lazily set once the
        // first record anchors the bare clock arguments).
        from time.Time
        to   time.Time
}

// newQueryMatcher compiles the static filters. The events list and the
// match expression are validated here so the CLI fails fast; the time
// window resolves against the first record inside the scan.
func newQueryMatcher(filter QueryFilter) (*queryMatcher, error) {
        m := &queryMatcher{
                events: nil,
                match:  nil,
                filter: filter,
                from:   time.Time{},
                to:     time.Time{},
        }
        if len(filter.Events) > 0 {
                m.events = make(map[string]bool, len(filter.Events))
                for _, name := range filter.Events {
                        m.events[strings.TrimSpace(name)] = true
                }
        }
        if filter.Match != "" {
                compiled, err := regexp.Compile(filter.Match)
                if err != nil {
                        return nil, fmt.Errorf("%w: %s", errQueryBadRegex, filter.Match)
                }
                m.match = compiled
        }

        return m, nil
}

// anchor resolves the time window against the first record of the
// scan. A record without a parsable timestamp cannot anchor anything.
func (m *queryMatcher) anchor(first record) error {
        base := time.Unix(first.T, 0).UTC()
        var err error
        if m.filter.From != "" {
                if m.from, err = ParseQueryTime(m.filter.From, base); err != nil {
                        return fmt.Errorf("-from: %w", err)
                }
        }
        if m.filter.To != "" {
                if m.to, err = ParseQueryTime(m.filter.To, base); err != nil {
                        return fmt.Errorf("-to: %w", err)
                }
        }

        return nil
}

// matchRecord reports whether the record passes every filter.
func (m *queryMatcher) matchRecord(r record) bool {
        if m.filter.Bot != "" && r.B != m.filter.Bot {
                return false
        }
        if m.events != nil && !m.events[r.E] {
                return false
        }
        if !m.from.IsZero() && r.T < m.from.Unix() {
                return false
        }
        if !m.to.IsZero() && r.T > m.to.Unix() {
                return false
        }
        if m.match != nil && !m.match.MatchString(r.M) && !m.match.MatchString(r.R) {
                return false
        }

        return true
}

// queryStats carries the run summary printed after the records.
type queryStats struct {
        scanned int
        matched int
        printed int
}

// RunQuery streams the journal file once and prints every record that
// passes the filter, with optional time context around the matches.
// The output lands in out; the tool is the grep of the journal, bounded
// in memory whatever the file size (only the context ring and the
// summary counters live).
func RunQuery(path string, filter QueryFilter, out io.Writer) error {
        matcher, err := newQueryMatcher(filter)
        if err != nil {
                return err
        }
        run := &queryRun{
                matcher:     matcher,
                out:         out,
                before:      nil,
                activeUntil: 0,
                stats:       queryStats{scanned: 0, matched: 0, printed: 0},
                limitHit:    false,
        }
        if err := scanJournal(path, run.visit); err != nil {
                return err
        }
        printQuerySummary(out, run.stats)

        return nil
}

// queryRun is the streaming state of one RunQuery pass.
type queryRun struct {
        matcher *queryMatcher
        out     io.Writer
        // before buffers the records of the trailing context window so a
        // match can flush its own preamble.
        before []record
        // activeUntil is the unix second until which context records print
        // after a match (0 when the context mode is off).
        activeUntil int64
        stats       queryStats
        limitHit    bool
}

// visit folds one scanned record.
func (r *queryRun) visit(rec record) error {
        r.stats.scanned++
        if r.limitHit {
                return nil
        }
        if r.stats.scanned == 1 {
                if err := r.matcher.anchor(rec); err != nil {
                        return err
                }
        }
        r.trimBefore(rec.T)
        if r.matcher.matchRecord(rec) {
                r.flushBefore(rec.T)
                r.printRecord(rec, false)
                r.stats.matched++
                if r.matcher.filter.Context > 0 {
                        r.activeUntil = rec.T + int64(r.matcher.filter.Context.Seconds())
                }

                return nil
        }
        if r.activeUntil != 0 && rec.T <= r.activeUntil {
                r.printRecord(rec, true)

                return nil
        }
        if r.matcher.filter.Context > 0 {
                r.before = append(r.before, rec)
        }

        return nil
}

// trimBefore drops the records that fell out of the context window.
func (r *queryRun) trimBefore(now int64) {
        if r.matcher.filter.Context <= 0 {
                return
        }
        cut := now - int64(r.matcher.filter.Context.Seconds())
        drop := 0
        for drop < len(r.before) && r.before[drop].T < cut {
                drop++
        }
        if drop > 0 {
                r.before = append(r.before[:0], r.before[drop:]...)
        }
}

// flushBefore prints the buffered preamble of a match.
func (r *queryRun) flushBefore(matchAt int64) {
        cut := matchAt - int64(r.matcher.filter.Context.Seconds())
        for _, rec := range r.before {
                if rec.T >= cut {
                        r.printRecord(rec, true)
                }
        }
        r.before = r.before[:0]
}

// printRecord renders one output line and keeps the limit honest. The
// prefix marks plain matches (" ") and context lines ("~") so a drill
// down reads like grep -C output.
func (r *queryRun) printRecord(rec record, context bool) {
        if r.matcher.filter.Limit > 0 && r.stats.printed >= r.matcher.filter.Limit {
                r.limitHit = true

                return
        }
        r.stats.printed++
        prefix := " "
        if context {
                prefix = "~"
        }
        _, _ = fmt.Fprintln(r.out, prefix+" "+formatQueryRecord(rec))
}

// formatQueryRecord renders one record as a compact single line: the
// clock time, the bot, the kind and the payload the kind carries.
func formatQueryRecord(r record) string {
        timeText := time.Unix(r.T, 0).UTC().Format("15:04:05")
        bot := r.B
        if bot == "" {
                bot = "-"
        }
        var payload strings.Builder
        switch r.E {
        case kindStory:
                payload.WriteString(r.M)
        case kindLogout:
                fmt.Fprintf(&payload, "emergency logout: %s (login cooldown %s)",
                        r.R, durText(r.Ti))
        case kindSample:
                fmt.Fprintf(&payload, "lv %d xp %d adena %d hp %.0f%% at %d %d phase %s",
                        r.Lv, r.Xp, r.Ad, r.Hp, r.X, r.Y, r.Ph)
        case kindKill:
                fmt.Fprintf(&payload, "%s (lvl %d) in %s, hp left %.0f%%",
                        r.Mob, r.Lvl, durText(r.Dur), r.Hp)
        case kindDeath:
                fmt.Fprintf(&payload, "died at level %d, position %d %d", r.Lv, r.X, r.Y)
        case kindLevel:
                fmt.Fprintf(&payload, "level %d, xp %d", r.Lv, r.Xp)
        case kindTripStart:
                fmt.Fprintf(&payload, "trip starts: %s", r.R)
        case kindTripEnd:
                fmt.Fprintf(&payload, "trip ends: %s (%s)", r.R, durText(r.Dur))
        case kindStall:
                fmt.Fprintf(&payload, "%s stall held %s at %d %d", r.R, durText(r.Dur), r.X, r.Y)
        case kindConnect:
                fmt.Fprintf(&payload, "connect %s %s", r.R, r.M)
        case kindLost:
                fmt.Fprintf(&payload, "lost: %s", r.R)
        case kindShutdown:
                fmt.Fprintf(&payload, "shutdown: %s", r.R)
        case kindSell:
                fmt.Fprintf(&payload, "sold %d items", r.N)
        case kindBuy:
                fmt.Fprintf(&payload, "bought %d items for %s: %s",
                        r.N, groupDigits(r.Cost), r.Items)
        case kindRepath:
                fmt.Fprintf(&payload, "walk re-path, attempt %d", r.N)
        case kindZone:
                fmt.Fprintf(&payload, "zone %s (%s)", r.Mob, r.R)
        default:
                payload.WriteString(r.M)
                if payload.Len() == 0 {
                        payload.WriteString(r.R)
                }
        }

        return fmt.Sprintf("%s [%s] %-10s %s", timeText, bot, r.E, payload.String())
}

// printQuerySummary writes the run footer: how many records the scan
// visited, how many matched and how many printed.
func printQuerySummary(out io.Writer, stats queryStats) {
        fmt.Fprintf(out, "-- %d scanned, %d matched, %d printed\n",
                stats.scanned, stats.matched, stats.printed)
}
