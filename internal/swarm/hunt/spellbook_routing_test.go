// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package hunt

import (
	"testing"

	"github.com/melg8/swarm/internal/swarm/npcdata"
	"github.com/stretchr/testify/require"
)

// The spellbook routing tests of T-020: the lesson book resolution
// across the two town catalogs (the survey follow-up 5 - the level 24
// lessons need the Cure Bleeding book only Dion sells) and the
// merchant lookup that covers the Dion stations.

// TestBookPurchaseVillageBooksStayLocal pins the near stop: the books
// the elven village sells resolve to the village merchant at the 15
// percent village tax, never to the Dion fallback.
func TestBookPurchaseVillageBooksStayLocal(t *testing.T) {
	for _, itemID := range []int32{1513, 1377} {
		purchase, ok := bookPurchase(itemID)
		require.True(t, ok, "the book %d resolves", itemID)
		require.Equal(t, int32(7149),
			purchase.MerchantTemplateID,
			"the book %d is the village Creamees stop", itemID)
		base := npcdata.ItemPrice(itemID)
		require.Equal(t, base+base*15/100, purchase.Price,
			"the book %d carries the 15 percent village tax", itemID)
	}
}

// TestBookPurchaseDionFallback pins the fallback: the Cure Bleeding
// book of the level 24 lessons is not in any village list, so the
// resolution routes through the Dion Sonia stop at the 20 percent
// Dion tax.
func TestBookPurchaseDionFallback(t *testing.T) {
	purchase, ok := bookPurchase(1379)
	require.True(t, ok, "the Cure Bleeding book resolves")
	require.Equal(t, int32(7062), purchase.MerchantTemplateID,
		"the book routes through the Dion Sonia stop")
	require.NotZero(t, purchase.ListID)
	base := npcdata.ItemPrice(1379)
	require.Equal(t, base+base*20/100, purchase.Price,
		"the book carries the 20 percent Dion tax")
}

// TestMerchantByTemplateCoversDion pins the station lookup: the Dion
// merchants resolve to their survey positions so the trip stop
// planning finds the book stop.
func TestMerchantByTemplateCoversDion(t *testing.T) {
	merchant, ok := merchantByTemplate(7062)
	require.True(t, ok)
	require.Equal(t, "Sonia", merchant.Name)
	require.Equal(t, int32(19313), merchant.X)
	require.Equal(t, int32(146229), merchant.Y)
	_, ok = merchantByTemplate(7149)
	require.True(t, ok, "the village lookup stays")
	_, ok = merchantByTemplate(9999)
	require.False(t, ok, "the unknown id stays unknown")
}
