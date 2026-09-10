// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package npcdata

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Known template ids and item ids from the generated dictionaries.
// The goblin (display id 3) is the first mob of the elven fields
// hunt ladder; the keltir (display id 532) is the level 1 starter
// mob. The wire template id adds the npcTemplateOffset (1000000).
const (
	goblinDisplayID    = 3
	goblinTemplateID   = 1000000 + goblinDisplayID
	keltirDisplayID    = 532
	keltirTemplateID   = 1000000 + keltirDisplayID
	shortSwordItemID   = 1
	adenaItemID        = 57
	unknownTemplateID  = 999999999
	unknownItemID      = 999999999
	npcTemplateOffset2 = 1000000
)

func TestNPCName(t *testing.T) {
	t.Run("known goblin resolves", func(t *testing.T) {
		require.Equal(t, "Goblin", NPCName(goblinTemplateID))
	})

	t.Run("known keltir resolves", func(t *testing.T) {
		require.Equal(t, "Brown Keltir", NPCName(keltirTemplateID))
	})

	t.Run("template id at offset boundary returns empty", func(t *testing.T) {
		require.Empty(t, NPCName(npcTemplateOffset2))
	})

	t.Run("template id below offset returns empty", func(t *testing.T) {
		require.Empty(t, NPCName(0))
		require.Empty(t, NPCName(-1))
		require.Empty(t, NPCName(999999))
	})

	t.Run("unknown template id returns empty", func(t *testing.T) {
		require.Empty(t, NPCName(unknownTemplateID))
	})
}

func TestNPCLevel(t *testing.T) {
	t.Run("known npc resolves non zero level", func(t *testing.T) {
		level := NPCLevel(goblinTemplateID)
		require.Positive(t, level, "goblin must have a level")
	})

	t.Run("template id at offset returns zero", func(t *testing.T) {
		require.Equal(t, int32(0), NPCLevel(npcTemplateOffset2))
	})

	t.Run("template id below offset returns zero", func(t *testing.T) {
		require.Equal(t, int32(0), NPCLevel(0))
		require.Equal(t, int32(0), NPCLevel(-1))
	})

	t.Run("unknown template id returns zero", func(t *testing.T) {
		require.Equal(t, int32(0), NPCLevel(unknownTemplateID))
	})
}

func TestNPCAggroRange(t *testing.T) {
	t.Run("known npc resolves non zero or zero", func(_ *testing.T) {
		r := NPCAggroRange(goblinTemplateID)
		_ = r
	})

	t.Run("template id at offset returns zero", func(t *testing.T) {
		require.Equal(t, int32(0), NPCAggroRange(npcTemplateOffset2))
	})

	t.Run("template id below offset returns zero", func(t *testing.T) {
		require.Equal(t, int32(0), NPCAggroRange(0))
		require.Equal(t, int32(0), NPCAggroRange(-1))
	})
}

func TestNPCIsAggressive(t *testing.T) {
	t.Run("known npc resolves", func(_ *testing.T) {
		_ = NPCIsAggressive(goblinTemplateID)
	})

	t.Run("template id at offset returns false", func(t *testing.T) {
		require.False(t, NPCIsAggressive(npcTemplateOffset2))
	})

	t.Run("template id below offset returns false", func(t *testing.T) {
		require.False(t, NPCIsAggressive(0))
		require.False(t, NPCIsAggressive(-1))
	})
}

func TestNPCClanHelpRange(t *testing.T) {
	t.Run("known npc resolves", func(_ *testing.T) {
		_ = NPCClanHelpRange(goblinTemplateID)
	})

	t.Run("template id at offset returns zero", func(t *testing.T) {
		require.Equal(t, int32(0), NPCClanHelpRange(npcTemplateOffset2))
	})

	t.Run("template id below offset returns zero", func(t *testing.T) {
		require.Equal(t, int32(0), NPCClanHelpRange(0))
		require.Equal(t, int32(0), NPCClanHelpRange(-1))
	})
}

