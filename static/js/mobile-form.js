(function () {
  "use strict";

  function init() {
    if (!document.body.classList.contains("mobile-app-body") &&
        !document.querySelector(".page-body.mobile-page")) {
      return;
    }
    document.querySelectorAll(".mobile-form-accordion .form-card-header").forEach(function (header) {
      header.addEventListener("click", function () {
        var card = header.closest(".mobile-form-accordion");
        if (card) card.classList.toggle("is-collapsed");
      });
    });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
