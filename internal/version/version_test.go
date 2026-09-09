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
			commit: "6a0c183",
			dirty:  "false",
			built:  "2026-09-10T18:00:00Z",
			want: "branch feature/proxy-server, " +
				"commit 6a0c183 (clean), " +
				"built 2026-09-10T18:00:00Z",
		},
		{
			name:   "the dirty tree",
			branch: "main",
			commit: "abcdef1",
			dirty:  "true",
			built:  "",
			want:   "branch main, commit abcdef1 (dirty)",
		},
		{
			name:   "the tree state unknown",
			branch: "",
			commit: "abcdef1",
			dirty:  "",
			built:  "",
			want:   "commit abcdef1",
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

// TestShortHash pins the hash trim: a full git hash shrinks to the 7
// character prefix, a short one passes through untouched.
func TestShortHash(t *testing.T) {
	require.Equal(t, "6a0c183", shortHash(
		"6a0c1839e9c4a610c66d3808dcd3b186ae5897f4"))
	require.Equal(t, "abc", shortHash("abc"))
}

// TestWorktreeBranch pins the .git/HEAD resolution: the ref form
// carries the branch (slashes included), the gitdir pointer of a
// linked worktree leads to its HEAD, a detached HEAD and a missing
// .git report no branch at all.
func TestWorktreeBranch(t *testing.T) {
	t.Run("ref head", func(t *testing.T) {
		dir := t.TempDir()
		writeGitHead(t, filepath.Join(dir, ".git", "HEAD"),
			"ref: refs/heads/feature/proxy-server\n")
		require.Equal(t, "feature/proxy-server", worktreeBranch(dir))
	})

	t.Run("detached head", func(t *testing.T) {
		dir := t.TempDir()
		writeGitHead(t, filepath.Join(dir, ".git", "HEAD"),
			"6a0c1839e9c4a610c66d3808dcd3b186ae5897f4\n")
		require.Empty(t, worktreeBranch(dir))
	})

	t.Run("gitdir pointer", func(t *testing.T) {
		dir := t.TempDir()
		gitdir := t.TempDir()
		writeGitHead(t, filepath.Join(gitdir, "HEAD"),
			"ref: refs/heads/main\n")
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".git"),
			[]byte("gitdir: "+gitdir+"\n"), 0o600))
		require.Equal(t, "main", worktreeBranch(dir))
	})

	t.Run("no git", func(t *testing.T) {
		require.Empty(t, worktreeBranch(t.TempDir()))
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
		"feature/x", "6a0c183", "false", "T"
	require.Equal(t, "branch feature/x, commit 6a0c183 (clean), built T",
		Identity())
}

// writeGitHead creates a HEAD file with the content of a git dir.
func writeGitHead(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}
