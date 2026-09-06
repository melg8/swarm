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

  initChat();
  MapView.init();
  refreshBots();
  setInterval(refreshBots, 2000);
  setInterval(() => {
    if (App.snapshot) { renderFooter(App.snapshot); }
  }, 1000);
  document.getElementById("log-filter")
    .addEventListener("input", () => { App.seenEvents = 0; resetPanels(); });
});

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
