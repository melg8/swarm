// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "testing"

    "github.com/stretchr/testify/require"
)

// The smoke pass of issue #9: the character name filter of the DB
// position helper (the plain ASCII names pass, anything a shell or
// a log injection could hide behind stays out).

func TestPlainName(t *testing.T) {
    t.Run("plain alphanumeric names pass", func(t *testing.T) {
        require.True(t, plainName("temp14"))
        require.True(t, plainName("A"))
        require.True(t, plainName("unit0test9"))
    })

    t.Run("an empty name stays out", func(t *testing.T) {
        require.False(t, plainName(""))
    })

    t.Run("anything else stays out", func(t *testing.T) {
        require.False(t, plainName("temp-14"))
        require.False(t, plainName("temp 14"))
        require.False(t, plainName("temp_14"))
        require.False(t, plainName("temp14;rm-rf"))
        require.False(t, plainName("Ünicode"))
    })
}
