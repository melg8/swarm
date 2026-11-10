#!/usr/bin/env python3
# SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
#
# SPDX-License-Identifier: MIT
#
# comment_audit.py reports the comment debt of the Go tree against the
# rule in AGENTS.md ("Comments: the fact, not the story"): a comment
# that retells the code, the round or the old behavior costs a reader
# more than it pays.
#
# It does not judge prose. It counts what the rule makes countable:
# the comment share per file, the blocks longer than --block lines and
# the history markers a current-fact comment never needs (a date, a
# round number, an issue id, "legacy", the shouting words).
#
# Usage:
#   python tools/comment_audit.py              # the summary
#   python tools/comment_audit.py --top 25     # a longer file list
#   python tools/comment_audit.py --tests      # include _test.go
#   python tools/comment_audit.py --markers    # the marker hits
#
# The target of the sweep: the comment share under 10 percent, no
# block above 8 lines outside a package doc, and the marker count
# falling with every round. The script exits 0 on a report, so it can
# run in the middle of a cleanup; pass --gate to make it fail when the
# share is above --max-share.

import argparse
import os
import re
import sys

COMMENT = "//"
MARKER_PATTERNS = [
    ("date", re.compile(r"\b20\d\d-\d\d-\d\d\b")),
    ("issue", re.compile(r"\bissue #\d+\b")),
    ("round", re.compile(r"\bround[- ]\d+\b")),
    ("legacy", re.compile(r"\blegacy\b")),
    ("shout", re.compile(r"\b[A-Z]{4,}\b")),
]
GO_DIRS = ["internal", "cmd"]


def scan(path, block_limit):
    lines = open(path, encoding="utf-8").read().split("\n")
    comments = 0
    markers = []
    blocks = []
    run = 0
    run_start = 0
    for number, line in enumerate(lines, start=1):
        stripped = line.strip()
        if stripped.startswith(COMMENT):
            comments += 1
            if run == 0:
                run_start = number
            run += 1
            for name, pattern in MARKER_PATTERNS:
                if pattern.search(stripped):
                    markers.append((number, name))
        else:
            if run >= block_limit:
                blocks.append((run, run_start))
            run = 0
    if run >= block_limit:
        blocks.append((run, run_start))

    return {
        "path": path,
        "lines": len(lines),
        "comments": comments,
        "blocks": blocks,
        "markers": markers,
    }


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--top", type=int, default=12)
    parser.add_argument("--block", type=int, default=8)
    parser.add_argument("--tests", action="store_true")
    parser.add_argument("--markers", action="store_true")
    parser.add_argument("--max-share", type=float, default=10.0)
    parser.add_argument("--gate", action="store_true")
    args = parser.parse_args()

    rows = []
    for root_dir in GO_DIRS:
        for root, dirs, files in os.walk(root_dir):
            dirs[:] = sorted(d for d in dirs if d not in (".git",))
            for name in sorted(files):
                if not name.endswith(".go"):
                    continue
                if not args.tests and name.endswith("_test.go"):
                    continue
                rows.append(scan(
                    os.path.join(root, name).replace(os.sep, "/"), args.block))

    total_comments = sum(r["comments"] for r in rows)
    total_lines = sum(r["lines"] for r in rows)
    share = 100.0 * total_comments / max(total_lines, 1)
    block_count = sum(len(r["blocks"]) for r in rows)
    marker_count = sum(len(r["markers"]) for r in rows)

    print("comment lines %d of %d (%.1f%%), files %d"
          % (total_comments, total_lines, share, len(rows)))
    print("blocks >= %d lines: %d" % (args.block, block_count))
    print("history markers: %d" % marker_count)
    print()

    rows.sort(key=lambda r: r["comments"], reverse=True)
    print("by comment lines:")
    for row in rows[:args.top]:
        row_share = 100.0 * row["comments"] / max(row["lines"], 1)
        print("%6d %5.1f%% %s" % (row["comments"], row_share, row["path"]))

    longest = sorted(
        ((run, start, row["path"]) for row in rows for run, start in row["blocks"]),
        reverse=True,
    )
    print()
    print("longest blocks:")
    for run, start, path in longest[:args.top]:
        print("%5d lines %s:%d" % (run, path, start))

    if args.markers:
        print()
        print("markers:")
        for row in rows:
            for number, kind in row["markers"][:20]:
                print("%s:%d %s" % (row["path"], number, kind))

    if args.gate and share > args.max_share:
        print()
        print("comment share %.1f%% is above the %.1f%% target"
              % (share, args.max_share), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
