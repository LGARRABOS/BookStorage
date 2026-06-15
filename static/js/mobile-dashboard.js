(function () {
  "use strict";

  function closeAllMenus(except) {
    document.querySelectorAll(".work-mobile-menu").forEach(function (menu) {
      if (menu !== except) {
        menu.hidden = true;
        var btn = menu.parentElement && menu.parentElement.querySelector(".work-mobile-menu-btn");
        if (btn) btn.setAttribute("aria-expanded", "false");
      }
    });
  }

  function bindActionMenus() {
    document.addEventListener("click", function (e) {
      var menuBtn = e.target.closest(".work-mobile-menu-btn");
      if (menuBtn) {
        e.preventDefault();
        e.stopPropagation();
        var wrap = menuBtn.closest(".work-mobile-menu-wrap");
        var menu = wrap && wrap.querySelector(".work-mobile-menu");
        if (!menu) return;
        var willOpen = menu.hidden;
        closeAllMenus(willOpen ? menu : null);
        menu.hidden = !willOpen;
        menuBtn.setAttribute("aria-expanded", willOpen ? "true" : "false");
        return;
      }
      if (!e.target.closest(".work-mobile-menu-wrap")) {
        closeAllMenus(null);
      }
    });
  }

  function bindDelete() {
    document.addEventListener("click", function (e) {
      var del = e.target.closest(".js-confirm-delete");
      if (!del) return;
      e.preventDefault();
      closeAllMenus(null);
      var deleteId = del.getAttribute("data-delete-id");
      var msg = del.getAttribute("data-confirm") || "Confirm?";
      showConfirm(msg).then(function (ok) {
        if (!ok || !deleteId) return;
        fetch("/api/delete/" + deleteId, {
          method: "POST",
          credentials: "same-origin",
          headers: { "X-Requested-With": "XMLHttpRequest" },
        })
          .then(function (r) {
            if (r.status === 401) {
              window.location.href = "/login?expired=1";
              return null;
            }
            return r.json();
          })
          .then(function (data) {
            if (data && data.ok) {
              var card = del.closest(".work-mobile-card");
              if (card) card.remove();
              else window.location.reload();
            }
          });
      });
    });
  }

  function bindChapters() {
    document.addEventListener("click", function (e) {
      var btn = e.target.closest(".btn-chapter-inc, .btn-chapter-dec");
      if (!btn) return;
      e.preventDefault();
      var id = btn.getAttribute("data-work-id");
      var isInc = btn.classList.contains("btn-chapter-inc");
      var url = "/api/" + (isInc ? "increment" : "decrement") + "/" + id;
      var counter = document.getElementById("chapter-count-" + id);
      btn.disabled = true;
      fetch(url, { method: "POST", credentials: "same-origin" })
        .then(function (r) {
          if (!r.ok) throw new Error(r.status);
          var cur = parseInt(counter.textContent, 10) || 0;
          counter.textContent = isInc ? cur + 1 : Math.max(0, cur - 1);
        })
        .catch(function () {
          if (counter) counter.classList.add("chapter-error");
        })
        .finally(function () {
          btn.disabled = false;
        });
    });
  }

  function syncChaptersFromAPI() {
    fetch("/api/works?limit=500&sort=title_asc", { credentials: "same-origin" })
      .then(function (r) {
        if (!r.ok) return null;
        return r.json();
      })
      .then(function (payload) {
        if (!payload || !payload.data) return;
        payload.data.forEach(function (work) {
          var el = document.getElementById("chapter-count-" + work.id);
          if (el && String(el.textContent) !== String(work.chapter)) {
            el.textContent = work.chapter;
            el.classList.remove("chapter-error");
          }
        });
      })
      .catch(function () {});
  }

  function bindStaleCheck() {
    var lastCheck = Date.now();
    var MIN_INTERVAL = 30000;

    document.addEventListener("visibilitychange", function () {
      if (document.visibilityState !== "visible") return;
      if (Date.now() - lastCheck < MIN_INTERVAL) return;
      lastCheck = Date.now();
      syncChaptersFromAPI();
    });
  }

  var menusBound = false;
  var deleteBound = false;
  var chaptersBound = false;

  function init() {
    if (!deleteBound) {
      bindDelete();
      deleteBound = true;
    }
    if (!chaptersBound) {
      bindChapters();
      chaptersBound = true;
    }
    if (!menusBound) {
      bindActionMenus();
      menusBound = true;
    }
    bindStaleCheck();
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }

  window.MobileDashboard = {
    rebind: function () {},
    syncChapters: syncChaptersFromAPI,
  };
})();
