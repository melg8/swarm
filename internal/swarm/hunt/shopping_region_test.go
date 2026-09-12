// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestShopCatalogForRegionElvenDefault pins the default: an empty or
// unknown region returns the elven village catalog (the 1-19 band
// default), so the M0 acceptance stays unchanged.
func TestShopCatalogForRegionElvenDefault(t *testing.T) {
	for _, region := range []string{"", "elven", "unknown"} {
		catalog := shopCatalogForRegion(region)
		require.NotEmpty(t, catalog.Shops,
			"region %q must fall back to the elven catalog", region)
		require.Equal(t, int32(7147),
			catalog.Shops[0].MerchantTemplateID,
			"region %q must select the elven weapon merchant", region)
	}
}

// TestShopCatalogForRegionDion pins the Dion selection: the Dion region
// returns the Dion merchants (Sabrin 7060 first) at the 20 percent
// Dion tax, distinct from the elven catalog.
func TestShopCatalogForRegionDion(t *testing.T) {
	catalog := shopCatalogForRegion(regionDion)
	require.Len(t, catalog.Shops, 4, "the Dion catalog has four shops")
	require.Equal(t, int32(7060),
		catalog.Shops[0].MerchantTemplateID,
		"the first Dion shop is Sabrin (7060)")
	require.InDelta(t, 0.20, catalog.Shops[0].TaxRate, 0.001,
		"the Dion tax rate is 20 percent")
}

// TestShopCatalogForRegionElvenTaxStays pins the elven tax: the elven
// region keeps the 15 percent tax the M0 acceptance prices against.
func TestShopCatalogForRegionElvenTaxStays(t *testing.T) {
	catalog := shopCatalogForRegion(regionElven)
	require.NotEmpty(t, catalog.Shops)
	for _, shop := range catalog.Shops {
		require.InDelta(t, 0.15, shop.TaxRate, 0.001,
			"the elven tax rate stays 15 percent")
	}
}

// TestDionMerchantsList pins the Dion merchant set of the survey: the
// four merchants the 20-25 band shopping trip targets, at their survey
// positions.
func TestDionMerchantsList(t *testing.T) {
	require.Len(t, dionMerchants, 4)
	names := map[string]bool{}
	for _, m := range dionMerchants {
		names[m.Name] = true
		require.Positive(t, m.TemplateID)
		require.Positive(t, m.X)
		require.Positive(t, m.Y)
	}
	for _, want := range []string{"Sabrin", "Casey", "Sonia", "Lara"} {
		require.True(t, names[want], "the Dion merchant %q is present",
			want)
	}
}

// TestDionShopCatalogBuildsFromBuylists pins the live buylist data: the
// Dion catalog shops carry the buylist ids the npcdata generator loaded
// (3006000-3006300), so the gear planner prices real D-grade items.
func TestDionShopCatalogBuildsFromBuylists(t *testing.T) {
	catalog := shopCatalogForRegion(regionDion)
	lists := map[int32]bool{}
	for _, shop := range catalog.Shops {
		for _, listID := range shop.Lists {
			lists[listID] = true
		}
	}
	for _, want := range []int32{
		3006000, 3006001, 3006100, 3006101,
		3006200, 3006201, 3006300,
	} {
		require.True(t, lists[want],
			"the Dion buylist %d is in the catalog", want)
	}
}