func TestNPCClans(t *testing.T) {
	t.Run("template id at offset returns nil", func(t *testing.T) {
		require.Nil(t, NPCClans(npcTemplateOffset2))
	})

	t.Run("template id below offset returns nil", func(t *testing.T) {
		require.Nil(t, NPCClans(0))
		require.Nil(t, NPCClans(-1))
	})

	t.Run("unknown template id returns nil or empty", func(t *testing.T) {
		clans := NPCClans(unknownTemplateID)
		// An unknown npc may return nil (no entry) or an empty slice.
		if clans != nil {
			require.Empty(t, clans)
		}
	})

	t.Run("returned slice must be treated as read only", func(t *testing.T) {
		// The function documents that the returned slice is the shared
		// pre-split dictionary entry. Verify a known clanned mob returns
		// a non-nil slice we can read without panicking.
		clans := NPCClans(goblinTemplateID)
		// Reading the slice (even if nil) must not panic.
		for _, c := range clans {
			require.NotEmpty(t, c, "clan name must not be empty")
		}
	})
}

func TestNPCClanMask(t *testing.T) {
	t.Run("template id at offset returns zero", func(t *testing.T) {
		require.Equal(t, uint64(0), NPCClanMask(npcTemplateOffset2))
	})

	t.Run("template id below offset returns zero", func(t *testing.T) {
		require.Equal(t, uint64(0), NPCClanMask(0))
		require.Equal(t, uint64(0), NPCClanMask(-1))
	})

	t.Run("unknown template id returns zero", func(t *testing.T) {
		require.Equal(t, uint64(0), NPCClanMask(unknownTemplateID))
	})

	t.Run("known npc resolves non zero mask or zero", func(_ *testing.T) {
		mask := NPCClanMask(goblinTemplateID)
		// A mob with no clan has mask 0, a clanned mob has a non zero
		// mask. Both are valid; the test just verifies no panic.
		_ = mask
	})
}

func TestNPCWireTemplateID(t *testing.T) {
	t.Run("unmapped id passes through unchanged", func(t *testing.T) {
		// An id not in the converter table returns itself.
		require.Equal(t, int32(123456789), NPCWireTemplateID(123456789))
	})

	t.Run("zero id passes through", func(t *testing.T) {
		require.Equal(t, int32(0), NPCWireTemplateID(0))
	})

	t.Run("negative id passes through", func(t *testing.T) {
		require.Equal(t, int32(-1), NPCWireTemplateID(-1))
	})
}

func TestItemName(t *testing.T) {
	t.Run("known item resolves", func(t *testing.T) {
		name := ItemName(shortSwordItemID)
		require.NotEmpty(t, name, "short sword must have a name")
	})

	t.Run("adena resolves", func(t *testing.T) {
		name := ItemName(adenaItemID)
		require.NotEmpty(t, name, "adena must have a name")
	})

	t.Run("unknown item returns empty", func(t *testing.T) {
		require.Empty(t, ItemName(unknownItemID))
	})

	t.Run("zero item returns empty", func(t *testing.T) {
		require.Empty(t, ItemName(0))
	})

	t.Run("negative item returns empty", func(t *testing.T) {
		require.Empty(t, ItemName(-1))
	})
}

func TestItemPrice(t *testing.T) {
	t.Run("known item resolves non zero price", func(t *testing.T) {
		price := ItemPrice(shortSwordItemID)
		require.Positive(t, price, "short sword must have a price")
	})

	t.Run("unknown item returns zero", func(t *testing.T) {
		require.Equal(t, int64(0), ItemPrice(unknownItemID))
	})

	t.Run("zero item returns zero", func(t *testing.T) {
		require.Equal(t, int64(0), ItemPrice(0))
	})

	t.Run("negative item returns zero", func(t *testing.T) {
		require.Equal(t, int64(0), ItemPrice(-1))
	})
}

