// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "strings"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// formatted renders src through FormatSource and fails the test on
// any formatting error.
func formatted(t *testing.T, src string) string {
    t.Helper()

    out, err := FormatSource([]byte(src))
    require.NoError(t, err)

    return string(out)
}

// realTab is a true tab byte, built without an escape so the source
// of the test itself stays unambiguous.
func realTab() string {
    return string(rune(9))
}

func TestFormatSourceReplacesIndentTabs(t *testing.T) {
    src := "package main\n\nfunc f() {\n\tif x {\n\t\treturn\n\t}\n}\n"
    want := "package main\n\nfunc f() {\n    if x {\n        return\n    }\n}\n"

    assert.Equal(t, want, formatted(t, src))
}

func TestFormatSourceKeepsLiteralTabs(t *testing.T) {
    tab := realTab()
    src := "package main\n\n" +
        "const raw = `a" + tab + "b`\n" +
        "const esc = \"a\\tb\"\n" +
        "const ch = '" + tab + "'\n"

    out := formatted(t, src)

    // The raw string and the char literal carry a real tab byte; the
    // interpreted string carries the two character escape. None of
    // them may be widened to spaces.
    assert.Contains(t, out, "`a"+tab+"b`")
    assert.Contains(t, out, `"a\tb"`)
    assert.Contains(t, out, "'"+tab+"'")
    assert.NotContains(t, out, "a    b")
}

func TestFormatSourceAlignsStructFields(t *testing.T) {
    src := "package main\n\ntype t struct {\n" +
        "\tA   int\n\tBB  int\n\tCCC int\n}\n"

    out := formatted(t, src)
    lines := strings.Split(out, "\n")

    // package, blank, struct, three fields, closing, trailing empty.
    require.Len(t, lines, 8)

    typeColumn := -1
    for _, line := range lines[3:6] {
        at := strings.Index(line, "int")
        require.NotEqual(t, -1, at, "no int in %q", line)

        if typeColumn == -1 {
            typeColumn = at

            continue
        }
        assert.Equal(t, typeColumn, at, "unaligned field %q", line)
    }
    assert.NotContains(t, out, "\t")
}

func TestFormatSourceIsIdempotent(t *testing.T) {
    src := "package main\n\n" +
        "type t struct {\n\tA   int\n\tBB  int\n}\n\n" +
        "func f(a int) int {\n\tif a > 0 {\n\t\treturn a\n\t}\n\treturn -a\n}\n"

    once := formatted(t, src)
    twice := formatted(t, once)

    assert.Equal(t, once, twice)
    assert.NotContains(t, once, "\t")
}

func TestFormatSourceNormalizesWideIndents(t *testing.T) {
    src := "package main\n\nfunc f() {\n" +
        "        if x {\n                return\n        }\n}\n"

    out := formatted(t, src)

    assert.Equal(
        t,
        "package main\n\nfunc f() {\n    if x {\n        return\n    }\n}\n",
        out,
    )
}

func TestFormatSourceExpandsCommentTabs(t *testing.T) {
    src := "package main\n\n// Note:\tsecond column.\nfunc f() {}\n"

    out := formatted(t, src)

    // A tab directly after the slashes becomes the gofmt single
    // space; a tab inside the comment text widens to four spaces.
    assert.Contains(t, out, "// Note:    second column.")
    assert.NotContains(t, out, "\t")
}

func TestFormatSourceReportsBrokenSource(t *testing.T) {
    _, err := FormatSource([]byte("package main\n\nfunc {\n"))

    require.Error(t, err)
}
