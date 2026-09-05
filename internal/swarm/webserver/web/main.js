/*

SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>

SPDX-License-Identifier: MIT

*/

// Boot the web interface.
window.addEventListener("DOMContentLoaded", () => {
  initTheme();
  initTabs();
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