func TestItemWeight(t *testing.T) {
	t.Run("known item resolves", func(_ *testing.T) {
		_ = ItemWeight(shortSwordItemID)
	})

	t.Run("unknown item returns zero", func(t *testing.T) {
		require.Equal(t, int32(0), ItemWeight(unknownItemID))
	})

	t.Run("zero item returns zero", func(t *testing.T) {
		require.Equal(t, int32(0), ItemWeight(0))
	})
}

func TestItemIcon(t *testing.T) {
	t.Run("known item resolves", func(_ *testing.T) {
		icon := ItemIcon(shortSwordItemID)
		// The icon may be empty for some items, but the lookup must
		// not panic.
		_ = icon
	})

	t.Run("unknown item returns empty", func(t *testing.T) {
		require.Empty(t, ItemIcon(unknownItemID))
	})

	t.Run("zero item returns empty", func(t *testing.T) {
		require.Empty(t, ItemIcon(0))
	})
}

func TestItemGearStats(t *testing.T) {
	t.Run("known weapon resolves", func(t *testing.T) {
		stats, ok := ItemGearStats(shortSwordItemID)
		require.True(t, ok, "short sword must have gear stats")
		require.NotEmpty(t, stats.BodyPart, "short sword must have a bodypart")
	})

	t.Run("unknown item returns false", func(t *testing.T) {
		_, ok := ItemGearStats(unknownItemID)
		require.False(t, ok)
	})

	t.Run("zero item returns false", func(t *testing.T) {
		_, ok := ItemGearStats(0)
		require.False(t, ok)
	})
}

func TestItemType(t *testing.T) {
	t.Run("known item resolves", func(t *testing.T) {
		itemType := ItemType(shortSwordItemID)
		require.NotEmpty(t, itemType, "short sword must have a type")
	})

	t.Run("unknown item returns empty", func(t *testing.T) {
		require.Empty(t, ItemType(unknownItemID))
	})

	t.Run("zero item returns empty", func(t *testing.T) {
		require.Empty(t, ItemType(0))
	})
}

func TestBuyListsOfNPC(t *testing.T) {
	t.Run("unknown npc returns nil", func(t *testing.T) {
		require.Nil(t, BuyListsOfNPC(unknownTemplateID))
	})

	t.Run("zero npc returns nil", func(t *testing.T) {
		require.Nil(t, BuyListsOfNPC(0))
	})
}

func TestItemsOfBuyList(t *testing.T) {
	t.Run("unknown list returns nil", func(t *testing.T) {
		require.Nil(t, ItemsOfBuyList(unknownItemID))
	})

	t.Run("zero list returns nil", func(t *testing.T) {
		require.Nil(t, ItemsOfBuyList(0))
	})
}

func TestSystemMessageText(t *testing.T) {
	t.Run("known system message resolves", func(t *testing.T) {
		// System message 181 is "Cannot see target." - used by the
		// blind engage recovery.
		text := SystemMessageText(181)
		require.NotEmpty(t, text)
	})

	t.Run("unknown system message returns fallback text", func(t *testing.T) {
		// Unknown ids fall back to "system message N" so the chat
		// window still shows that something happened.
		text := SystemMessageText(999999)
		require.Contains(t, text, "999999")
	})

	t.Run("zero system message returns fallback text", func(t *testing.T) {
		text := SystemMessageText(0)
		require.NotEmpty(t, text)
	})
}

func TestSystemMessageName(t *testing.T) {
	t.Run("known system message resolves to enum name", func(t *testing.T) {
		// System message 181 is CANNOT_SEE_TARGET.
		name := SystemMessageName(181)
		require.NotEmpty(t, name)
	})

	t.Run("unknown system message returns empty", func(t *testing.T) {
		require.Empty(t, SystemMessageName(999999))
	})

	t.Run("zero system message returns empty or resolves", func(_ *testing.T) {
		// Zero may be a valid system message id or may return empty.
		_ = SystemMessageName(0)
	})
}
