(function () {
  "use strict";

  var STORAGE_KEY = "bs_mobile_bd_filters";

  function normalize(v) {
    return (v || "").toLowerCase();
  }

  function getEls() {
    return {
      search: document.getElementById("mobile-bd-search"),
      status: document.getElementById("mobile-bd-filter-status"),
      adultOnlyCheck: document.getElementById("mobile-bd-adult-only-check"),
      sortSel: document.getElementById("mobile-bd-sort"),
      quickFilters: document.getElementById("mobile-bd-quick-filters"),
      badge: document.getElementById("mobile-bd-filters-badge"),
      summary: document.getElementById("mobile-bd-summary"),
      filtersOpen: document.getElementById("mobile-bd-filters-open"),
      filtersClose: document.getElementById("mobile-bd-filters-close"),
      filtersApply: document.getElementById("mobile-bd-filters-apply"),
      filtersSheet: document.getElementById("mobile-bd-filters-sheet"),
      filtersOverlay: document.getElementById("mobile-bd-filters-overlay"),
    };
  }

  function getRows() {
    return Array.prototype.slice.call(
      document.querySelectorAll("#bd-mobile-list .bd-mobile-row")
    );
  }

  function saveState(els) {
    try {
      sessionStorage.setItem(
        STORAGE_KEY,
        JSON.stringify({
          status: els.status ? els.status.value : "",
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
        if (els.status) els.status.value = "";
        syncQuickTabs(els, "");
        return false;
      }
      var state = JSON.parse(raw);
      if (els.search && state.search) els.search.value = state.search;
      if (els.status) {
        els.status.value =
          state.status === undefined || state.status === null
            ? ""
            : state.status;
      }
      if (els.adultOnlyCheck) els.adultOnlyCheck.checked = !!state.adult;
      if (els.sortSel && state.sort) els.sortSel.value = state.sort;
      syncQuickTabs(els, els.status ? els.status.value : "");
      return true;
    } catch (e) {
      if (els.status) els.status.value = "";
      syncQuickTabs(els, "");
      return false;
    }
  }

  function syncQuickTabs(els, statusVal) {
    if (!els.quickFilters) return;
    var tabs = els.quickFilters.querySelectorAll(".mobile-collection-tab");
    var activeStatus =
      statusVal === undefined || statusVal === null ? "" : statusVal;
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
    rows.forEach(function (row) {
      if (row.style.display === "none") return;
      visible += 1;
    });
    var worksLabel = els.summary.getAttribute("data-label-works") || "works";
    els.summary.textContent = visible + " " + worksLabel;
  }

  function applyClientFilters(els) {
    var rows = getRows();
    var q = normalize(els.search ? els.search.value : "");
    var s = els.status ? els.status.value : "";
    rows.forEach(function (row) {
      var title = normalize(row.getAttribute("data-title"));
      var cardStatus = row.getAttribute("data-status") || "";
      var statuses = (row.getAttribute("data-statuses") || "")
        .split("|")
        .filter(Boolean);
      var matchSearch = !q || title.indexOf(q) !== -1;
      var matchStatus =
        !s || cardStatus === s || statuses.indexOf(s) !== -1;
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
    var restored = restoreState(els);
    var adultInURL =
      new URL(window.location.href).searchParams.get("adult") === "only";
    if (adultInURL) {
      if (els.adultOnlyCheck) els.adultOnlyCheck.checked = true;
      if (els.status) {
        els.status.value = "";
        syncQuickTabs(els, "");
      }
    } else if (
      restored &&
      els.adultOnlyCheck &&
      els.adultOnlyCheck.checked
    ) {
      reloadWithServerParams(els);
      return;
    }
    applyClientFilters(els);
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
