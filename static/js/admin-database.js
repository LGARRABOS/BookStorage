(function () {
  "use strict";

  var i18n = document.getElementById("admin-db-i18n");
  if (!i18n) return;

  function dbAttr(k, fb) {
    return i18n.getAttribute("data-" + k) || fb;
  }

  var confirmMsg = dbAttr("confirm-delete", "Delete this row?");
  var errGeneric = dbAttr("delete-error", "Delete failed.");
  var filterCountTpl = dbAttr("filter-count", "{shown} / {total}");
  var copyLabel = dbAttr("copy", "Copy");
  var copiedLabel = dbAttr("copied", "Copied");
  var truncatedLabel = dbAttr("truncated", "limited");
  var rowsLabel = dbAttr("rows-label", "rows");
  var kpiTruncatedNone = dbAttr("kpi-truncated-none", "None");

  function dbConfirmDelete() {
    if (typeof window.showConfirm !== "function") {
      return Promise.resolve(window.confirm(confirmMsg));
    }
    return window.showConfirm(confirmMsg, {
      title: dbAttr("modal-confirm-title", "Confirm"),
      okLabel: dbAttr("modal-delete-ok", "Delete"),
      cancelLabel: dbAttr("modal-cancel", "Cancel"),
      okClass: "btn-danger",
    });
  }

  function dbAlert(msg) {
    if (typeof window.showAlert !== "function") {
      window.alert(msg);
      return Promise.resolve();
    }
    return window.showAlert(msg, {
      title: dbAttr("modal-error-title", "Error"),
      okLabel: dbAttr("modal-ok", "OK"),
    });
  }

  function formatFilterCount(shown, total) {
    return filterCountTpl
      .replace("{shown}", String(shown))
      .replace("{total}", String(total));
  }

  function slugTable(name) {
    return String(name || "").trim().toLowerCase().replace(/[^a-z0-9_-]+/g, "_");
  }

  function initKpi() {
    var pills = document.querySelectorAll(".db-table-pill");
    var rowSum = 0;
    var truncCount = 0;
    pills.forEach(function (pill) {
      rowSum += parseInt(pill.getAttribute("data-total") || "0", 10) || 0;
      if (pill.getAttribute("data-truncated") === "true") truncCount++;
    });
    var rowsEl = document.getElementById("db-kpi-rows");
    if (rowsEl) rowsEl.textContent = String(rowSum);
    var truncEl = document.getElementById("db-kpi-truncated");
    if (truncEl) {
      truncEl.textContent = truncCount > 0 ? String(truncCount) : kpiTruncatedNone;
    }
  }

  function showPanel(tableName) {
    var slug = slugTable(tableName);
    var panels = document.querySelectorAll(".db-panel");
    var pills = document.querySelectorAll(".db-table-pill");
    var found = false;

    panels.forEach(function (panel) {
      var match = panel.getAttribute("data-table") === tableName;
      if (match) {
        panel.hidden = false;
        found = true;
      } else {
        panel.hidden = true;
      }
    });

    pills.forEach(function (pill) {
      var active = pill.getAttribute("data-table") === tableName;
      pill.classList.toggle("active", active);
      pill.setAttribute("aria-selected", active ? "true" : "false");
    });

    if (found) {
      var hash = "#" + slug;
      if (window.location.hash !== hash) {
        history.replaceState(null, "", hash);
      }
    }
    return found;
  }

  function initTableSwitcher() {
    var nav = document.getElementById("db-table-nav");
    if (!nav) return;

    nav.addEventListener("click", function (e) {
      var pill = e.target.closest(".db-table-pill");
      if (!pill) return;
      var table = pill.getAttribute("data-table");
      if (table) showPanel(table);
    });

    var hash = (window.location.hash || "").replace(/^#/, "");
    if (hash) {
      var pills = nav.querySelectorAll(".db-table-pill");
      for (var i = 0; i < pills.length; i++) {
        if (slugTable(pills[i].getAttribute("data-table")) === hash) {
          showPanel(pills[i].getAttribute("data-table"));
          return;
        }
      }
    }

    var first = nav.querySelector(".db-table-pill");
    if (first) showPanel(first.getAttribute("data-table"));
  }

  function applyFilter(panel) {
    var input = panel.querySelector(".db-search-input");
    var table = panel.querySelector("table[data-db-table]");
    var countEl = panel.querySelector(".db-filter-count");
    var clearBtn = panel.querySelector(".db-search-clear");
    if (!input || !table) return;

    var q = (input.value || "").trim().toLowerCase();
    var rows = table.querySelectorAll("tbody tr");
    var shown = 0;
    rows.forEach(function (tr) {
      var text = (tr.textContent || "").toLowerCase();
      var visible = !q || text.indexOf(q) !== -1;
      tr.style.display = visible ? "" : "none";
      if (visible) shown++;
    });

    if (countEl) {
      countEl.textContent = formatFilterCount(shown, rows.length);
    }
    if (clearBtn) {
      clearBtn.hidden = !q;
    }
  }

  function initFilters() {
    document.querySelectorAll(".db-panel").forEach(function (panel) {
      var input = panel.querySelector(".db-search-input");
      var clearBtn = panel.querySelector(".db-search-clear");
      if (!input) return;

      input.addEventListener("input", function () {
        applyFilter(panel);
      });

      if (clearBtn) {
        clearBtn.addEventListener("click", function () {
          input.value = "";
          applyFilter(panel);
          input.focus();
        });
      }

      applyFilter(panel);
    });
  }

  function copyText(text) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      return navigator.clipboard.writeText(text);
    }
    return new Promise(function (resolve, reject) {
      var ta = document.createElement("textarea");
      ta.value = text;
      ta.setAttribute("readonly", "");
      ta.style.position = "fixed";
      ta.style.left = "-9999px";
      document.body.appendChild(ta);
      ta.select();
      try {
        if (document.execCommand("copy")) resolve();
        else reject(new Error("copy failed"));
      } catch (e) {
        reject(e);
      } finally {
        document.body.removeChild(ta);
      }
    });
  }

  function initCopyButtons() {
    document.addEventListener("click", function (e) {
      var btn = e.target.closest(".db-cell-copy");
      if (!btn) return;
      var wrap = btn.closest(".db-cell-long-wrap");
      var span = wrap && wrap.querySelector(".db-cell-truncate");
      var text = span ? span.getAttribute("title") || span.textContent : "";
      if (!text) return;

      copyText(text)
        .then(function () {
          var prev = btn.textContent;
          btn.textContent = copiedLabel;
          btn.classList.add("copied");
          setTimeout(function () {
            btn.textContent = copyLabel;
            btn.classList.remove("copied");
          }, 1600);
        })
        .catch(function () {
          dbAlert(errGeneric);
        });
    });
  }

  function initDeleteButtons() {
    document.querySelectorAll(".db-row-delete").forEach(function (btn) {
      btn.addEventListener("click", function () {
        var table = btn.getAttribute("data-table");
        var id = btn.getAttribute("data-id");
        if (!table || !id) return;

        dbConfirmDelete().then(function (ok) {
          if (!ok) return;
          btn.disabled = true;
          fetch("/api/admin/database/delete", {
            method: "POST",
            headers: {
              "Content-Type": "application/json",
              Accept: "application/json",
            },
            credentials: "same-origin",
            body: JSON.stringify({ table: table, id: id }),
          })
            .then(function (res) {
              return res.text().then(function (text) {
                if (!res.ok) {
                  try {
                    var j = JSON.parse(text);
                    if (j && j.error) throw new Error(String(j.error));
                  } catch (e) {
                    if (e instanceof SyntaxError) throw new Error(errGeneric);
                    throw e;
                  }
                  throw new Error(errGeneric);
                }
                var row = btn.closest("tr");
                var panel = btn.closest(".db-panel");
                if (row) row.parentNode.removeChild(row);
                if (panel) applyFilter(panel);
              });
            })
            .catch(function (e) {
              dbAlert(e && e.message ? e.message : errGeneric).then(function () {
                btn.disabled = false;
              });
            });
        });
      });
    });
  }

  document.addEventListener("DOMContentLoaded", function () {
    initKpi();
    initTableSwitcher();
    initFilters();
    initCopyButtons();
    initDeleteButtons();
  });
})();
