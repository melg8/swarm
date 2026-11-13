// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Package logfmt turns the logging conventions of AGENTS.md into a check: it
// parses the production Go files of a tree, finds the standard log helper call
// sites and asserts the format rules that survive a static look:
//
//   - period: no message ends with a trailing period (the newline of a
//     Printf multi-line call is stripped first);
//   - capital: a message starts with a capital letter, a format verb (a
//     "%v"-first printf) or a lowercase component tag ("login#3:").
package logfmt

import (
    "fmt"
    "go/ast"
    "go/parser"
    "go/token"
    "os"
    "path/filepath"
    "regexp"
    "strconv"
    "strings"
)

// Violation is one broken log message: the file, the line, the rule
// id and the message literal that broke it.
type Violation struct {
    File    string
    Line    int
    Rule    string
    Message string
}

// tagPattern matches the lowercase component tag the proxy, the hunt audit
// and the e2e layers mark their streams with ("login#%d:", "hunt audit:");
// a run that already carries a format verb is not a tag and gets flagged.
var tagPattern = regexp.MustCompile(
    `^[a-z][a-z0-9_-]*( [a-z0-9_-]+)*[#:]`)

// logHelpers are the log entry points the conventions cover; the receiver
// side matches loggerSelector below.
var logHelpers = map[string]bool{
    "Printf": true, "Println": true,
    "Fatalf": true, "Fatalln": true, "Fatal": true,
}

// ScanTree walks the production .go files under root (the test chatter of
// t.Log is not the bot log) and returns the violations of their log call
// sites; the generated marker files are skipped like the lint exclusions.
func ScanTree(root string) ([]Violation, error) {
    violations := []Violation{}
    walkErr := filepath.WalkDir(root, func(
        path string, entry os.DirEntry, err error,
    ) error {
        if err != nil {
            return err
        }
        if entry.IsDir() {
            name := entry.Name()
            if name == ".git" || name == "testdata" || name == "vendor" {
                return filepath.SkipDir
            }

            return nil
        }
        if !strings.HasSuffix(path, ".go") ||
            strings.HasSuffix(path, "_test.go") {
            return nil
        }
        found, fileErr := scanFile(path)
        if fileErr != nil {
            return fileErr
        }
        violations = append(violations, found...)

        return nil
    })
    if walkErr != nil {
        return nil, fmt.Errorf("walk %s: %w", root, walkErr)
    }

    return violations, nil
}

// scanFile parses one file and returns its violations; a file with
// the generated marker parses to an empty list.
func scanFile(path string) ([]Violation, error) {
    content, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("read %s: %w", path, err)
    }
    if isGenerated(content) {
        return nil, nil
    }
    fileSet := token.NewFileSet()
    parsed, err := parser.ParseFile(fileSet, path, content, 0)
    if err != nil {
        return nil, fmt.Errorf("parse %s: %w", path, err)
    }

    violations := []Violation{}
    ast.Inspect(parsed, func(node ast.Node) bool {
        call, ok := node.(*ast.CallExpr)
        if !ok || !isLogCall(call) || len(call.Args) == 0 {
            return true
        }
        literals := stringLiterals(call.Args[0])
        if len(literals) == 0 {
            return true
        }
        line := fileSet.Position(call.Pos()).Line
        for _, rule := range checkMessage(literals) {
            violations = append(violations, Violation{
                File:    path,
                Line:    line,
                Rule:    rule,
                Message: firstMeaningful(literals),
            })
        }

        return true
    })

    return violations, nil
}

// isGenerated reports whether the file carries the canonical
// "Code generated ... DO NOT EDIT." marker in its header.
func isGenerated(content []byte) bool {
    head := content
    if len(head) > 4096 {
        head = head[:4096]
    }
    for _, line := range strings.Split(string(head), "\n") {
        line = strings.TrimSpace(line)
        if strings.HasPrefix(line, "package ") {
            return false
        }
        if strings.HasPrefix(line, "// Code generated") &&
            strings.Contains(line, "DO NOT EDIT.") {
            return true
        }
    }

    return false
}

// isLogCall matches log.Printf(...) and the logger.Printf(...) form of the
// package level *log.Logger or the field named "logger".
func isLogCall(call *ast.CallExpr) bool {
    selector, ok := call.Fun.(*ast.SelectorExpr)
    if !ok || !logHelpers[selector.Sel.Name] {
        return false
    }
    switch receiver := selector.X.(type) {
    case *ast.Ident:
        return receiver.Name == "log" || receiver.Name == "logger"
    case *ast.SelectorExpr:
        return receiver.Sel.Name == "logger" || receiver.Sel.Name == "log"
    }

    return false
}

// stringLiterals collects the literals of the message in source order (a
// plain literal or the parts of a concatenation); a dynamic message returns
// nothing and cannot be checked.
func stringLiterals(expr ast.Expr) []string {
    switch part := expr.(type) {
    case *ast.BasicLit:
        if part.Kind == token.STRING {
            value, err := strconv.Unquote(part.Value)
            if err != nil {
                return nil
            }

            return []string{value}
        }
    case *ast.BinaryExpr:
        if part.Op != token.ADD {
            return nil
        }
        left := stringLiterals(part.X)
        right := stringLiterals(part.Y)

        return append(left, right...)
    }

    return nil
}

// checkMessage applies both rules to the literal chain (the capital rule
// reads the first part, the period rule the last) and returns the broken
// rule ids, both for a doubly broken message.
func checkMessage(literals []string) []string {
    broken := []string{}
    first := strings.TrimLeft(literals[0], " \t")
    if first != "" {
        head := first[:1]
        upper := strings.ToUpper(head) == head
        tagged := tagPattern.MatchString(first)
        if !upper && !strings.HasPrefix(first, "%") && !tagged {
            broken = append(broken, "capital")
        }
    }
    last := literals[len(literals)-1]
    last = strings.TrimRight(last, "\n")
    last = strings.TrimRight(last, " \t")
    if strings.HasSuffix(last, ".") {
        broken = append(broken, "period")
    }

    return broken
}

// firstMeaningful renders the violation message for the report: the
// first non-empty literal, truncated to a readable width.
func firstMeaningful(literals []string) string {
    for _, literal := range literals {
        if strings.TrimSpace(literal) != "" {
            if len(literal) > 60 {
                literal = literal[:57] + "..."
            }

            return literal
        }
    }

    return ""
}
