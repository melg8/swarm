// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Package version pins the build identity of the running binary: the
// git branch, the commit, the tree state and the build time. The
// build scripts bake the values in at link time with -ldflags -X; a
// plain go build or go run falls back to the VCS stamp Go embeds into
// binaries built from a git repository (the package path form - `go
// run ./cmd/swarm`; a file path build like `go run ./cmd/swarm/main.go`
// compiles the command-line-arguments package and gets NO stamp) and
// to the .git directory of the working directory, which carries both
// the branch and the commit. Every artifact that must match a code
// state - the state dump of the web UI, the startup log line of the
// bot - renders the identity through Identity, so a live problem
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

	// Commit is the full git hash of the build - the long form, so
	// a report line is searchable against any git interface as is.
	Commit string

	// Dirty is "true" when the working tree had uncommitted changes
	// at build time, "false" otherwise.
	Dirty string

	// BuildTime is the RFC3339 UTC timestamp of the build.
	BuildTime string
)

// Identity renders the build identity line of the running binary:
// the branch, the commit with the tree state and the build time. The
// link-time fields win, the VCS stamp of the binary and the .git
// directory of the working directory fill the gaps, unknown fields
// drop out.
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
	if branch == "" || commit == "" {
		tree := readWorktree(".")
		if branch == "" {
			branch = tree.branch
		}
		if commit == "" {
			commit = tree.commit
		}
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
// -buildvcs=false was passed or the build has no module package.
func readVCSStamp() (vcsStamp, bool) {
	var stamp vcsStamp
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return stamp, false
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			stamp.revision = setting.Value
		case "vcs.modified":
			stamp.modified = setting.Value
		case "vcs.time":
			stamp.time = setting.Value
		}
	}

	return stamp, stamp.revision != ""
}

// worktree carries the branch and the commit resolved straight out
// of the .git directory of a worktree - the last resort of a binary
// with no VCS stamp.
type worktree struct {
	branch string
	commit string
}

// readWorktree resolves the branch and the commit of the worktree
// rooted at dir out of .git: HEAD points either at a ref (resolved
// through the loose ref file or packed-refs) or carries the hash of
// a detached HEAD.
func readWorktree(dir string) worktree {
	var tree worktree
	gitdir, head, ok := readHead(dir)
	if !ok {
		return tree
	}
	head = strings.TrimSpace(head)
	const refPrefix = "ref: "
	if !strings.HasPrefix(head, refPrefix) {
		// A detached HEAD: the hash itself, no branch to report.
		tree.commit = head

		return tree
	}
	ref := strings.TrimPrefix(head, refPrefix)
	if !strings.HasPrefix(ref, "refs/heads/") {
		return tree
	}
	tree.branch = strings.TrimPrefix(ref, "refs/heads/")
	tree.commit = resolveRef(gitdir, ref)

	return tree
}

// readHead reads the HEAD file of the worktree rooted at dir and
// returns the git directory it lives in: .git is a directory in a
// plain checkout and a "gitdir: <path>" pointer file in a linked
// worktree or a submodule.
func readHead(dir string) (string, string, bool) {
	dotGit := filepath.Join(dir, ".git")
	head, err := readGitEntry(filepath.Join(dotGit, "HEAD"))
	if err == nil {
		return dotGit, string(head), true
	}
	pointer, err := readGitEntry(dotGit)
	if err != nil {
		return "", "", false
	}
	gitdir := strings.TrimSpace(
		strings.TrimPrefix(string(pointer), "gitdir:"))
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(dir, gitdir)
	}
	head, err = readGitEntry(filepath.Join(gitdir, "HEAD"))
	if err != nil {
		return "", "", false
	}

	return gitdir, string(head), true
}

// resolveRef looks a ref up inside a git directory: the loose ref
// file first, then packed-refs. A linked worktree keeps the shared
// refs in the common dir - the commondir file points there.
func resolveRef(gitdir, ref string) string {
	for _, dir := range refSearchDirs(gitdir) {
		if hash, err := readGitEntry(filepath.Join(dir, ref)); err == nil {
			return strings.TrimSpace(string(hash))
		}
		if hash := packedRef(dir, ref); hash != "" {
			return hash
		}
	}

	return ""
}

// refSearchDirs lists the directories a shared ref can live in: the
// git dir itself and, behind the commondir pointer, the common dir
// of a linked worktree.
func refSearchDirs(gitdir string) []string {
	dirs := make([]string, 0, 2)
	dirs = append(dirs, gitdir)
	pointer, err := readGitEntry(filepath.Join(gitdir, "commondir"))
	if err != nil {
		return dirs
	}
	common := strings.TrimSpace(string(pointer))
	if !filepath.IsAbs(common) {
		common = filepath.Join(gitdir, common)
	}

	return append(dirs, common)
}

// packedRef scans the packed-refs file of a git directory for a ref.
func packedRef(gitdir, ref string) string {
	packed, err := readGitEntry(filepath.Join(gitdir, "packed-refs"))
	if err != nil {
		return ""
	}
	want := " " + ref
	for _, line := range strings.Split(string(packed), "\n") {
		if hash, found := strings.CutSuffix(
			strings.TrimSpace(line), want); found {
			return hash
		}
	}

	return ""
}

// readGitEntry reads a file of the local git layout - HEAD, the
// gitdir/commondir pointers, refs. Every path here resolves inside
// the .git directory of the worktree being inspected, never through
// user input.
func readGitEntry(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// render assembles the identity line out of the resolved fields:
// empty fields drop out, an empty input reports the missing metadata.
func render(branch, commit, dirty, built string) string {
	parts := make([]string, 0, 4)
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
