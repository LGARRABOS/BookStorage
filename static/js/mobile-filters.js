(function () {
  "use strict";

  var STORAGE_KEY = "bs_mobile_filters";

  function normalize(v) {
    return (v || "").toLowerCase();
  }

  function getEls() {
    return {
      search: document.getElementById("mobile-search"),
      status: document.getElementById("mobile-filter-status"),
      followCheck: document.getElementById("mobile-follow-unfollowed-check"),
      adultOnlyCheck: document.getElementById("mobile-adult-only-check"),
      site: document.getElementById("mobile-filter-site"),
      sortSel: document.getElementById("mobile-sort"),
      quickFilters: document.getElementById("mobile-quick-filters"),
      badge: document.getElementById("mobile-filters-badge"),
      filtersOpen: document.getElementById("mobile-filters-open"),
      filtersClose: document.getElementById("mobile-filters-close"),
      filtersApply: document.getElementById("mobile-filters-apply"),
      filtersSheet: document.getElementById("mobile-filters-sheet"),
      filtersOverlay: document.getElementById("mobile-filters-overlay"),
      worksContainer: document.getElementById("mobile-works-container"),
    };
  }

  function getCards() {
    return Array.prototype.slice.call(
      document.querySelectorAll("#works-mobile-list .work-mobile-card")
    );
  }

  function saveState(els) {
    try {
      sessionStorage.setItem(
        STORAGE_KEY,
        JSON.stringify({
          status: els.status ? els.status.value : "",
          site: els.site ? els.site.value : "",
          follow: els.followCheck ? els.followCheck.checked : false,
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
      if (!raw) return;
      var state = JSON.parse(raw);
      if (els.search && state.search) els.search.value = state.search;
      if (els.status && state.status) els.status.value = state.status;
      if (els.site && state.site) els.site.value = state.site;
      if (els.followCheck) els.followCheck.checked = !!state.follow;
      if (els.adultOnlyCheck) els.adultOnlyCheck.checked = !!state.adult;
      if (els.sortSel && state.sort) els.sortSel.value = state.sort;
      syncQuickChips(els, state.status || "");
    } catch (e) {}
  }

  function syncQuickChips(els, statusVal) {
    if (!els.quickFilters) return;
    var chips = els.quickFilters.querySelectorAll(".mobile-filter-chip");
    for (var i = 0; i < chips.length; i += 1) {
      var chipStatus = chips[i].getAttribute("data-status") || "";
      var active = chipStatus === (statusVal || "");
      chips[i].classList.toggle("is-active", active);
      chips[i].setAttribute("aria-selected", active ? "true" : "false");
    }
  }

  function countActiveFilters(els) {
    var n = 0;
    if (els.status && els.status.value) n += 1;
    if (els.site && els.site.value) n += 1;
    if (els.followCheck && els.followCheck.checked) n += 1;
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

  function applyClientFilters(els) {
    var cards = getCards();
    if (!cards.length) return;
    var q = normalize(els.search ? els.search.value : "");
    var s = els.status ? normalize(els.status.value) : "";
    var fl = els.followCheck && els.followCheck.checked ? "unfollowed" : "";
    var siteVal = els.site ? els.site.value : "";

    cards.forEach(function (card) {
      var title = normalize(card.getAttribute("data-title"));
      var cs = normalize(card.getAttribute("data-status"));
      var rawStatus = card.getAttribute("data-status") || "";
      var notifyVal = card.getAttribute("data-notify-new-chapters") || "1";
      var cardSite = card.getAttribute("data-reading-site-id") || "none";
      var matchFollow =
        !fl || (fl === "unfollowed" && rawStatus === "En cours" && notifyVal === "0");
      var matchSite =
        !siteVal || (siteVal === "none" ? cardSite === "none" : cardSite === siteVal);
      var visible =
        (!q || title.indexOf(q) !== -1) &&
        (!s || cs === s) &&
        matchFollow &&
        matchSite;
      card.style.display = visible ? "" : "none";
    });

    saveState(els);
    updateBadge(els);
  }

  function buildServerUrl(els) {
    var u = new URL(window.location.href);
    u.searchParams.delete("partial");
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
    u.searchParams.set("partial", "works");
    return u.toString();
  }

  function reloadWorksList(els) {
    var url = buildServerUrl(els);
    saveState(els);
    fetch(url, {
      credentials: "same-origin",
      headers: { "X-Requested-With": "XMLHttpRequest" },
    })
      .then(function (r) {
        if (r.status === 401) {
          window.location.href = "/login?expired=1";
          return null;
        }
        if (!r.ok) throw new Error(r.status);
        return r.text();
      })
      .then(function (html) {
        if (!html || !els.worksContainer) return;
        els.worksContainer.innerHTML = html;
        if (window.MobileDashboard && window.MobileDashboard.rebind) {
          window.MobileDashboard.rebind();
        }
        applyClientFilters(els);
        var newUrl = new URL(buildServerUrl(els));
        newUrl.searchParams.delete("partial");
        window.history.replaceState({}, "", newUrl.toString());
      })
      .catch(function () {
        var u = new URL(buildServerUrl(els));
        u.searchParams.delete("partial");
        window.location.href = u.toString();
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
        syncQuickChips(els, els.status ? els.status.value : "");
        applyClientFilters(els);
        reloadWorksList(els);
        window.MobileShell.closeSheet(els.filtersSheet, els.filtersOverlay);
      });
    }
  }

  function initQuickFilters(els) {
    if (!els.quickFilters) return;
    els.quickFilters.addEventListener("click", function (e) {
      var chip = e.target.closest(".mobile-filter-chip");
      if (!chip) return;
      var statusVal = chip.getAttribute("data-status") || "";
      if (els.status) els.status.value = statusVal;
      syncQuickChips(els, statusVal);
      applyClientFilters(els);
    });
  }

  function init() {
    var els = getEls();
    restoreState(els);
    updateBadge(els);

    if (els.search) els.search.addEventListener("input", function () {
      applyClientFilters(els);
    });
    if (els.status) els.status.addEventListener("change", function () {
      syncQuickChips(els, els.status.value);
      applyClientFilters(els);
    });
    if (els.followCheck) els.followCheck.addEventListener("change", function () {
      applyClientFilters(els);
      updateBadge(els);
    });
    if (els.site) els.site.addEventListener("change", function () {
      applyClientFilters(els);
      updateBadge(els);
    });
    document.addEventListener("workstatuschanged", function () {
      applyClientFilters(els);
    });

    initQuickFilters(els);
    initFiltersSheet(els);
    applyClientFilters(els);
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }

  window.MobileFilters = { applyClientFilters: function () {
    applyClientFilters(getEls());
  }};
})();
