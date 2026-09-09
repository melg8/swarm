// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Package version pins the build identity of the running binary: the
// git branch, the commit, the tree state and the build time. The
// build scripts bake the values in at link time with -ldflags -X; a
// plain go build or go run falls back to the VCS stamp Go embeds into
// binaries built from a git repository and to the .git/HEAD of the
// working directory for the branch. Every artifact that must match a
// code state - the state dump of the web UI, the startup log line of
// the bot - renders the identity through Identity, so a live problem
// report always tells which exact code produced it.
package version

import (
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
)

// Link-time identity fields, set by the -ldflags -X of the build
// scripts. Empty when the binary was built without them.
var (
	// Branch is the git branch name of the build.
	Branch string

	// Commit is the short git hash of the build.
	Commit string

	// Dirty is "true" when the working tree had uncommitted changes
	// at build time, "false" otherwise.
	Dirty string

	// BuildTime is the RFC3339 UTC timestamp of the build.
	BuildTime string
)

// Identity renders the build identity line of the running binary:
// the branch, the commit with the tree state and the build time. The
// link-time fields win, the VCS stamp of the binary and the .git/HEAD
// of the working directory fill the gaps, unknown fields drop out.
func Identity() string {
	branch, commit, dirty, built := Branch, Commit, Dirty, BuildTime
	if stamp, ok := readVCSStamp(); ok {
		if commit == "" {
			commit = stamp.revision
		}
		if dirty == "" {
			dirty = stamp.modified
		}
		if built == "" {
			built = stamp.time
		}
	}
	if branch == "" {
		branch = worktreeBranch(".")
	}

	return render(branch, commit, dirty, built)
}

// vcsStamp holds the VCS settings Go embeds into binaries built from
// a git repository.
type vcsStamp struct {
	revision string
	modified string
	time     string
}

// readVCSStamp reads the vcs.* build settings: go build stamps the
// revision, the modified flag and the commit time unless
// -buildvcs=false was passed.
func readVCSStamp() (vcsStamp, bool) {
	var stamp vcsStamp
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return stamp, false
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			stamp.revision = shortHash(setting.Value)
		case "vcs.modified":
			stamp.modified = setting.Value
		case "vcs.time":
			stamp.time = setting.Value
		}
	}

	return stamp, stamp.revision != ""
}

// shortHash trims a git hash to the 7 character prefix the git log
// output carries.
func shortHash(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}

	return hash
}

// worktreeBranch resolves the branch name of the git worktree rooted
// at dir: .git/HEAD holds "ref: refs/heads/<name>" on a branch and a
// plain hash on a detached HEAD, a linked worktree keeps a
// "gitdir: <path>" pointer file instead of the directory. It returns
// "" whenever anything is off - a stale branch hint is worse than
// none.
func worktreeBranch(dir string) string {
	head, err := os.ReadFile(filepath.Join(dir, ".git", "HEAD"))
	if err != nil {
		head, err = readGitDirHead(dir)
	}
	if err != nil {
		return ""
	}

	return branchOfHead(string(head))
}

// readGitDirHead follows the "gitdir: <path>" pointer file of a
// linked worktree to its HEAD.
func readGitDirHead(dir string) ([]byte, error) {
	pointer, err := os.ReadFile(filepath.Join(dir, ".git"))
	if err != nil {
		return nil, err
	}
	gitdir := strings.TrimSpace(
		strings.TrimPrefix(string(pointer), "gitdir:"))
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(dir, gitdir)
	}

	//nolint:gosec // G703: the gitdir pointer comes from the .git
	// file of the worktree being inspected - a local build hint, not
	// a user-supplied path.
	return os.ReadFile(filepath.Join(gitdir, "HEAD"))
}

// branchOfHead extracts the branch name out of a HEAD file content,
// "" for a detached HEAD.
func branchOfHead(head string) string {
	head = strings.TrimSpace(head)
	const refPrefix = "ref: refs/heads/"
	if !strings.HasPrefix(head, refPrefix) {
		return ""
	}

	return strings.TrimPrefix(head, refPrefix)
}

// render assembles the identity line out of the resolved fields:
// empty fields drop out, an empty input reports the missing metadata.
func render(branch, commit, dirty, built string) string {
	var parts []string
	if branch != "" {
		parts = append(parts, "branch "+branch)
	}
	if commit != "" {
		treeState := ""
		switch dirty {
		case "true":
			treeState = " (dirty)"
		case "false":
			treeState = " (clean)"
		}
		parts = append(parts, "commit "+commit+treeState)
	}
	if built != "" {
		parts = append(parts, "built "+built)
	}
	if len(parts) == 0 {
		return "unknown (built outside a git repository)"
	}

	return strings.Join(parts, ", ")
}
