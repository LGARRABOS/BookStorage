(function () {
  "use strict";

  function openSheet(sheet, overlay) {
    if (!sheet || !overlay) return;
    overlay.hidden = false;
    sheet.hidden = false;
    requestAnimationFrame(function () {
      overlay.classList.add("is-open");
      sheet.classList.add("is-open");
    });
    document.body.style.overflow = "hidden";
  }

  function closeSheet(sheet, overlay) {
    if (!sheet || !overlay) return;
    overlay.classList.remove("is-open");
    sheet.classList.remove("is-open");
    document.body.style.overflow = "";
    setTimeout(function () {
      if (!sheet.classList.contains("is-open")) {
        sheet.hidden = true;
        overlay.hidden = true;
      }
    }, 280);
  }

  function initSheet(openBtn, closeBtn, sheet, overlay) {
    if (!sheet || !overlay) return;
    if (openBtn) {
      openBtn.addEventListener("click", function () {
        openSheet(sheet, overlay);
      });
    }
    if (closeBtn) {
      closeBtn.addEventListener("click", function () {
        closeSheet(sheet, overlay);
      });
    }
    overlay.addEventListener("click", function () {
      closeSheet(sheet, overlay);
    });
  }

  function initSearchToggle() {
    var toggle = document.getElementById("mobile-search-toggle");
    var panel = document.getElementById("mobile-search-panel");
    var input = document.getElementById("mobile-search");
    if (!toggle || !panel) return;

    toggle.addEventListener("click", function () {
      var willShow = panel.hidden;
      panel.hidden = !willShow;
      toggle.setAttribute("aria-expanded", willShow ? "true" : "false");
      if (willShow && input) {
        setTimeout(function () {
          input.focus();
        }, 50);
      }
    });
  }

  function initSettingsSheet() {
    initSheet(
      document.getElementById("mobile-settings-open"),
      document.getElementById("mobile-settings-close"),
      document.getElementById("mobile-settings-sheet"),
      document.getElementById("mobile-settings-overlay")
    );
  }

  function initInstallBanner() {
    var banner = document.getElementById("mobile-install-banner");
    var dismiss = document.getElementById("mobile-install-dismiss");
    if (!banner) return;

    var key = "bs_mobile_install_dismissed";
    if (localStorage.getItem(key) === "1") return;

    var isStandalone =
      window.matchMedia("(display-mode: standalone)").matches ||
      window.navigator.standalone === true;
    if (isStandalone) return;

    var ua = navigator.userAgent || "";
    var isIOS = /iphone|ipad|ipod/i.test(ua);
    var isAndroid = /android/i.test(ua);
    if (!isIOS && !isAndroid) return;

    var iosHint = banner.querySelector("[data-install-ios]");
    var androidHint = banner.querySelector("[data-install-android]");
    if (isIOS && androidHint) androidHint.hidden = true;
    if (isAndroid && iosHint) iosHint.hidden = true;

    banner.hidden = false;

    if (dismiss) {
      dismiss.addEventListener("click", function () {
        banner.hidden = true;
        localStorage.setItem(key, "1");
      });
    }
  }

  function init() {
    initSearchToggle();
    initSettingsSheet();
    initInstallBanner();
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }

  window.MobileShell = {
    openSheet: openSheet,
    closeSheet: closeSheet,
  };
})();
