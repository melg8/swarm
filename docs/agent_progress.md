## Active task: T-012 the html dialog link parser

Started: 2026-09-12 08:15 UTC. Branch: `feature/proxy-server`.
Commits as melg8. Agent label: agent-quest. Taken after T-011
closed (T-009/T-010 still wait on T-008).

### Progress

- 08:15-08:25 UTC: implemented and verified:
  - packets/from_game_server/html_links.go: ParseHTMLLinks
    extracts the bypass links of a server dialog page (the command
    per <a action="bypass ..."> with the -h prefix stripped and
    trimmed, plus the visible link text), mirroring
    HtmlUtil.buildHtmlBypassCache (the case-insensitive "=\"bypass "
    match on the lowercased html, the original casing preserved,
    the unterminated attribute ends the scan, the 128 link cap);
  - 8 unit tests: the real Sorius quest page, the Rains class
    master page (the -h strip, the multi-link order), the
    case-insensitive attribute, the $ parameter marker, the command
    trim, the non-bypass actions, the empty/broken pages, the cap;
  - verification against the real datapack: the parser walked every
    page of the Q00406 script, the ElfHumanFighterChange1 master
    pages and the Mirabel teleporter page - 112 links, commands and
    texts exact (the %objectId% of the file form stays as the raw
    token - the server replaces it at send time, the packet form
    arrives resolved);
  - go build ./... green, the package tests green,
    golangci-lint run --new: 0 issues (the HtmlLink -> HTMLLink
    naming and the TrimPrefix staticcheck findings fixed);
  - docs/quest_protocol.md: the follow-up list marks the link
    parser landed (and the T-011 journal parser landed).
- Status: done (2026-09-12, 08:25 UTC).

## Active task: T-013 the open dialog section of the tracker

Started: 2026-09-12 08:18 UTC. Branch: `feature/proxy-server`.
Commits as melg8. Agent label: agent-quest.

### Progress

- 08:18-08:27 UTC: implemented and verified:
  - state/dialog.go: the open dialog page of the NPC_HTML scope
    (the npc origin, the item id, the links) - ApplyDialog replaces
    the page whole (one page per scope, the same semantics as the
    server cache), the accessors (DialogLinks defensive copy,
    DialogOrigin) and IsDialogCommand mirroring
    Player.validateHtmlAction (the exact match or the trimmed
    prefix of the '$' variable parameter link); ClearDialog and the
    ResetSession clear (the relogin starts with no dialog);
  - 7 unit tests: the Sorius quest page pins, the page replacement
    (the old links stop validating), the '$' parameter prefix rule
    (the prefix must match, the suffix commands pass, the near miss
    fails), the defensive copy (no aliasing of the applied page or
    the returned slice), the session reset and the explicit clear;
  - go build ./... green, the state and packet package tests green,
    golangci-lint run --new: 0 issues (the '$' prefix fix mirrors
    the Java exactly - the marker is stripped and the remainder
    trimmed before the prefix match; the exhaustruct findings
    resolved with the repo's zero-view nolint pattern);
  - the consumer wiring (the 0x1B dispatcher case feeding
    ApplyDialog) stays with T-008 (their scope: connection/).
- Status: done (2026-09-12, 08:27 UTC).

## Session handover notes (agent-quest, 2026-09-12 08:30 UTC)

The quest reading chain of this session (all live verified or
datapack verified, all pushed):

- T-004 (done): docs/quest_protocol.md - the quest protocol map
  (the engine, the dialog packets, the bypass validation, the two
  class transfer chains, the gatekeeper geography, the follow-up
  list).
- T-011 (done): the QuestList 0x98 parser + the state quest journal
  + the dispatcher wiring ("Quest journal with 0 quests, 0 quest
  items" at the enter world second, live).
- T-012 (done): ParseHTMLLinks - the bypass link extraction of a
  dialog page (112 links verified against the real datapack pages).
- T-013 (done): state/dialog.go - the open dialog page of the
  tracker with the IsDialogCommand mirror of
  Player.validateHtmlAction.

For the next agent:

- The dialog pieces now compose: the connection layer of T-008
  exposes GameClient.LastHTMLMessage() (the parsed html string of
  the last NpcHtmlMessage); the tracker exposes ApplyDialog (the
  links + the origin + the validation mirror); the walker should
  feed the tracker from the apply path (or read both) before any
  bypass is sent.
- T-009/T-010 wait on T-008 (in progress - the dispatcher wiring
  landed at 08:20, the live Mirabel -> Gludio drive remains); once
  it closes, the zone registry generation and the band gear
  catalogs open up.
- The M2 quest rounds that follow: the dialog walker (pick links by
  text through the tracker), the Q00406/Q00407 chains as data, the
  class-transfer acceptance scenario (the T-003 level milestone
  scenario - closed by another agent at 08:15 - is the injection
  vehicle).
- The quest journal, the dialog section and the link parser carry
  unit tests next to them; the live facts (the world entry push,
  the 813 byte merchant pages, the 2.5 s kill delay) live in
  docs/quest_protocol.md.

## Active task: T-014 the quest dialog walker engine

Started: 2026-09-12 08:45 UTC. Branch: `feature/proxy-server`.
Commits as melg8. Agent label: quest-walker-e3f8.

Goal: the generic dialog walker the M2 quest brain runs on (the
follow-up 4 engine half of docs/quest_protocol.md): the two-click
talk entry, the new-page wait with content change detection, the
hunt-side feed of the tracker dialog section and the link-by-text
bypass walk with IsDialogCommand validation.

Constraints: new files only in internal/swarm/hunt/ (T-008 owns
the live edits of loop.go and the gatekeeper files); the loop
phase wiring is NOT this task (the M2 acceptance round composes
it); no behavior change of the running bot until the wiring.

Acceptance: unit tests on the real Q00406 and class master page
goldens (the fakeGame stub) green, golangci-lint run --new clean,
a live round trip against a village npc of the deployed stack
(click -> html -> link -> bypass -> next html) recorded in the dev
log, docs/quest_protocol.md follow-up list updated.

### Progress

- 08:45 UTC: claimed (BACKLOG T-014 in_progress + this entry).

## Session close: the M1 evidence runs (2026-09-12, zai-agent)

The session closed T-002 (the stagnation watch), T-003 (the level
milestone scenario) and the xpPerHour double-count fix of the metrics
trail; the last minutes added one more M1 evidence row:

- Live soak 15 min: PASS (temp7, level 3 -> 4, 5533 xp/h, 0 deaths,
  0 stuck events, the row in runs/metrics.jsonl) - the corrected
  metrics math on a real long run, the stagnation guard and the hunt
  watch both quiet on a healthy session.
- The M1 trail so far: the 2 min soak smoke (1 -> 2), the 15 min
  soak (3 -> 4), the level milestone 10 -> 11 and the env run
  2 -> 3 - all PASS. The 8 hour M1 proof remains the operator run
  (SWARM_SOAK_MINUTES=480) - it cannot fit a 2h agent session.
- Operational lesson recorded: a long acceptance run piped through
  grep to the tool call dies on the 1 MiB result frame limit - the
  next long run must redirect the bot stdout to a file (the
  tools/mobius_e2e.sh pattern) and poll the metrics file instead.
- The queue check at the close: every todo task (T-009, T-010)
  waits on T-008 (in_progress by another agent); nothing claimable
  without fighting over work.

Status: session complete - the next agent starts from the BACKLOG
queue (T-008 unblocks T-009/T-010) or the M1 operator soak run.

