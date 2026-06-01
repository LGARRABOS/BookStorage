(function () {
  "use strict";

  var i18n = document.getElementById("admin-db-i18n");
  if (!i18n) return;

  var COL_STORAGE_PREFIX = "bookstorage-admin-db-cols-v1:";

  function dbAttr(k, fb) {
    return i18n.getAttribute("data-" + k) || fb;
  }

  var confirmMsg = dbAttr("confirm-delete", "Delete this row?");
  var errGeneric = dbAttr("delete-error", "Delete failed.");
  var filterCountTpl = dbAttr("filter-count", "{shown} / {total}");
  var copyLabel = dbAttr("copy", "Copy");
  var copiedLabel = dbAttr("copied", "Copied");
  var kpiTruncatedNone = dbAttr("kpi-truncated-none", "None");
  var rowDetailTitleTpl = dbAttr("row-detail-title", "Row detail");
  var rowDetailClose = dbAttr("row-detail-close", "Close");

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
    var panels = document.querySelectorAll(".db-panel");
    var pills = document.querySelectorAll(".db-table-pill");
    var found = false;

    panels.forEach(function (panel) {
      var match = panel.getAttribute("data-table") === tableName;
      panel.hidden = !match;
      if (match) found = true;
    });

    pills.forEach(function (pill) {
      var active = pill.getAttribute("data-table") === tableName;
      pill.classList.toggle("active", active);
      pill.setAttribute("aria-selected", active ? "true" : "false");
    });

    if (found) {
      var hash = "#" + slugTable(tableName);
      if (window.location.hash !== hash) {
        history.replaceState(null, "", hash);
      }
      var panel = document.getElementById("db-panel-" + tableName);
      if (panel) {
        var zone = panel.querySelector(".db-table-scroll-zone");
        if (zone) updateScrollShadows(zone);
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

    if (countEl) countEl.textContent = formatFilterCount(shown, rows.length);
    if (clearBtn) clearBtn.hidden = !q;
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

  function updateScrollShadows(zone) {
    if (!zone) return;
    var max = zone.scrollWidth - zone.clientWidth;
    var sl = zone.scrollLeft;
    zone.classList.toggle("db-scroll-left", sl > 4);
    zone.classList.toggle("db-scroll-right", max > 4 && sl < max - 4);
  }

  function setColumnWidth(table, colIndex, widthPx) {
    var w = Math.max(48, Math.round(widthPx));
    var cols = table.querySelectorAll("colgroup col");
    if (cols[colIndex]) cols[colIndex].style.width = w + "px";
    table.querySelectorAll("tr").forEach(function (tr) {
      var cell = tr.children[colIndex];
      if (!cell) return;
      cell.style.width = w + "px";
      cell.style.minWidth = w + "px";
      cell.style.maxWidth = w + "px";
    });
  }

  function clearColumnWidths(table) {
    table.querySelectorAll("colgroup col").forEach(function (col) {
      col.style.width = "";
    });
    table.querySelectorAll("th, td").forEach(function (cell) {
      cell.style.width = "";
      cell.style.minWidth = "";
      cell.style.maxWidth = "";
    });
  }

  function loadColumnWidths(table) {
    var tableName = table.getAttribute("data-db-table");
    if (!tableName) return;
    try {
      var raw = localStorage.getItem(COL_STORAGE_PREFIX + tableName);
      if (!raw) return;
      var widths = JSON.parse(raw);
      if (!Array.isArray(widths)) return;
      widths.forEach(function (w, i) {
        if (typeof w === "number" && w > 0) setColumnWidth(table, i, w);
      });
    } catch (e) {
      /* ignore */
    }
  }

  function saveColumnWidths(table) {
    var tableName = table.getAttribute("data-db-table");
    if (!tableName) return;
    var header = table.querySelector("thead tr");
    if (!header) return;
    var widths = [];
    for (var i = 0; i < header.children.length; i++) {
      var cell = header.children[i];
      var w = cell.offsetWidth;
      widths.push(w > 0 ? w : null);
    }
    try {
      localStorage.setItem(COL_STORAGE_PREFIX + tableName, JSON.stringify(widths));
    } catch (e) {
      /* ignore */
    }
  }

  function autoFitColumn(table, colIndex) {
    var max = 80;
    table.querySelectorAll("tr").forEach(function (tr) {
      var cell = tr.children[colIndex];
      if (!cell || cell.classList.contains("db-actions-col")) return;
      cell.style.width = "auto";
      cell.style.maxWidth = "none";
      var need = cell.scrollWidth + 16;
      if (need > max) max = need;
    });
    setColumnWidth(table, colIndex, Math.min(max, 480));
    saveColumnWidths(table);
  }

  function initColumnResize(table) {
    var header = table.querySelector("thead tr");
    if (!header) return;

    header.querySelectorAll(".db-col-resize").forEach(function (handle) {
      var th = handle.closest("th");
      if (!th) return;
      var colIndex = Array.prototype.indexOf.call(header.children, th);

      handle.addEventListener("mousedown", function (e) {
        e.preventDefault();
        e.stopPropagation();
        var startX = e.clientX;
        var startW = th.offsetWidth;
        handle.classList.add("db-resizing");
        document.body.style.cursor = "col-resize";
        document.body.style.userSelect = "none";

        function onMove(ev) {
          setColumnWidth(table, colIndex, startW + (ev.clientX - startX));
        }

        function onUp() {
          handle.classList.remove("db-resizing");
          document.body.style.cursor = "";
          document.body.style.userSelect = "";
          document.removeEventListener("mousemove", onMove);
          document.removeEventListener("mouseup", onUp);
          saveColumnWidths(table);
          var zone = table.closest(".db-table-scroll-zone");
          if (zone) updateScrollShadows(zone);
        }

        document.addEventListener("mousemove", onMove);
        document.addEventListener("mouseup", onUp);
      });

      th.querySelector(".db-th-inner").addEventListener("dblclick", function (e) {
        e.preventDefault();
        autoFitColumn(table, colIndex);
        var zone = table.closest(".db-table-scroll-zone");
        if (zone) updateScrollShadows(zone);
      });
    });
  }

  function initScrollZones() {
    document.querySelectorAll(".db-table-scroll-zone").forEach(function (zone) {
      var table = zone.querySelector("table[data-db-table]");
      if (!table) return;

      loadColumnWidths(table);
      initColumnResize(table);
      updateScrollShadows(zone);

      zone.addEventListener("scroll", function () {
        updateScrollShadows(zone);
      });

      zone.addEventListener("wheel", function (e) {
        if (Math.abs(e.deltaX) > Math.abs(e.deltaY)) return;
        if (e.shiftKey && zone.scrollWidth > zone.clientWidth) {
          zone.scrollLeft += e.deltaY;
          e.preventDefault();
        }
      }, { passive: false });

      var panel = zone.closest(".db-panel");
      if (!panel) return;

      panel.querySelectorAll(".db-scroll-btn").forEach(function (btn) {
        btn.addEventListener("click", function () {
          var dir = btn.getAttribute("data-dir") === "right" ? 1 : -1;
          var step = Math.max(280, zone.clientWidth * 0.72);
          zone.scrollBy({ left: dir * step, behavior: "smooth" });
          setTimeout(function () {
            updateScrollShadows(zone);
          }, 320);
        });
      });

      setTimeout(function () {
        updateScrollShadows(zone);
      }, 0);

      var resetBtn = panel.querySelector(".db-reset-cols");
      if (resetBtn) {
        resetBtn.addEventListener("click", function () {
          var tableName = table.getAttribute("data-db-table");
          if (tableName) {
            try {
              localStorage.removeItem(COL_STORAGE_PREFIX + tableName);
            } catch (e) {
              /* ignore */
            }
          }
          clearColumnWidths(table);
          updateScrollShadows(zone);
        });
      }
    });

    window.addEventListener("resize", function () {
      document.querySelectorAll(".db-table-scroll-zone").forEach(updateScrollShadows);
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
      e.stopPropagation();
      var td = btn.closest("td");
      var text = (td && td.getAttribute("data-full")) || "";
      if (!text) {
        var span = btn.closest(".db-cell-long-block") && btn.closest(".db-cell-long-block").querySelector(".db-cell-truncate");
        text = span ? span.textContent : "";
      }
      if (!text) return;

      copyText(text)
        .then(function () {
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

  function initRowDetail() {
    var overlay = document.getElementById("db-row-detail");
    var titleEl = document.getElementById("db-row-detail-title");
    var dl = document.getElementById("db-row-detail-dl");
    var closeBtn = document.getElementById("db-row-detail-close");
    if (!overlay || !dl || !titleEl || !closeBtn) return;

    closeBtn.textContent = rowDetailClose;

    function closeDetail() {
      overlay.hidden = true;
    }

    function openDetail(tr) {
      var table = tr.closest("table");
      var tableName = table && table.getAttribute("data-db-table");
      titleEl.textContent = rowDetailTitleTpl.replace("{table}", tableName || "").replace("{id}", "");
      dl.innerHTML = "";
      tr.querySelectorAll("td[data-col]").forEach(function (td) {
        if (td.classList.contains("db-actions-col")) return;
        var col = td.getAttribute("data-col") || "";
        var full = td.getAttribute("data-full") || "";
        if (!full) {
          var badge = td.querySelector(".badge");
          full = badge ? badge.textContent : (td.textContent || "").trim();
        }
        var dt = document.createElement("dt");
        dt.textContent = col;
        var dd = document.createElement("dd");
        dd.textContent = full || "—";
        dl.appendChild(dt);
        dl.appendChild(dd);
        if (col === "id" || col === "ID") {
          titleEl.textContent = rowDetailTitleTpl
            .replace("{table}", tableName || "")
            .replace("{id}", full ? " #" + full : "");
        }
      });
      overlay.hidden = false;
      closeBtn.focus();
    }

    closeBtn.addEventListener("click", closeDetail);
    overlay.addEventListener("click", function (e) {
      if (e.target === overlay) closeDetail();
    });
    document.addEventListener("keydown", function (e) {
      if (e.key === "Escape" && !overlay.hidden) closeDetail();
    });

    document.querySelectorAll(".db-data-table tbody").forEach(function (tbody) {
      tbody.addEventListener("click", function (e) {
        if (e.target.closest(".db-row-delete, .db-cell-copy")) return;
        var tr = e.target.closest("tr.db-data-row");
        if (tr && tr.style.display !== "none") openDetail(tr);
      });
      tbody.addEventListener("keydown", function (e) {
        if (e.key !== "Enter") return;
        var tr = e.target.closest("tr.db-data-row");
        if (tr) {
          e.preventDefault();
          openDetail(tr);
        }
      });
    });
  }

  function initDeleteButtons() {
    document.querySelectorAll(".db-row-delete").forEach(function (btn) {
      btn.addEventListener("click", function (e) {
        e.stopPropagation();
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
                  } catch (err) {
                    if (err instanceof SyntaxError) throw new Error(errGeneric);
                    throw err;
                  }
                  throw new Error(errGeneric);
                }
                var row = btn.closest("tr");
                var panel = btn.closest(".db-panel");
                if (row) row.parentNode.removeChild(row);
                if (panel) applyFilter(panel);
              });
            })
            .catch(function (err) {
              dbAlert(err && err.message ? err.message : errGeneric).then(function () {
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
    initScrollZones();
    initCopyButtons();
    initRowDetail();
    initDeleteButtons();
  });
})();
