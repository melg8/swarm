// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Command gofmt-spaces formats Go source files exactly like gofmt
// except that it uses spaces instead of tabs: every tab of the
// canonical gofmt output is replaced by four spaces outside string
// and character literals. The repository-wide whitespace policy is
// "spaces only" (see AGENTS.md), so this tool - not the stock
// gofmt/gofumpt, both of which re-tab the tree - is the formatter of
// record. Run it through `task fmt` or directly:
//
//    gofmt-spaces -w . .agents/skills
//    gofmt-spaces -l cmd internal
package main

import (
    "bytes"
    "errors"
    "flag"
    "fmt"
    "go/format"
    "go/scanner"
    "go/token"
    "io"
    "io/fs"
    "os"
    "path/filepath"
    "strings"
)

// spaceWidth is the number of spaces one indentation tab becomes.
const spaceWidth = 4

// options carries the command line switches of the run.
type options struct {
    list  bool
    write bool
}

func main() {
    if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
        fmt.Fprintf(os.Stderr, "Error %v\n", err)

        os.Exit(1)
    }
}

// run parses the flags, collects the go files of every path argument
// and processes them according to the switches.
func run(args []string, out, errOut io.Writer) error {
    var opts options

    flags := flag.NewFlagSet("gofmt-spaces", flag.ContinueOnError)
    flags.BoolVar(&opts.list, "l", false, "list files whose formatting differs")
    flags.BoolVar(&opts.write, "w", false, "write result back to the file")
    if err := flags.Parse(args); err != nil {
        return fmt.Errorf("parsing flags: %w", err)
    }

    paths := flags.Args()
    if len(paths) == 0 {
        paths = []string{"."}
    }
    files, err := collectGoFiles(paths)
    if err != nil {
        return fmt.Errorf("collecting go files: %w", err)
    }

    return processFiles(files, opts, out, errOut)
}

// collectGoFiles expands the path arguments into an explicit file
// list. Directory arguments are walked recursively; nested
// directories that start with a dot or an underscore are skipped
// (the go tooling convention), while an explicitly named root is
// always entered, so `.agents/skills` works as an argument.
func collectGoFiles(paths []string) ([]string, error) {
    files := make([]string, 0, len(paths))

    for _, root := range paths {
        info, err := os.Stat(root)
        if err != nil {
            return nil, fmt.Errorf("stat %s: %w", root, err)
        }
        if !info.IsDir() {
            if strings.HasSuffix(root, ".go") {
                files = append(files, root)
            }

            continue
        }
        walk := func(path string, entry fs.DirEntry, err error) error {
            if err != nil {
                return fmt.Errorf("walking %s: %w", path, err)
            }
            if entry.IsDir() {
                name := entry.Name()
                if path != root && (strings.HasPrefix(name, ".") ||
                    strings.HasPrefix(name, "_")) {
                    return fs.SkipDir
                }

                return nil
            }
            if strings.HasSuffix(path, ".go") {
                files = append(files, path)
            }

            return nil
        }
        if err := filepath.WalkDir(root, walk); err != nil {
            return nil, fmt.Errorf("walking %s: %w", root, err)
        }
    }

    return files, nil
}

// processFiles formats every file and reports the result the way the
// switches ask: -l prints the paths that differ, -w rewrites the
// files that differ, otherwise the formatted source of every file
// goes to out. Files that fail to parse are reported to errOut and
// fail the run without stopping the others.
func processFiles(
    files []string,
    opts options,
    out, errOut io.Writer,
) error {
    failed := false

    for _, path := range files {
        src, err := os.ReadFile(path)
        if err != nil {
            fmt.Fprintf(errOut, "Error reading %s: %v\n", path, err)
            failed = true

            continue
        }
        formatted, err := FormatSource(src)
        if err != nil {
            fmt.Fprintf(errOut, "Error formatting %s: %v\n", path, err)
            failed = true

            continue
        }
        if !bytes.Equal(src, formatted) || !opts.list && !opts.write {
            if err := reportFile(path, formatted, opts, out); err != nil {
                fmt.Fprintf(errOut, "Error writing %s: %v\n", path, err)
                failed = true

                continue
            }
        }
    }
    if failed {
        return errors.New("some files could not be formatted")
    }

    return nil
}

// reportFile emits one formatting change: the path with -l, the
// rewritten file with -w, the formatted source on out otherwise.
func reportFile(
    path string,
    formatted []byte,
    opts options,
    out io.Writer,
) error {
    switch {
    case opts.list:
        _, err := fmt.Fprintln(out, path)

        return err
    case opts.write:
        info, err := os.Stat(path)
        if err != nil {
            return fmt.Errorf("stat %s: %w", path, err)
        }

        // A formatter writes the files the caller pointed it at, the
        // same trust model as gofmt -w; the taint is the feature.
        return os.WriteFile(path, formatted, info.Mode().Perm()) //nolint:gosec
    default:
        _, err := out.Write(formatted)

        return err
    }
}

// FormatSource formats src the way gofmt does and then replaces every
// tab of the canonical output with spaces outside string and character
// literals - the tab of a literal is data, never indentation.
func FormatSource(src []byte) ([]byte, error) {
    canonical, err := format.Source(src)
    if err != nil {
        return nil, fmt.Errorf("gofmt: %w", err)
    }
    spans, err := literalSpans(canonical)
    if err != nil {
        return nil, fmt.Errorf("scanning literals: %w", err)
    }

    return expandTabs(canonical, spans), nil
}

// literalSpans returns the sorted [start, end) byte offsets of every
// string and character literal of src.
func literalSpans(src []byte) ([][2]int, error) {
    fset := token.NewFileSet()
    file := fset.AddFile("", fset.Base(), len(src))

    var scanErr error
    onError := func(pos token.Position, msg string) {
        if scanErr == nil {
            scanErr = fmt.Errorf("%s: %s", pos, msg)
        }
    }

    var scan scanner.Scanner
    scan.Init(file, src, onError, scanner.ScanComments)

    spans := make([][2]int, 0, 16)
    for {
        pos, tok, lit := scan.Scan()
        if tok == token.EOF {
            break
        }
        if tok != token.STRING && tok != token.CHAR {
            continue
        }
        start := file.Offset(pos)
        spans = append(spans, [2]int{start, start + len(lit)})
    }
    if scanErr != nil {
        return nil, fmt.Errorf("scanning: %w", scanErr)
    }

    return spans, nil
}

// expandTabs replaces every tab outside the given literal spans with
// spaces. The spans must be sorted and non-overlapping, which the
// scanner guarantees.
func expandTabs(src []byte, spans [][2]int) []byte {
    tabCount := bytes.Count(src, []byte{'\t'})
    out := make([]byte, 0, len(src)+(spaceWidth-1)*tabCount)

    cursor := 0
    for _, span := range spans {
        out = appendExpanded(out, src[cursor:span[0]])
        out = append(out, src[span[0]:span[1]]...)
        cursor = span[1]
    }
    out = appendExpanded(out, src[cursor:])

    return out
}

// appendExpanded appends segment to dst with every tab widened to
// spaceWidth spaces.
func appendExpanded(dst, segment []byte) []byte {
    for _, b := range segment {
        if b == '\t' {
            dst = append(dst, ' ', ' ', ' ', ' ')

            continue
        }
        dst = append(dst, b)
    }

    return dst
}
