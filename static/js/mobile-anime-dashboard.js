(function () {
  "use strict";

  var STORAGE_KEY = "bs_mobile_anime_filters";

  function normalize(v) {
    return (v || "").toLowerCase();
  }

  function getEls() {
    return {
      search: document.getElementById("mobile-anime-search"),
      status: document.getElementById("mobile-anime-filter-status"),
      adultOnlyCheck: document.getElementById("mobile-anime-adult-only-check"),
      sortSel: document.getElementById("mobile-anime-sort"),
      quickFilters: document.getElementById("mobile-anime-quick-filters"),
      badge: document.getElementById("mobile-anime-filters-badge"),
      summary: document.getElementById("mobile-anime-summary"),
      filtersOpen: document.getElementById("mobile-anime-filters-open"),
      filtersClose: document.getElementById("mobile-anime-filters-close"),
      filtersApply: document.getElementById("mobile-anime-filters-apply"),
      filtersSheet: document.getElementById("mobile-anime-filters-sheet"),
      filtersOverlay: document.getElementById("mobile-anime-filters-overlay"),
    };
  }

  function getRows() {
    return Array.prototype.slice.call(
      document.querySelectorAll("#anime-mobile-list .anime-mobile-row")
    );
  }

  function saveState(els) {
    try {
      sessionStorage.setItem(
        STORAGE_KEY,
        JSON.stringify({
          status: els.status ? els.status.value : "En cours",
          adult: els.adultOnlyCheck ? els.adultOnlyCheck.checked : false,
          sort: els.sortSel ? els.sortSel.value : "title",
          search: els.search ? els.search.value : "",
        })
      );
    } catch (e) {}
  }

  function restoreState(els) {
    try {
      var raw = sessionStorage.getItem(STORAGE_KEY);
      if (!raw) {
        if (els.status) els.status.value = "En cours";
        syncQuickTabs(els, "En cours");
        return false;
      }
      var state = JSON.parse(raw);
      if (els.search && state.search) els.search.value = state.search;
      if (els.status) {
        els.status.value =
          state.status === undefined || state.status === null
            ? "En cours"
            : state.status;
      }
      if (els.adultOnlyCheck) els.adultOnlyCheck.checked = !!state.adult;
      if (els.sortSel && state.sort) els.sortSel.value = state.sort;
      syncQuickTabs(els, els.status ? els.status.value : "En cours");
      return true;
    } catch (e) {
      if (els.status) els.status.value = "En cours";
      syncQuickTabs(els, "En cours");
      return false;
    }
  }

  function syncQuickTabs(els, statusVal) {
    if (!els.quickFilters) return;
    var tabs = els.quickFilters.querySelectorAll(".mobile-collection-tab");
    var activeStatus =
      statusVal === undefined || statusVal === null ? "En cours" : statusVal;
    for (var i = 0; i < tabs.length; i += 1) {
      var tabStatus = tabs[i].getAttribute("data-status") || "";
      var active = tabStatus === activeStatus;
      tabs[i].classList.toggle("is-active", active);
      tabs[i].setAttribute("aria-selected", active ? "true" : "false");
    }
  }

  function countActiveFilters(els) {
    var n = 0;
    if (els.adultOnlyCheck && els.adultOnlyCheck.checked) n += 1;
    if (els.sortSel && els.sortSel.value && els.sortSel.value !== "title") n += 1;
    return n;
  }

  function updateBadge(els) {
    if (!els.badge) return;
    var n = countActiveFilters(els);
    if (n > 0) {
      els.badge.textContent = String(n);
      els.badge.hidden = false;
    } else {
      els.badge.hidden = true;
    }
  }

  function updateSummary(els) {
    if (!els.summary) return;
    var rows = getRows();
    var visible = 0;
    var episodes = 0;
    rows.forEach(function (row) {
      if (row.style.display === "none") return;
      visible += 1;
      episodes += parseInt(row.getAttribute("data-episode"), 10) || 0;
    });
    var worksLabel = els.summary.getAttribute("data-label-works") || "works";
    var episodesLabel =
      els.summary.getAttribute("data-label-episodes") || "episodes";
    els.summary.textContent =
      visible + " " + worksLabel + " • " + episodes + " " + episodesLabel;
  }

  function applyClientFilters(els) {
    var rows = getRows();
    var q = normalize(els.search ? els.search.value : "");
    var s = els.status ? els.status.value : "En cours";
    rows.forEach(function (row) {
      var title = normalize(row.getAttribute("data-title"));
      var cardStatus = row.getAttribute("data-status") || "";
      var matchSearch = !q || title.indexOf(q) !== -1;
      var matchStatus = !s || cardStatus === s;
      row.style.display = matchSearch && matchStatus ? "" : "none";
    });
    saveState(els);
    updateBadge(els);
    updateSummary(els);
  }

  function reloadWithServerParams(els) {
    var u = new URL(window.location.href);
    if (els.sortSel && els.sortSel.value && els.sortSel.value !== "title") {
      u.searchParams.set("sort", els.sortSel.value);
    } else {
      u.searchParams.delete("sort");
    }
    if (els.adultOnlyCheck && els.adultOnlyCheck.checked) {
      u.searchParams.set("adult", "only");
    } else {
      u.searchParams.delete("adult");
    }
    saveState(els);
    window.location.href = u.toString();
  }

  function post(url) {
    return fetch(url, {
      method: "POST",
      headers: { "X-Requested-With": "XMLHttpRequest" },
      credentials: "same-origin",
    });
  }

  function bindEpisodeControls() {
    document.querySelectorAll(".anime-mobile-ep-controls").forEach(function (counter) {
      var id = counter.getAttribute("data-id");
      var countEl = counter.querySelector(".anime-ep-count");
      var row = counter.closest(".anime-mobile-row");
      var epValue = row ? row.querySelector(".ep-value") : null;
      function setCount(n) {
        countEl.textContent = String(n);
        if (epValue) epValue.textContent = String(n);
        if (row) row.setAttribute("data-episode", String(n));
        updateSummary(getEls());
      }
      var plus = counter.querySelector(".anime-ep-plus");
      var minus = counter.querySelector(".anime-ep-minus");
      if (plus) {
        plus.addEventListener("click", function (e) {
          e.preventDefault();
          e.stopPropagation();
          post("/api/anime/increment/" + id).then(function (r) {
            if (r.ok) setCount((parseInt(countEl.textContent, 10) || 0) + 1);
          });
        });
      }
      if (minus) {
        minus.addEventListener("click", function (e) {
          e.preventDefault();
          e.stopPropagation();
          var cur = parseInt(countEl.textContent, 10) || 0;
          if (cur <= 0) return;
          post("/api/anime/decrement/" + id).then(function (r) {
            if (r.ok) setCount(cur - 1);
          });
        });
      }
    });
  }

  function initFiltersSheet(els) {
    if (!els.filtersSheet || !els.filtersOverlay || !window.MobileShell) return;
    if (els.filtersOpen) {
      els.filtersOpen.addEventListener("click", function () {
        window.MobileShell.openSheet(els.filtersSheet, els.filtersOverlay);
      });
    }
    if (els.filtersClose) {
      els.filtersClose.addEventListener("click", function () {
        window.MobileShell.closeSheet(els.filtersSheet, els.filtersOverlay);
      });
    }
    if (els.filtersApply) {
      els.filtersApply.addEventListener("click", function () {
        syncQuickTabs(els, els.status ? els.status.value : "");
        applyClientFilters(els);
        window.MobileShell.closeSheet(els.filtersSheet, els.filtersOverlay);
        reloadWithServerParams(els);
      });
    }
    if (els.filtersOverlay) {
      els.filtersOverlay.addEventListener("click", function () {
        window.MobileShell.closeSheet(els.filtersSheet, els.filtersOverlay);
      });
    }
  }

  function init() {
    var els = getEls();
    restoreState(els);
    applyClientFilters(els);
    bindEpisodeControls();
    initFiltersSheet(els);

    if (els.search) {
      els.search.addEventListener("input", function () {
        applyClientFilters(els);
      });
    }
    if (els.quickFilters) {
      els.quickFilters.addEventListener("click", function (e) {
        var tab = e.target.closest(".mobile-collection-tab");
        if (!tab) return;
        var status = tab.getAttribute("data-status") || "";
        if (els.status) els.status.value = status;
        syncQuickTabs(els, status);
        applyClientFilters(els);
      });
    }
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
