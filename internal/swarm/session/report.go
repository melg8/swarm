// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package session

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// The report shape. The rendered page stays within tens of kilobytes
// for a 24 hour run: the aggregates below carry the numbers, the story
// tail carries the last decisions and the journal file path points the
// deeper investigation at the full record trail.
const (
	storyTailLen     = 60
	histogramBuckets = 6
)

// renderReport builds the compact plain text session report of one
// bot. The sections answer the owner questions directly: how much was
// farmed and spent (economy), how often the bot stalled (stalls), and
// which fights went badly (combat, deaths).
func renderReport(path string, a *botAgg, live *LiveView) string {
	b := &strings.Builder{}
	b.WriteString("swarm session report\n")
	writeReportPeriod(b, path, a)
	writeReportLive(b, live)
	writeReportJourney(b, a)
	writeReportEconomy(b, a)
	writeReportCombat(b, a)
	writeReportDeaths(b, a)
	writeReportDowntime(b, a)
	writeReportStalls(b, a)
	writeReportStory(b, a)

	return b.String()
}

// writeReportPeriod writes the header block: the journal provenance
// and the covered window.
func writeReportPeriod(b *strings.Builder, path string, a *botAgg) {
	fmt.Fprintf(b, "journal: %s\n", path)
	if a.first == 0 {
		b.WriteString("period: no events recorded\n\n")

		return
	}
	fmt.Fprintf(b, "period: %s .. %s (%s)\n",
		utc(a.first), utc(a.last), durText(float64(a.last-a.first)))
	fmt.Fprintf(b, "bot: %s (connects %d, lost %d)\n\n", a.id, a.connects, a.lost)
}

// writeReportLive writes the point-in-time block of the running bot
// (the offline CLI renders without it).
func writeReportLive(b *strings.Builder, live *LiveView) {
	if live == nil {
		return
	}
	fmt.Fprintf(b, "live now: level %d, exp %.1f%%, hp %.0f%%, adena %s\n",
		live.Level, live.ExpPercent, live.Health, groupDigits(live.Adena))
	fmt.Fprintf(b, "  status %s, phase %s, position %d %d\n\n",
		live.Status, live.Phase, live.X, live.Y)
}

// writeReportJourney writes the level curve and the experience pace.
func writeReportJourney(b *strings.Builder, a *botAgg) {
	b.WriteString("== character journey ==\n")
	if len(a.levels) == 0 {
		b.WriteString("  no level ups recorded\n\n")

		return
	}
	first := a.levels[0]
	last := a.levels[len(a.levels)-1]
	hours := timeSpanHours(first.t, last.t)
	pace := ""
	if hours > 0 {
		pace = fmt.Sprintf(" (%.2f levels/h)", float64(last.lv-first.lv)/hours)
	}
	fmt.Fprintf(b, "  %s -> level %d at %s%s\n", groupDigits(int64(first.lv)),
		last.lv, utc(last.t), pace)
	marks := make([]string, 0, len(a.levels))
	for _, mark := range a.levels {
		marks = append(marks, fmt.Sprintf("%d@%s", mark.lv, hhmm(mark.t)))
	}
	fmt.Fprintf(b, "  marks: %s\n", strings.Join(marks, ", "))
	if a.prev != nil && a.prev.seen && a.hours != nil {
		writeReportXPBuckets(b, a)
	}
	b.WriteString("\n")
}

// writeReportXPBuckets writes the hourly experience and adena deltas.
func writeReportXPBuckets(b *strings.Builder, a *botAgg) {
	keys := hourKeys(a.hours)
	fmt.Fprintf(b, "  hour | level | xp gain | adena | kills | deaths\n")
	for _, key := range keys {
		bucket := a.hours[key]
		xp := bucket.xpLast - bucket.xpFirst
		ad := bucket.adLast - bucket.adFirst
		fmt.Fprintf(b, "  %s | %d->%d | +%s | %s%s | %d | %d\n",
			hhmm(int64(key)*3600), bucket.lvFirst, bucket.lvLast,
			groupDigits(xp), signPrefix(ad), groupDigits(abs64(ad)),
			bucket.kills, bucket.deaths)
	}
}

// writeReportEconomy writes the money trail: the adena curve, the
// itemized purchases and the sell volume.
func writeReportEconomy(b *strings.Builder, a *botAgg) {
	b.WriteString("== economy ==\n")
	if a.prev == nil || !a.prev.seen {
		b.WriteString("  no samples recorded\n\n")

		return
	}
	earned := int64(0)
	for _, key := range hourKeys(a.hours) {
		bucket := a.hours[key]
		earned += bucket.adLast - bucket.adFirst
	}
	spend := int64(0)
	for _, buy := range a.buys {
		spend += buy.cost
	}
	fmt.Fprintf(b, "  adena: %s now, %s%s net over the samples (%.0f/h)\n",
		groupDigits(a.prev.ad), signPrefix(earned), groupDigits(abs64(earned)),
		float64(earned)/timeSpanHours(a.first, a.last))
	fmt.Fprintf(b, "  purchases: %d batches for -%s adena\n",
		len(a.buys), groupDigits(spend))
	for _, buy := range a.buys {
		fmt.Fprintf(b, "    %s %s x%d for %s\n", hhmm(buy.t), buy.items,
			buy.count, groupDigits(buy.cost))
	}
	fmt.Fprintf(b, "  sells: %d batches, %d items offered\n\n",
		a.sellBatch, a.sellItems)
}

