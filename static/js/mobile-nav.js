(function () {
  function init() {
    var toggle = document.querySelector("[data-mobile-menu-toggle]");
    var menu = document.querySelector("[data-mobile-menu]");
    if (toggle && menu) {
      toggle.addEventListener("click", function () {
        var isOpen = menu.classList.toggle("is-open");
        toggle.setAttribute("aria-expanded", isOpen ? "true" : "false");
      });
    }

    var current = window.location.pathname;
    var links = document.querySelectorAll(".mobile-bottom-link[data-nav]");
    for (var i = 0; i < links.length; i += 1) {
      var href = links[i].getAttribute("data-nav");
      if (!href) continue;
      var isActive = false;
      if (href === "/" || href === "/hub") {
        isActive = current === "/" || current === "" || current === "/hub";
      } else if (href === "/anime/dashboard") {
        isActive =
          current === "/anime/dashboard" ||
          current.indexOf("/anime/edit/") === 0;
      } else if (href === "/manga/dashboard") {
        isActive =
          current === "/manga/dashboard" ||
          current.indexOf("/manga/work/") === 0;
      } else {
        isActive = current === href || current.indexOf(href) === 0;
      }
      if (isActive) {
        links[i].classList.add("is-active");
      }
    }

    var accountOpen = document.getElementById("mobile-account-open");
    if (accountOpen && window.MobileShell) {
      accountOpen.addEventListener("click", function () {
        var sheet = document.getElementById("mobile-settings-sheet");
        var overlay = document.getElementById("mobile-settings-overlay");
        if (sheet && overlay) {
          window.MobileShell.openSheet(sheet, overlay);
        }
      });
    }

    var langToggle = document.querySelector("[data-lang-burger-toggle]");
    var langPanel = document.querySelector("[data-lang-burger-panel]");
    var langRoot = document.querySelector("[data-lang-burger]");
    if (langToggle && langPanel && langRoot) {
      langToggle.addEventListener("click", function (e) {
        e.stopPropagation();
        var willShow = langPanel.hasAttribute("hidden");
        if (willShow) {
          langPanel.removeAttribute("hidden");
          langToggle.setAttribute("aria-expanded", "true");
        } else {
          langPanel.setAttribute("hidden", "");
          langToggle.setAttribute("aria-expanded", "false");
        }
      });
      document.addEventListener(
        "click",
        function (e) {
          if (langPanel.hasAttribute("hidden")) return;
          if (langRoot.contains(e.target)) return;
          langPanel.setAttribute("hidden", "");
          langToggle.setAttribute("aria-expanded", "false");
        },
        false
      );
    }
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
