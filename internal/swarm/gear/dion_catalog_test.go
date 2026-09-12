// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package gear

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDionCatalogShops pins the Dion merchant set of the 20-25 band
// shopping trip: the four merchants the survey names (Sabrin, Casey,
// Sonia, Lara), the Dion 20 percent buy tax and the buylist ids the
// npcdata generator already loaded.
func TestDionCatalogShops(t *testing.T) {
	catalog := DionCatalog()
	require.Len(t, catalog.Shops, 4, "the Dion catalog has four shops")
	weapon := catalog.Shops[0]
	require.Equal(t, int32(7060), weapon.MerchantTemplateID,
		"the weapon merchant is Sabrin (7060)")
	require.InDelta(t, 0.20, weapon.TaxRate, 0.001,
		"the Dion baseTax is 20 percent")
	require.Equal(t, []int32{3006000, 3006001}, weapon.Lists,
		"the fighter + mystic weapon buylists")
	armor := catalog.Shops[1]
	require.Equal(t, int32(7061), armor.MerchantTemplateID)
	require.Equal(t, []int32{3006100, 3006101}, armor.Lists)
	jewel := catalog.Shops[2]
	require.Equal(t, int32(7062), jewel.MerchantTemplateID)
	require.Equal(t, []int32{3006200, 3006201}, jewel.Lists)
	grocery := catalog.Shops[3]
	require.Equal(t, int32(7063), grocery.MerchantTemplateID)
	require.Equal(t, []int32{3006300}, grocery.Lists)
}

// TestDionCatalogTaxMatchesElvenShape pins the tax shape: the Dion
// tax rate is the baseTax of the MerchantPriceConfig (20 percent),
// the same shape the elven catalog uses (15 percent), so the gear
// planner prices the Dion items with the buyPrice formula the
// shopping strategy expects.
func TestDionCatalogTaxMatchesElvenShape(t *testing.T) {
	dion := DionCatalog()
	elven := elvenCatalog()
	for _, shop := range dion.Shops {
		require.Positive(t, shop.TaxRate,
			"the Dion tax rate is positive")
	}
	for _, shop := range elven.Shops {
		require.Positive(t, shop.TaxRate,
			"the elven tax rate is positive (the shape mirror)")
	}
	require.Greater(t, dion.Shops[0].TaxRate, elven.Shops[0].TaxRate,
		"the Dion tax (20%%) is higher than the elven (15%%)")
}
