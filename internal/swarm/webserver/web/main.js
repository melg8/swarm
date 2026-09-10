/*

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT

*/

// Boot the web interface. The server mode decides the shape: the
// regular bot control boots the bot list and the event streams, the
// pathfind test mode boots the interactive path search instead.
window.addEventListener("DOMContentLoaded", async () => {
  initTheme();
  initTabs();
  initZonePanel();
  initHotkeys();

  let config = null;
  try {
    const response = await fetch("/api/config");
    config = await response.json();
  } catch (err) {
    config = null;
  }

  if (config && config.mode === "pathfind") {
    document.body.classList.add("mode-pathfind");
    MapView.init();
    PathfindUI.init(config);

    return;
  }

  if (config && config.mode === "fight") {
    document.body.classList.add("mode-fight");
    FightUI.init();

    return;
  }

  if (config && config.mode === "test-fight") {
    document.body.classList.add("mode-test-fight");
    FightTest.init();

    return;
  }

  initChat();
  initDumpButton();
  initLegend();
  MapView.init();
  refreshBots();
  setInterval(refreshBots, 2000);
  setInterval(() => {
    if (App.snapshot) { renderFooter(App.snapshot); }
  }, 1000);
  document.getElementById("log-filter")
    .addEventListener("input", () => { App.seenEvents = 0; resetPanels(); });
});

// initLegend wires the collapsible legend head of the sidebar foot
// (C7): the aria-expanded flag carries the state, the css hides the
// rows while it stays false. The note about ticks and rings lives on
// the head tooltip.
function initLegend() {
  const head = document.getElementById("legend-head");
  if (!head || !head.addEventListener) { return; }
  head.addEventListener("click", () => {
    const open = head.getAttribute("aria-expanded") === "true";
    head.setAttribute("aria-expanded", open ? "false" : "true");
  });
}

// Hotkeys (C1): M/L switch the Map/Log tabs, F toggles follow, V opens
// the view layers dropdown, T flips the theme. The keys stay silent
// while the focus sits in a text input, a select or a textarea, and
// while a modifier rides along - the browser shortcuts keep working.
// Every action checks its element exists and is visible (offsetParent),
// so the pathfind and fight modes simply skip the missing pieces.
function initHotkeys() {
  document.addEventListener("keydown", (event) => {
    if (event.ctrlKey || event.altKey || event.metaKey) { return; }
    const active = document.activeElement;
    const tag = active ? active.tagName : "";
    if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") {
      return;
    }
    const key = String(event.key || "").toLowerCase();
    const visible = (el) =>
      Boolean(el) && el.offsetParent !== null;
    if (key === "m" || key === "l") {
      const name = key === "m" ? "map" : "log";
      const tab = document.querySelector('.tab[data-tab="' + name + '"]');
      if (visible(tab) && typeof tab.click === "function") {
        tab.click();
        event.preventDefault();
      }

      return;
    }
    if (key === "f") {
      const follow = document.getElementById("follow");
      if (visible(follow)) {
        follow.checked = !follow.checked;
        follow.dispatchEvent(new Event("change"));
        event.preventDefault();
      }

      return;
    }
    if (key === "v") {
      const btn = document.getElementById("view-menu-btn");
      if (visible(btn) && typeof btn.click === "function") {
        btn.click();
        event.preventDefault();
      }

      return;
    }
    if (key === "t") {
      const btn = document.getElementById("theme-toggle");
      if (btn && typeof btn.click === "function") {
        btn.click();
        event.preventDefault();
      }
    }
  });
}

// Tab switching.
function initTabs() {
  const tabs = document.querySelectorAll(".tab");
  for (const tab of tabs) {
    tab.addEventListener("click", () => {
      for (const t of tabs) { t.classList.remove("active"); }
      tab.classList.add("active");
      const name = tab.dataset.tab;
      for (const content of document.querySelectorAll(".tab-content")) {
        content.classList.toggle("active", content.id === "tab-" + name);
      }
      if (name === "map") {
        MapView.refreshColors();
        MapView.resize();
      }
    });
  }
}
