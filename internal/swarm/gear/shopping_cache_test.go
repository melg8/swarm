// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package gear

import (
    "testing"

    "github.com/stretchr/testify/require"
)

// cacheEntries counts the entries of both gear caches. The test
// helper reads the private state because the cache bounds are exactly
// what the regression pins.
func cacheEntries() (candidates int, jewelIDs int) {
    candidateCache.Range(func(_, _ any) bool {
        candidates++

        return true
    })
    jewelIDCache.Range(func(_, _ any) bool {
        jewelIDs++

        return true
    })

    return candidates, jewelIDs
}

// TestCatalogCandidatesCacheHitsSameContent pins the leak fix: the
// catalog travels by value, so two calls with equal content (two
// fresh copies of the same shops) must hit one cache entry and return
// the same slice. The old pointer key addressed the per call copy -
// every call stored a fresh candidate slice and the maps grew by an
// entry per call forever (the slow continuous heap growth of the long
// runs).
func TestCatalogCandidatesCacheHitsSameContent(t *testing.T) {
    profile := MeleeFighter{}
    first := catalogCandidates(profile, elvenCatalog())
    require.NotEmpty(t, first)
    candidatesBefore, _ := cacheEntries()
    second := catalogCandidates(profile, elvenCatalog())
    require.Same(t, &first[0], &second[0],
        "the equal content catalog must return the cached slice")
    candidatesAfter, _ := cacheEntries()
    require.Equal(t, candidatesBefore, candidatesAfter,
        "the equal content catalog must not grow the cache")
}

// TestCatalogCandidatesCacheBoundedUnderReplanning replays the fleet
// shape that leaked: the hunt loop replans every shoppingPlanPeriod
// (five seconds), each replan passing a fresh by value catalog copy.
// The caches must stay at one entry per distinct content whatever the
// call count.
func TestCatalogCandidatesCacheBoundedUnderReplanning(t *testing.T) {
    profile := MeleeFighter{}
    equipment := equipmentWith(nil, nil)
    candidatesBefore, jewelBefore := cacheEntries()
    for range 50 {
        PlanPurchases(profile, equipment, elvenCatalog(), 500)
    }
    candidatesAfter, jewelAfter := cacheEntries()
    require.LessOrEqual(t, candidatesAfter, candidatesBefore+1,
        "the candidate cache grew beyond one entry per content")
    require.LessOrEqual(t, jewelAfter, jewelBefore+1,
        "the jewel cache grew beyond one entry per content")
}

// TestCatalogHashSeparatesContent pins the key correctness: equal
// content hashes equal (the by value copy case), and every content
// dimension the build reads (a merchant, a tax rate, a buylist) flips
// the hash so a changed catalog rebuilds instead of reusing stale
// offers.
func TestCatalogHashSeparatesContent(t *testing.T) {
    base := elvenCatalog()
    require.Equal(t, catalogHash(base), catalogHash(elvenCatalog()),
        "equal content must hash equal")

    noLists := elvenCatalog()
    noLists.Shops[0].Lists = nil
    require.NotEqual(t, catalogHash(base), catalogHash(noLists),
        "a changed buylist must change the hash")

    otherTax := elvenCatalog()
    otherTax.Shops[1].TaxRate = 0.20
    require.NotEqual(t, catalogHash(base), catalogHash(otherTax),
        "a changed tax rate must change the hash")

    otherMerchant := elvenCatalog()
    otherMerchant.Shops[2].MerchantTemplateID = 7150
    require.NotEqual(t, catalogHash(base), catalogHash(otherMerchant),
        "a changed merchant must change the hash")
}
