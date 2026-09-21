// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "os"
    "path/filepath"
    "strings"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// dirtySource is a gofmt-clean source with indentation tabs: the
// input every CLI test starts from.
const dirtySource = "package main\n\nfunc f() {\n\treturn\n}\n"

// cleanSource is what FormatSource turns dirtySource into.
const cleanSource = "package main\n\nfunc f() {\n    return\n}\n"

// writeTree lays out one go file at rel inside a fresh temp dir and
// returns the absolute path.
func writeTree(t *testing.T, rel, content string) string {
    t.Helper()
    path := filepath.Join(t.TempDir(), rel)
    require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
    require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

    return path
}

func TestRunListPrintsOnlyUnformattedFiles(t *testing.T) {
    dirty := writeTree(t, "dirty.go", dirtySource)
    clean := writeTree(t, "clean.go", cleanSource)

    var out, errOut strings.Builder
    require.NoError(t, run([]string{"-l", dirty, clean}, &out, &errOut))

    require.Equal(t, dirty+"\n", out.String())
    require.Empty(t, errOut.String())
}

func TestRunWriteRewritesTheFilesInPlace(t *testing.T) {
    path := writeTree(t, "dirty.go", dirtySource)

    var out, errOut strings.Builder
    require.NoError(t, run([]string{"-w", path}, &out, &errOut))

    rewritten, err := os.ReadFile(path)
    require.NoError(t, err)
    require.Equal(t, cleanSource, string(rewritten))
    require.Empty(t, out.String())
}

func TestRunWithoutFlagsPrintsFormattedSource(t *testing.T) {
    path := writeTree(t, "dirty.go", dirtySource)

    var out, errOut strings.Builder
    require.NoError(t, run([]string{path}, &out, &errOut))

    // The stdout mode emits the formatted source of every file and
    // leaves the file itself untouched.
    require.Equal(t, cleanSource, out.String())
    onDisk, err := os.ReadFile(path)
    require.NoError(t, err)
    require.Equal(t, dirtySource, string(onDisk))
}

func TestCollectGoFilesSkipsDotAndUnderscoreDirs(t *testing.T) {
    root := t.TempDir()
    require.NoError(t, os.MkdirAll(filepath.Join(root, "sub"), 0o755))
    require.NoError(t, os.MkdirAll(filepath.Join(root, ".hidden"), 0o755))
    require.NoError(t, os.MkdirAll(filepath.Join(root, "_skip"), 0o755))
    files := map[string]string{
        filepath.Join(root, "a.go"):            dirtySource,
        filepath.Join(root, "sub", "b.go"):     dirtySource,
        filepath.Join(root, "notes.txt"):       "not go",
        filepath.Join(root, ".hidden", "c.go"): dirtySource,
        filepath.Join(root, "_skip", "d.go"):   dirtySource,
    }
    for rel, content := range files {
        require.NoError(t, os.WriteFile(rel, []byte(content), 0o600))
    }

    collected, err := collectGoFiles([]string{root})
    require.NoError(t, err)
    names := map[string]bool{}
    for _, path := range collected {
        names[filepath.Base(path)] = true
    }
    assert.True(t, names["a.go"])
    assert.True(t, names["b.go"])
    assert.False(t, names["c.go"], "dot dirs are skipped")
    assert.False(t, names["d.go"], "underscore dirs are skipped")
}

func TestCollectGoFilesEntersAnExplicitDotDir(t *testing.T) {
    // The dot convention is a skip rule for nested walks only: the
    // explicitly named root is always entered, so .agents/skills
    // works as an argument.
    hidden := filepath.Join(t.TempDir(), ".agents")
    require.NoError(t, os.MkdirAll(hidden, 0o755))
    require.NoError(t, os.WriteFile(
        filepath.Join(hidden, "e.go"), []byte(dirtySource), 0o600))

    collected, err := collectGoFiles([]string{hidden})
    require.NoError(t, err)
    require.Len(t, collected, 1)
}

func TestCollectGoFilesAcceptsASingleFileArgument(t *testing.T) {
    single := writeTree(t, "single.go", dirtySource)
    collected, err := collectGoFiles([]string{single})
    require.NoError(t, err)
    require.Equal(t, []string{single}, collected)
}

func TestCollectGoFilesSkipsNonGoFiles(t *testing.T) {
    txt := writeTree(t, "notes.txt", "not go")
    collected, err := collectGoFiles([]string{txt})
    require.NoError(t, err)
    require.Empty(t, collected)
}

func TestRunMissingPathFails(t *testing.T) {
    missing := filepath.Join(t.TempDir(), "absent.go")

    var out, errOut strings.Builder
    err := run([]string{missing}, &out, &errOut)

    require.Error(t, err)
    assert.Contains(t, err.Error(), "collecting go files")
}

func TestRunReportsBrokenSourceAndFails(t *testing.T) {
    broken := writeTree(t, "broken.go", "package main\n\nfunc {\n")

    var out, errOut strings.Builder
    err := run([]string{broken}, &out, &errOut)

    require.Error(t, err)
    assert.Contains(t, errOut.String(), "Error formatting")
    // The clean file in the same run still formats: the failure does
    // not stop the others.
    clean := writeTree(t, "clean.go", cleanSource)
    out.Reset()
    err = run([]string{clean, broken}, &out, &errOut)
    require.Error(t, err)
    require.Contains(t, out.String(), cleanSource)
}

func TestRunDefaultsToTheWorkingDirectory(t *testing.T) {
    // No path arguments: the walk falls back to ".". A working
    // directory without go files keeps the list empty - the flag
    // parsing and the walk complete without an explicit path.
    t.Chdir(t.TempDir())

    var out, errOut strings.Builder
    require.NoError(t, run([]string{"-l"}, &out, &errOut))
    require.Empty(t, out.String())
    require.Empty(t, errOut.String())
}
