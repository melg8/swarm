#!/usr/bin/env python3
# SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
#
# SPDX-License-Identifier: MIT
#
# journal_stats.py summarizes one or more session journals into the
# comparison table a measurement round reads: the kills, the deaths,
# the stalls, the logouts, the town trips, the money the fleet
# accumulated and the experience it gained, per bot and for the fleet,
# with the phase time share of the state samples.
#
# The journal is the record of record of a live run (internal/swarm/
# session writes it, docs/deployment.md names it): the console log is
# truncated by a restart while the journal keeps every kill, death and
# sample, so an A/B round compares two journals instead of two logs.
#
# Usage:
#   python tools/journal_stats.py logs/session-20261115-030925-33068.jsonl
#   python tools/journal_stats.py logs/session-*.jsonl --phases
#
# The money figure is the net purse gain (end minus start): the
# pickups and the sales come in, the gear and the scrolls go out, and
# the difference is what the fleet actually kept. The buys are
# reported beside it, so a spending-heavy run reads honestly.

import argparse
import collections
import json
import sys
import time

def read_journal(path):
    """Returns the parsed records of one journal, skipping broken lines."""
    records = []
    with open(path, encoding="utf-8", errors="replace") as handle:
        for line in handle:
            line = line.strip()
            if not line:
                continue
            try:
                records.append(json.loads(line))
            except json.JSONDecodeError:
                continue

    return records


def clock(stamp):
    return time.strftime("%H:%M", time.localtime(stamp))


class BotStats:
    """The counters of one bot of the journal."""

    def __init__(self):
        self.kills = 0
        self.deaths = 0
        self.stalls = 0
        self.logouts = 0
        self.trips = 0
        self.sells = 0
        self.buy_cost = 0
        self.levels = 0
        self.first_sample = None
        self.last_sample = None
        self.phase_seconds = collections.Counter()
        self.money_sum = 0
        self.exp_sum = 0
        self.hours_sum = 0.0
        self.bots = 0

    def feed(self, record):
        kind = record.get("e", "")
        stamp = record.get("t", 0)
        if kind == "kill":
            self.kills += 1
        elif kind == "death":
            self.deaths += 1
        elif kind == "stall":
            self.stalls += 1
        elif kind == "logout":
            self.logouts += 1
        elif kind == "trip-start":
            self.trips += 1
        elif kind == "sell":
            self.sells += 1
        elif kind == "buy":
            self.buy_cost += int(record.get("cost", 0))
        elif kind == "level":
            self.levels += 1
        elif kind == "s":
            self.sample(record, stamp)

    def sample(self, record, stamp):
        if self.last_sample is not None:
            gap = stamp - self.last_sample["t"]
            phase = self.last_sample.get("ph", "?")
            if 0 < gap <= 120:
                self.phase_seconds[phase] += gap
        self.first_sample = self.first_sample or record
        self.last_sample = record

    def money(self):
        if not self.first_sample or not self.last_sample:
            return 0

        return self.last_sample["ad"] - self.first_sample["ad"]

    def exp(self):
        if not self.first_sample or not self.last_sample:
            return 0

        return self.last_sample["xp"] - self.first_sample["xp"]

    def hours(self):
        if not self.first_sample or not self.last_sample:
            return 0.0

        return (self.last_sample["t"] - self.first_sample["t"]) / 3600


def summarize(path, show_phases):
    records = read_journal(path)
    bots = collections.defaultdict(BotStats)
    build = ""
    for record in records:
        kind = record.get("e", "")
        if kind == "build":
            build = record.get("v", "")
            continue
        bot = record.get("b", "")
        if not bot:
            continue
        bots[bot].feed(record)
    if not bots:
        print(f"{path}: no bot records")

        return

    stamps = [r.get("t", 0) for r in records if r.get("t")]
    window = (max(stamps) - min(stamps)) / 3600 if stamps else 0
    print(f"journal: {path}")
    if build:
        print(f"build: {build}")
    print(f"window: {clock(min(stamps))} - {clock(max(stamps))} "
          f"({window:.2f} h)")

    header = ("bot       kills deaths stalls logout trips sells buys"
              "   money   money/h     exp    exp/h lvl")
    print(header)
    total = BotStats()
    for name in sorted(bots):
        bot = bots[name]
        hours = bot.hours()
        money_per_hour = bot.money() / hours if hours else 0
        exp_per_hour = bot.exp() / hours if hours else 0
        print(f"{name:<9} {bot.kills:5d} {bot.deaths:6d} {bot.stalls:6d} "
              f"{bot.logouts:6d} {bot.trips:5d} {bot.sells:5d} "
              f"{bot.buy_cost:5d} {bot.money():7d} {money_per_hour:9.0f} "
              f"{bot.exp():7d} {exp_per_hour:8.0f} {bot.levels:3d}")
        total.kills += bot.kills
        total.deaths += bot.deaths
        total.stalls += bot.stalls
        total.logouts += bot.logouts
        total.trips += bot.trips
        total.sells += bot.sells
        total.buy_cost += bot.buy_cost
        total.levels += bot.levels
        total.money_sum += bot.money()
        total.exp_sum += bot.exp()
        total.hours_sum += hours
        total.bots += 1
        total.phase_seconds.update(bot.phase_seconds)

    # The fleet rate divides the summed money by the summed bot hours: the
    # fleet adena per hour of the whole process is that figure times the
    # number of bots.
    per_bot_money = total.money_sum / total.hours_sum \
        if total.hours_sum else 0
    per_bot_exp = total.exp_sum / total.hours_sum if total.hours_sum else 0
    money_per_hour = per_bot_money * total.bots
    kills_per_hour = total.kills / total.hours_sum * total.bots \
        if total.hours_sum else 0
    print(f"{'fleet':<9} {total.kills:5d} {total.deaths:6d} "
          f"{total.stalls:6d} {total.logouts:6d} {total.trips:5d} "
          f"{total.sells:5d} {total.buy_cost:5d} {total.money_sum:7d} "
          f"{money_per_hour:9.0f} {total.exp_sum:7d} {per_bot_exp:8.0f} "
          f"{total.levels:3d}")
    print(f"fleet per bot-hour: {per_bot_money:.0f} money, "
          f"{total.kills / total.hours_sum if total.hours_sum else 0:.0f} "
          f"kills (fleet rate {money_per_hour:.0f} money/h, "
          f"{kills_per_hour:.0f} kills/h)")

    if show_phases:
        spent = sum(total.phase_seconds.values())
        if spent:
            print("phase time:")
            for phase, seconds in total.phase_seconds.most_common():
                print(f"  {phase:<10} {seconds / 3600:6.2f} h "
                      f"{100 * seconds / spent:5.1f}%")


def main():
    parser = argparse.ArgumentParser(
        description="summarize the swarm session journals")
    parser.add_argument("journals", nargs="+", help="the journal files")
    parser.add_argument("--phases", action="store_true",
                        help="print the phase time share of the samples")
    args = parser.parse_args()
    for index, path in enumerate(args.journals):
        if index:
            print()
        try:
            summarize(path, args.phases)
        except OSError as err:
            print(f"{path}: {err}", file=sys.stderr)

            return 1

    return 0


if __name__ == "__main__":
    sys.exit(main())
