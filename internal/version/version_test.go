// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package version

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// The full hash fixture of the tests - the identity line carries the
// long form, searchable against any git interface as is.
const fullHash = "675d2e545262df3b7215c198310765c4577e09fc"

// TestRenderIdentityLine pins the layout of the identity line: the
// fields render comma separated, the commit carries the tree state,
// unknown fields drop out.
func TestRenderIdentityLine(t *testing.T) {
	tests := []struct {
		name   string
		branch string
		commit string
		dirty  string
		built  string
		want   string
	}{
		{
			name:   "the full clean build",
			branch: "feature/proxy-server",
			commit: fullHash,
			dirty:  "false",
			built:  "2026-09-09T21:24:29Z",
			want: "branch feature/proxy-server, " +
				"commit " + fullHash + " (clean), " +
				"built 2026-09-09T21:24:29Z",
		},
		{
			name:   "the dirty tree",
			branch: "main",
			commit: fullHash,
			dirty:  "true",
			built:  "",
			want:   "branch main, commit " + fullHash + " (dirty)",
		},
		{
			name:   "the tree state unknown",
			branch: "",
			commit: fullHash,
			dirty:  "",
			built:  "",
			want:   "commit " + fullHash,
		},
		{
			name:   "the branch only",
			branch: "main",
			commit: "",
			dirty:  "",
			built:  "",
			want:   "branch main",
		},
		{
			name: "nothing known",
			want: "unknown (built outside a git repository)",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want,
				render(test.branch, test.commit, test.dirty, test.built))
		})
	}
}

// TestReadWorktree pins the .git resolution - the last resort of a
// binary with no VCS stamp (a file path go run): the branch and the
// commit come out of HEAD and the ref it points at, the loose ref
// file and packed-refs both resolve, a linked worktree is followed
// through its gitdir and commondir pointers, a detached HEAD reports
// the hash only.
func TestReadWorktree(t *testing.T) {
	t.Run("ref head with a loose ref", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".git", "HEAD"),
			"ref: refs/heads/feature/proxy-server\n")
		writeFile(t,
			filepath.Join(dir, ".git", "refs", "heads",
				"feature", "proxy-server"),
			fullHash+"\n")
		tree := readWorktree(dir)
		require.Equal(t, "feature/proxy-server", tree.branch)
		require.Equal(t, fullHash, tree.commit)
	})

	t.Run("ref head with packed refs", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".git", "HEAD"),
			"ref: refs/heads/main\n")
		writeFile(t, filepath.Join(dir, ".git", "packed-refs"),
			"# pack-refs with: peeled fully-peeled sorted \n"+
				fullHash+" refs/heads/main\n"+
				"1111111111111111111111111111111111111111 "+
				"refs/heads/other\n")
		tree := readWorktree(dir)
		require.Equal(t, "main", tree.branch)
		require.Equal(t, fullHash, tree.commit)
	})

	t.Run("detached head", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".git", "HEAD"),
			fullHash+"\n")
		tree := readWorktree(dir)
		require.Empty(t, tree.branch)
		require.Equal(t, fullHash, tree.commit)
	})

	t.Run("gitdir pointer with commondir", func(t *testing.T) {
		// A linked worktree: .git points at the worktree git dir,
		// the shared refs live in the common dir behind commondir.
		dir := t.TempDir()
		common := t.TempDir()
		gitdir := t.TempDir()
		writeFile(t, filepath.Join(gitdir, "HEAD"),
			"ref: refs/heads/main\n")
		writeFile(t, filepath.Join(gitdir, "commondir"), common+"\n")
		writeFile(t,
			filepath.Join(common, "refs", "heads", "main"),
			fullHash+"\n")
		writeFile(t, filepath.Join(dir, ".git"),
			"gitdir: "+gitdir+"\n")
		tree := readWorktree(dir)
		require.Equal(t, "main", tree.branch)
		require.Equal(t, fullHash, tree.commit)
	})

	t.Run("gitdir pointer with a local ref", func(t *testing.T) {
		dir := t.TempDir()
		gitdir := t.TempDir()
		writeFile(t, filepath.Join(gitdir, "HEAD"),
			"ref: refs/heads/main\n")
		writeFile(t,
			filepath.Join(gitdir, "refs", "heads", "main"),
			fullHash+"\n")
		writeFile(t, filepath.Join(dir, ".git"),
			"gitdir: "+gitdir+"\n")
		tree := readWorktree(dir)
		require.Equal(t, "main", tree.branch)
		require.Equal(t, fullHash, tree.commit)
	})

	t.Run("ref head with a missing ref", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, ".git", "HEAD"),
			"ref: refs/heads/gone\n")
		tree := readWorktree(dir)
		require.Equal(t, "gone", tree.branch)
		require.Empty(t, tree.commit)
	})

	t.Run("no git", func(t *testing.T) {
		tree := readWorktree(t.TempDir())
		require.Empty(t, tree.branch)
		require.Empty(t, tree.commit)
	})
}

// TestIdentityPrefersLinkTimeFields pins the resolution order: the
// link-time identity fields win over the embedded VCS stamp of the
// test binary.
func TestIdentityPrefersLinkTimeFields(t *testing.T) {
	savedBranch, savedCommit := Branch, Commit
	savedDirty, savedBuilt := Dirty, BuildTime
	t.Cleanup(func() {
		Branch, Commit = savedBranch, savedCommit
		Dirty, BuildTime = savedDirty, savedBuilt
	})

	Branch, Commit, Dirty, BuildTime =
		"feature/x", fullHash, "false", "T"
	require.Equal(t,
		"branch feature/x, commit "+fullHash+" (clean), built T",
		Identity())
}

// writeFile creates a file together with its parent directories.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}