// writeReportCombat writes the kill output, the per-mob statistics, the
// fight duration histogram and the slowest fights.
func writeReportCombat(b *strings.Builder, a *botAgg) {
	b.WriteString("== kills & combat ==\n")
	hours := timeSpanHours(a.first, a.last)
	if hours <= 0 {
		hours = 1
	}
	fmt.Fprintf(b, "  kills: %d (%.0f/h), mobs known: %d\n",
		a.kills, float64(a.kills)/hours, len(a.mobs))
	if a.kills > 0 {
		writeReportDuration(b, a)
		writeReportMobs(b, a)
	}
	b.WriteString("\n")
}

// writeReportDuration writes the fight duration statistics.
func writeReportDuration(b *strings.Builder, a *botAgg) {
	p50, p90, p99 := percentiles(a.durHist, a.kills)
	fmt.Fprintf(b, "  fight duration: median %.1fs, p90 %.1fs, p99 %.1fs\n",
		p50, p90, p99)
	if len(a.slowFight) > 0 {
		lines := make([]string, 0, len(a.slowFight))
		for _, fight := range a.slowFight {
			lines = append(lines, fmt.Sprintf("%s %s (lvl %d) %.0fs at %s",
				"", fight.mob, fight.lvl, fight.dur, hhmm(fight.t)))
		}
		fmt.Fprintf(b, "  slowest: %s\n", strings.Join(lines, "; "))
	}
}

// writeReportMobs writes the per-mob aggregates sorted by kill count.
func writeReportMobs(b *strings.Builder, a *botAgg) {
	names := make([]string, 0, len(a.mobs))
	for name := range a.mobs {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		return a.mobs[names[i]].count > a.mobs[names[j]].count
	})
	limit := 12
	if len(names) < limit {
		limit = len(names)
	}
	for _, name := range names[:limit] {
		mob := a.mobs[name]
		avg := mob.durSum / float64(mob.count)
		hp := 0.0
		if mob.hpHits > 0 {
			hp = mob.hpBelow / float64(mob.hpHits)
		}
		fmt.Fprintf(b, "  %-24s %4d kills, avg %.1fs, max %.0fs, hp left %.0f%%\n",
			name, mob.count, avg, mob.durMax, hp)
	}
}

// writeReportDeaths writes the death list with the positions.
func writeReportDeaths(b *strings.Builder, a *botAgg) {
	b.WriteString("== deaths ==\n")
	if len(a.deaths) == 0 {
		b.WriteString("  none\n\n")

		return
	}
	for _, death := range a.deaths {
		fmt.Fprintf(b, "  %s level %d at %d %d\n",
			hhmm(death.t), death.lv, death.x, death.y)
	}
	if len(a.deaths) == maxDeaths {
		b.WriteString("  (the list keeps the newest entries)\n")
	}
	b.WriteString("\n")
}

// writeReportDowntime writes the trip and offline time attribution.
func writeReportDowntime(b *strings.Builder, a *botAgg) {
	b.WriteString("== downtime ==\n")
	hours := timeSpanHours(a.first, a.last)
	fmt.Fprintf(b, "  town trips: %d (%s total, avg %s, longest %s: %q)\n",
		a.trips, durText(a.tripSec), durText(a.tripSec/max(float64(a.trips), 1)),
		durText(a.tripMax.dur), a.tripMax.reason)
	if len(a.tripReason) > 0 {
		reasons := make([]string, 0, len(a.tripReason))
		for reason, count := range a.tripReason {
			reasons = append(reasons, fmt.Sprintf("%q x%d", reason, count))
		}
		sort.Strings(reasons)
		fmt.Fprintf(b, "  trip reasons: %s\n", strings.Join(reasons, ", "))
	}
	if len(a.gaps) > 0 {
		total := 0.0
		for _, gap := range a.gaps {
			total += gap.sec
		}
		fmt.Fprintf(b, "  offline gaps: %d (%s total)\n",
			len(a.gaps), durText(total))
	}
	writePhaseShare(b, a, hours)
	b.WriteString("\n")
}

