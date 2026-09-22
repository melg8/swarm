// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package version

import (
    "path/filepath"
    "testing"

    "github.com/stretchr/testify/require"
)

// TestIdentityFallsBackToTheWorktree pins the resolution order of the
// identity line: a binary built without the -ldflags -X identity fills
// the missing branch out of the .git directory of the working
// directory, the link time fields keep their precedence and a tree
// with no identity at all degrades to the unknown line.
func TestIdentityFallsBackToTheWorktree(t *testing.T) {
    savedBranch, savedCommit := Branch, Commit
    savedDirty, savedBuilt := Dirty, BuildTime
    t.Cleanup(func() {
        Branch, Commit = savedBranch, savedCommit
        Dirty, BuildTime = savedDirty, savedBuilt
    })

    writeWorktree := func(t *testing.T) {
        t.Helper()
        writeFile(t, filepath.Join(".git", "HEAD"),
            "ref: refs/heads/main\n")
        writeFile(t,
            filepath.Join(".git", "refs", "heads", "main"),
            fullHash+"\n")
    }

    t.Run("the branch comes from the worktree", func(t *testing.T) {
        t.Chdir(t.TempDir())
        writeWorktree(t)
        Branch, Commit, Dirty, BuildTime = "", "abc123", "", ""
        line := Identity()
        require.Contains(t, line, "branch main")
        require.Contains(t, line, "commit abc123",
            "the link time commit wins over the worktree")
    })

    t.Run("the worktree answers for the whole line", func(t *testing.T) {
        t.Chdir(t.TempDir())
        writeWorktree(t)
        Branch, Commit, Dirty, BuildTime = "", "", "", ""
        line := Identity()
        require.Contains(t, line, "branch main")
        require.Contains(t, line, "commit "+fullHash)
    })

    t.Run("the gitless tree degrades", func(t *testing.T) {
        t.Chdir(t.TempDir())
        Branch, Commit, Dirty, BuildTime = "", "", "", ""
        line := Identity()
        // The test binary itself may carry a VCS stamp (go test
        // builds from the repository embed it): with one present the
        // commit fills from the stamp, without one the unknown line
        // remains - both behaviors honor the resolution order.
        if _, ok := readVCSStamp(); !ok {
            require.Equal(t,
                "unknown (built outside a git repository)", line)
        } else {
            require.Contains(t, line, "commit ")
        }
    })
}

// TestReadWorktreeFollowsTheRelativeGitdir pins the linked worktree
// form of a submodule checkout: the .git pointer file carries a
// relative gitdir path and the commondir pointer resolves the shared
// refs against it.
func TestReadWorktreeFollowsTheRelativeGitdir(t *testing.T) {
    dir := t.TempDir()
    common := filepath.Join(dir, "common")
    gitdir := filepath.Join(dir, "common", "modules", "sub")
    writeFile(t, filepath.Join(gitdir, "HEAD"),
        "ref: refs/heads/main\n")
    writeFile(t, filepath.Join(gitdir, "commondir"), "../..\n")
    writeFile(t,
        filepath.Join(common, "refs", "heads", "main"),
        fullHash+"\n")
    writeFile(t, filepath.Join(dir, ".git"),
        "gitdir: "+filepath.Join("common", "modules", "sub")+"\n")

    tree := readWorktree(dir)
    require.Equal(t, "main", tree.branch)
    require.Equal(t, fullHash, tree.commit)
}

// TestReadWorktreeIgnoresTheForeignPointer pins the pointer guard: a
// .git file whose gitdir carries no HEAD (a broken pointer) answers
// the empty worktree instead of an error.
func TestReadWorktreeIgnoresTheForeignPointer(t *testing.T) {
    dir := t.TempDir()
    gitdir := t.TempDir()
    writeFile(t, filepath.Join(dir, ".git"), "gitdir: "+gitdir+"\n")

    tree := readWorktree(dir)
    require.Empty(t, tree.branch)
    require.Empty(t, tree.commit)
}
