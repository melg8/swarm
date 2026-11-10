# GitHub access from the machine: gh over the stored git credential (status: in progress)

Started 2026-11-10, branch main, issue0.

## Goal

The agent closes, comments on and reads pull requests itself instead of
leaving the GitHub side of every round to the owner. Done means:
`bash tools/gh_token.sh pr list` answers on this host, the five old
navmesh PRs are closed with their verdicts, none of it depends on a
secret pasted into a conversation, and a fresh clone on a clean host
can repeat the setup from the documentation alone.

## Context

Collected 2026-11-10:

- `gh` was not installed; `winget` offers 2.103.3 and `choco` exists,
  but the release zip into `~/go/bin` needs no elevation and needs no
  package manager (that directory is already on PATH with `task`,
  `golangci-lint`, `gopls` and `govulncheck`).
- git pushes through `credential.helper=manager` (Git Credential
  Manager); `git credential fill` for `github.com` answers
  `username=melg8` plus a 40 character OAuth token.
- That token is live: `GET /user` answers 200 with
  `X-OAuth-Scopes: gist, repo, workflow` and `GET /repos/melg8/swarm`
  answers 200, so it covers pull request and issue writes. `read:org`
  is the only missing scope and only org-wide queries need it.
- Storing the token through `gh auth login --with-token` would write a
  plain text `~/.config/gh/hosts.yml` next to the DPAPI encrypted
  copy in the Windows Credential Manager - a second home for the same
  secret, with weaker protection.

## Progress

### 2026-11-10 - the wrapper, the install note and the housekeeping

- `tools/gh_token.sh` reads the credential from the helper per
  invocation, exports it as `GH_TOKEN` and execs `gh`; it prints no
  token, writes nothing to disk and fails with a hint when the helper
  holds nothing.
- `docs/deployment.md` gains the "GitHub access from the machine"
  section (the release zip drop into `~/go/bin`, the rationale against
  `gh auth login`, the wrapper invocations); `AGENTS.md` points at it
  from the Git conventions.
- Housekeeping done through the wrapper, each with the commit
  reference: closed #26 (`77f6e6c4`), #55 (`69a88442`), #67
  (`f4c7f5db` + `60869338`) as landed, #61 as rejected by the
  zero-drift gate, #16 as already on main; commented #62 with the
  pending corpus answer contract decision; closed issue #11 (the size
  investigation landed) and issue #59 (the dual pack view landed); #27
  was already closed.

## Status

Access works end to end. Next: the final verification pass, the commit
and the archive of this fragment.