// writePhaseShare writes the sampled phase share of the window.
func writePhaseShare(b *strings.Builder, a *botAgg, hours float64) {
	hunt, town, other := 0.0, 0.0, 0.0
	for _, bucket := range a.hours {
		hunt += bucket.huntSec
		town += bucket.townSec
		other += bucket.otherS
	}
	total := hunt + town + other
	if total <= 0 || hours <= 0 {
		return
	}
	fmt.Fprintf(b,
		"  phase share (sampled): hunt %.0f%%, town %.0f%%, other %.0f%%\n",
		hunt/total*100, town/total*100, other/total*100)
}

// writeReportStalls writes the stagnation, freeze and re-path
// evidence: the direct answer to "how many stucks happened".
func writeReportStalls(b *strings.Builder, a *botAgg) {
	b.WriteString("== stalls & stucks ==\n")
	fmt.Fprintf(b, "  xp stalls: %d\n", len(a.xpStalls))
	for _, stall := range a.xpStalls {
		fmt.Fprintf(b, "    %s held %s\n", hhmm(stall.t), durText(stall.sec))
	}
	fmt.Fprintf(b, "  position stalls: %d\n", len(a.posStalls))
	for _, stall := range a.posStalls {
		fmt.Fprintf(b, "    %s held %s at %d %d\n",
			hhmm(stall.t), durText(stall.sec), stall.x, stall.y)
	}
	if len(a.freezes) > 0 {
		fmt.Fprintf(b, "  sample freezes: %d\n", len(a.freezes))
		for _, freeze := range a.freezes {
			fmt.Fprintf(b, "    %s at %d %d\n",
				hhmm(freeze.t), freeze.x, freeze.y)
		}
	}
	fmt.Fprintf(b, "  walk re-paths: %d\n", a.repaths)
	fmt.Fprintf(b, "  story muted by the flood cap: %d\n\n", a.storyMuted)
}

// writeReportStory writes the newest story lines (the hunt decisions
// and the game events of the last minutes).
func writeReportStory(b *strings.Builder, a *botAgg) {
	b.WriteString("== session story (newest first) ==\n")
	start := len(a.ring) - storyTailLen
	if start < 0 {
		start = 0
	}
	for i := len(a.ring) - 1; i >= start; i-- {
		line := a.ring[i]
		fmt.Fprintf(b, "  %s %s\n", hhmm(line.t), line.msg)
	}
	b.WriteString("\n")
}

// hourKeys returns the sorted hour bucket keys.
func hourKeys(hours map[int32]*hourBucket) []int32 {
	keys := make([]int32, 0, len(hours))
	for key := range hours {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	return keys
}

// percentiles reads the median and the 90th and 99th percentile off
// the fight duration histogram.
func percentiles(hist [62]int32, total int) (float64, float64, float64) {
	if total == 0 {
		return 0, 0, 0
	}
	half, ninth, ninetyNinth := total/2, total*9/10, total*99/100
	p50, p90, p99 := 0, 0, 0
	seen := 0
	for bin, count := range hist {
		seen += int(count)
		if p50 == 0 && seen > half {
			p50 = bin
		}
		if p90 == 0 && seen > ninth {
			p90 = bin
		}
		if p99 == 0 && seen > ninetyNinth {
			p99 = bin
		}
	}

	return float64(p50), float64(p90), float64(p99)
}

// timeSpanHours returns the hours between two unix seconds (at least
// one so the rates stay finite).
func timeSpanHours(from int64, to int64) float64 {
	if to <= from {
		return 1
	}

	return float64(to-from) / 3600
}

// utc renders a unix second as RFC3339 UTC.
func utc(t int64) string {
	return time.Unix(t, 0).UTC().Format(time.RFC3339)
}

// hhmm renders a unix second as HH:MM UTC.
func hhmm(t int64) string {
	return time.Unix(t, 0).UTC().Format("15:04")
}

// durText renders a duration in seconds as a compact h/m/s string.
func durText(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	if sec < 60 {
		return fmt.Sprintf("%.0fs", sec)
	}
	if sec < 3600 {
		return fmt.Sprintf("%.0fm%.0fs", sec/60, float64(int64(sec)%60))
	}

	return fmt.Sprintf("%.0fh%.0fm", sec/3600, float64(int64(sec)%3600)/60)
}

// groupDigits renders an integer with thin space groups (12345 ->
// "12 345"), the reading convention of the repository docs.
func groupDigits(v int64) string {
	digits := itoa(abs64(v))
	if len(digits) <= 4 {
		return digits
	}
	var parts []string
	for len(digits) > 3 {
		parts = append([]string{digits[len(digits)-3:]}, parts...)
		digits = digits[:len(digits)-3]
	}
	parts = append([]string{digits}, parts...)

	return strings.Join(parts, " ")
}

// signPrefix renders the sign of a delta for the report lines.
func signPrefix(v int64) string {
	if v < 0 {
		return "-"
	}

	return "+"
}

// abs64 returns the absolute value of an int64.
func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}

	return v
}
