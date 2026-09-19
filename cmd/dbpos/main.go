// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// dbpos prints the character rows from the stack DB through the
// acceptance wire client: the live scenario watch helper. The name
// argument (default temp14) selects the character row.
package main

import (
    "fmt"
    "os"
    "strings"

    "github.com/melg8/swarm/internal/swarm/acceptance"
)

func main() {
    if err := run(); err != nil {
        fmt.Fprintln(os.Stderr, "dbpos:", err)
        os.Exit(1)
    }
}

// run prints the selected character row; the wire client closes
// inside the function so the connection never leaks.
func run() error {
    db, err := acceptance.ConnectDB(acceptance.DefaultDBConfig())
    if err != nil {
        return fmt.Errorf("connect: %w", err)
    }
    defer db.Close()

    name := "temp14"
    if len(os.Args) > 1 {
        name = os.Args[1]
    }
    // The name lands inside a quoted SQL literal of a local scratch
    // tool: only the plain character name alphabet passes, anything
    // else fails before the query (no quoting games).
    if !plainName(name) {
        return fmt.Errorf("bad character name %q", name)
    }
    rows, err := db.Query("SELECT char_name, level, x, y, z, online " +
        "FROM characters WHERE char_name = '" + name + "'")
    if err != nil {
        return fmt.Errorf("query: %w", err)
    }
    if len(rows) == 0 {
        fmt.Fprintln(os.Stdout, "no rows for "+name)

        return nil
    }
    for _, row := range rows {
        fmt.Fprintln(os.Stdout, strings.Join(row, " "))
    }

    return nil
}

// plainName reports whether the name is a plain game character name:
// the latin letters and digits only.
func plainName(name string) bool {
    if name == "" {
        return false
    }
    for _, r := range name {
        switch {
        case r >= 'a' && r <= 'z':
        case r >= 'A' && r <= 'Z':
        case r >= '0' && r <= '9':
        default:
            return false
        }
    }

    return true
}
