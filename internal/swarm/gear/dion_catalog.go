// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package gear

// dionTownTaxRate is the buy tax markup of the Town of Dion
// (MerchantPriceConfig.xml priceConfig id="8", baseTax="20", the castle tax
// is 0): price = baseItemPrice * (1 + taxRate), as at the elven village.
const dionTownTaxRate = 0.20

// The packet template ids (the NpcInfo display ids) of the Dion merchants
// the band gear shopping trip targets (docs/band_20_25_survey.md). The
// buylist ids are the file names of dist/game/data/buylists/, resolved at
// runtime through the generated npcdata.npcBuyLists.
const (
    dionWeaponMerchant  = int32(7060) // Sabrin
    dionArmorMerchant   = int32(7061) // Casey
    dionJewelMerchant   = int32(7062) // Sonia
    dionGroceryMerchant = int32(7063) // Lara
)

// DionCatalog returns the gear catalog of the four Dion merchants the 20-25
// band trip targets (Sabrin weapons, Casey armor, Sonia jewels and
// spellbooks, Lara grocery) at the 20 percent buy tax; the buylist ids of
// 3006000-3006300 resolve through npcdata.BuyListsOfNPC. The elven village
// catalog serves the 1-19 band, and the hunt shopping loop selects the
// catalog by the current town.
func DionCatalog() Catalog {
    return Catalog{Shops: []Shop{
        {
            MerchantTemplateID: dionWeaponMerchant,
            TaxRate:            dionTownTaxRate,
            Lists:              []int32{3006000, 3006001},
        },
        {
            MerchantTemplateID: dionArmorMerchant,
            TaxRate:            dionTownTaxRate,
            Lists:              []int32{3006100, 3006101},
        },
        {
            MerchantTemplateID: dionJewelMerchant,
            TaxRate:            dionTownTaxRate,
            Lists:              []int32{3006200, 3006201},
        },
        {
            MerchantTemplateID: dionGroceryMerchant,
            TaxRate:            dionTownTaxRate,
            Lists:              []int32{3006300},
        },
    }}
}
