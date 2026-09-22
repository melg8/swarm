// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package main

import (
    "testing"

    "github.com/melg8/swarm/internal/swarm/pathfind/navmesh"
    "github.com/stretchr/testify/require"
)

// The smoke pass of issue #9: the pure argument parsers of the
// export tool (the region spec, the world triples) and the analysis
// helper, the parts that run before any tile pack is touched.

func TestParseRegions(t *testing.T) {
    t.Run("the X_Y forms parse and dedupe", func(t *testing.T) {
        keys, err := parseRegions("24_19, 24_20,24_19")
        require.NoError(t, err)
        require.Equal(t, []navmesh.RegionKey{
            {Col: 24, Row: 19}, {Col: 24, Row: 20},
        }, keys)
    })

    t.Run("blank parts skip", func(t *testing.T) {
        keys, err := parseRegions(" 24_19 , ,")
        require.NoError(t, err)
        require.Len(t, keys, 1)
    })

    t.Run("negative halves parse", func(t *testing.T) {
        keys, err := parseRegions("-3_-5")
        require.NoError(t, err)
        require.Equal(t, navmesh.RegionKey{Col: -3, Row: -5}, keys[0])
    })

    t.Run("a form without the underscore errors", func(t *testing.T) {
        _, err := parseRegions("2419")
        require.ErrorContains(t, err, "not the X_Y form")
    })

    t.Run("a non numeric half errors", func(t *testing.T) {
        _, err := parseRegions("24_abc")
        require.ErrorContains(t, err, "row")
    })

    t.Run("an empty spec errors", func(t *testing.T) {
        _, err := parseRegions(" , ")
        require.ErrorContains(t, err, "no regions given")
    })
}

func TestParsePos(t *testing.T) {
    t.Run("the x,y,z triple parses", func(t *testing.T) {
        pos, err := parsePos("46112.5, 41500, -3500")
        require.NoError(t, err)
        require.Equal(t, navmesh.Pos{
            X: 46112.5, Y: 41500, Z: -3500,
        }, pos)
    })

    t.Run("a two part form errors", func(t *testing.T) {
        _, err := parsePos("1,2")
        require.ErrorContains(t, err, "not the x,y,z form")
    })

    t.Run("a non numeric part errors", func(t *testing.T) {
        _, err := parsePos("1,2,zzz")
        require.ErrorContains(t, err, "zzz")
    })
}

func TestParseEndpoints(t *testing.T) {
    t.Run("both triples parse", func(t *testing.T) {
        from, to, err := parseEndpoints("1,2,3", "4,5,6")
        require.NoError(t, err)
        require.Equal(t, navmesh.Pos{X: 1, Y: 2, Z: 3}, from)
        require.Equal(t, navmesh.Pos{X: 4, Y: 5, Z: 6}, to)
    })

    t.Run("a bad from wraps its error", func(t *testing.T) {
        _, _, err := parseEndpoints("1,2", "4,5,6")
        require.ErrorContains(t, err, "from:")
    })

    t.Run("a bad to wraps its error", func(t *testing.T) {
        _, _, err := parseEndpoints("1,2,3", "4,5")
        require.ErrorContains(t, err, "to:")
    })
}
