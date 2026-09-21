// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

// Checks the quest sub tab wiring in the live DOM: the badge count,
// the grid swap with a stable panel height and the empty note state.
(() => {
  const panel = document.getElementById("gear-panel");
  const invGrid = document.getElementById("inv-grid");
  const questGrid = document.getElementById("quest-grid");
  const questEmpty = document.getElementById("quest-empty");
  const badge = document.getElementById("inv-tab-quest-badge");
  const itemsTab = document.getElementById("inv-tab-items");
  const questTab = document.getElementById("inv-tab-quest");
  const height = () => Math.round(panel.getBoundingClientRect().height);
  const before = {
    badge: badge.textContent,
    badgeHidden: badge.classList.contains("hidden"),
    invVisible: !invGrid.classList.contains("hidden"),
    questVisible: !questGrid.classList.contains("hidden"),
    itemsTabActive: itemsTab.classList.contains("active"),
    height: height()
  };
  questTab.click();
  const quest = {
    questTabActive: questTab.classList.contains("active"),
    invHidden: invGrid.classList.contains("hidden"),
    questVisible: !questGrid.classList.contains("hidden"),
    emptyHidden: questEmpty.classList.contains("hidden"),
    height: height()
  };
  itemsTab.click();
  const back = {
    itemsTabActive: itemsTab.classList.contains("active"),
    invVisible: !invGrid.classList.contains("hidden"),
    questHidden: questGrid.classList.contains("hidden"),
    height: height()
  };
  const ok = before.invVisible && !before.questVisible
    && before.itemsTabActive && before.badge === "2"
    && !before.badgeHidden
    && quest.questTabActive && quest.invHidden && quest.questVisible
    && quest.emptyHidden
    && back.itemsTabActive && back.invVisible && back.questHidden;
  const heightStable = before.height === quest.height
    && quest.height === back.height;
  return JSON.stringify({
    before: before, quest: quest, back: back,
    heightStable: heightStable,
    verdict: ok && heightStable ? "QUEST_TAB_OK"
      : "QUEST_TAB_BROKEN"
  });
})()
