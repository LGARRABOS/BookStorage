(function () {
  "use strict";

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

  function bindDelete() {
    document.addEventListener("click", function (e) {
      var del = e.target.closest(".js-confirm-delete");
      if (!del) return;
      e.preventDefault();
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
              window.location.href = "/dashboard";
            }
          });
      });
    });
  }

  function init() {
    bindChapters();
    bindDelete();
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
