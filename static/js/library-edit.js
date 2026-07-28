(function () {
  var root = document.getElementById("library-edit-root");
  if (!root) return;

  var furnitureId = root.getAttribute("data-furniture-id");
  var stage = document.getElementById("library-edit-stage");
  var listEl = document.getElementById("library-shelf-list");
  var statusEl = document.getElementById("library-edit-status");
  var nextLabelEl = document.getElementById("library-next-label");
  var newCasesEl = document.getElementById("library-new-cases");
  var addBtn = document.getElementById("library-add-shelf-btn");

  var i18n = {
    empty: root.getAttribute("data-i18n-empty") || "empty",
    preview: root.getAttribute("data-i18n-preview") || "Preview",
    saved: root.getAttribute("data-i18n-saved") || "Saved",
    cases: root.getAttribute("data-i18n-cases") || "Slots",
    deleteConfirm:
      root.getAttribute("data-i18n-delete-confirm") || "Delete this shelf?",
  };

  var shelves = [];
  var placements = [];
  try {
    shelves = JSON.parse(root.getAttribute("data-shelves") || "[]") || [];
  } catch (e) {
    shelves = [];
  }
  try {
    placements = JSON.parse(root.getAttribute("data-placements") || "[]") || [];
  } catch (e) {
    placements = [];
  }

  // Normalize Go json tags (ID vs id) — struct uses capital fields without json tags.
  shelves = shelves.map(function (s) {
    return {
      id: s.id || s.ID || 0,
      label: s.label || s.Label || "?",
      case_count: s.case_count || s.CaseCount || 1,
      sort_order: s.sort_order || s.SortOrder || 0,
    };
  });

  var saveTimer = null;
  var statusTimer = null;

  function esc(s) {
    return String(s == null ? "" : s)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  function caseCode(label, caseNum) {
    return String(label || "?").charAt(0).toUpperCase() + caseNum;
  }

  function setStatus(msg) {
    if (!statusEl) return;
    statusEl.textContent = msg || "";
    clearTimeout(statusTimer);
    if (msg) {
      statusTimer = setTimeout(function () {
        statusEl.textContent = "";
      }, 1800);
    }
  }

  function nextLabel() {
    var used = {};
    shelves.forEach(function (s) {
      used[String(s.label || "").toUpperCase()] = true;
    });
    for (var i = 0; i < 26; i++) {
      var L = String.fromCharCode(65 + i);
      if (!used[L]) return L;
    }
    return "Z";
  }

  function updateNextLabel() {
    if (nextLabelEl) nextLabelEl.textContent = nextLabel();
  }

  function placementsInCase(shelfId, caseNum) {
    return placements.filter(function (p) {
      return p.shelf_id === shelfId && p.case_num === caseNum;
    });
  }

  function renderPreview() {
    if (!stage) return;
    if (!shelves.length) {
      stage.innerHTML = '<p class="library-empty">' + esc(i18n.empty) + "</p>";
      return;
    }
    var html = '<div class="library-iso-world"><div class="library-iso-grid">';
    shelves.forEach(function (shelf) {
      html += '<div class="library-iso-row">';
      html += '<div class="library-iso-row-label">' + esc(shelf.label) + "</div>";
      html += '<div class="library-iso-cubbies">';
      for (var c = 1; c <= shelf.case_count; c++) {
        var items = placementsInCase(shelf.id, c);
        var filled = items.length > 0;
        html +=
          '<div class="library-iso-cubby' +
          (filled ? " is-filled" : "") +
          '" aria-label="' +
          esc(caseCode(shelf.label, c)) +
          '">';
        html += '<span class="library-iso-cubby-box">';
        html +=
          '<span class="library-iso-cubby-code">' +
          esc(caseCode(shelf.label, c)) +
          "</span>";
        if (filled) {
          html += '<span class="library-iso-spines">';
          for (var i = 0; i < Math.min(items.length, 8); i++) {
            html += '<span class="library-iso-spine" style="background:#3b82f6"></span>';
          }
          html += "</span>";
        } else {
          html +=
            '<span class="library-iso-empty-hint">' + esc(i18n.empty) + "</span>";
        }
        html += "</span></div>";
      }
      html += "</div></div>";
    });
    html += "</div></div>";
    stage.innerHTML = html;
  }

  function renderShelfList() {
    if (!listEl) return;
    if (!shelves.length) {
      listEl.innerHTML = '<li class="library-empty">' + esc(i18n.empty) + "</li>";
      updateNextLabel();
      renderPreview();
      return;
    }
    var html = "";
    shelves.forEach(function (shelf) {
      html += '<li class="library-shelf-editor" data-shelf-id="' + shelf.id + '">';
      html += '<span class="library-shelf-label">' + esc(shelf.label) + "</span>";
      html += '<div class="library-cases-stepper">';
      html +=
        '<button type="button" class="btn btn-secondary btn-sm" data-delta="-1" aria-label="-">−</button>';
      html +=
        '<input type="number" class="library-cases-input" min="1" max="50" value="' +
        shelf.case_count +
        '" aria-label="' +
        esc(i18n.cases) +
        '">';
      html +=
        '<button type="button" class="btn btn-secondary btn-sm" data-delta="1" aria-label="+">+</button>';
      html += "</div>";
      html +=
        '<button type="button" class="btn btn-danger btn-sm" data-delete="1">' +
        "✕" +
        "</button>";
      html += "</li>";
    });
    listEl.innerHTML = html;

    listEl.querySelectorAll(".library-shelf-editor").forEach(function (row) {
      var id = parseInt(row.getAttribute("data-shelf-id"), 10);
      var input = row.querySelector(".library-cases-input");
      row.querySelectorAll("[data-delta]").forEach(function (btn) {
        btn.addEventListener("click", function () {
          var d = parseInt(btn.getAttribute("data-delta"), 10);
          var v = parseInt(input.value, 10) || 1;
          v = Math.max(1, Math.min(50, v + d));
          input.value = String(v);
          setCaseCount(id, v, true);
        });
      });
      input.addEventListener("input", function () {
        var v = parseInt(input.value, 10) || 1;
        v = Math.max(1, Math.min(50, v));
        setCaseCount(id, v, true);
      });
      row.querySelector("[data-delete]").addEventListener("click", function () {
        if (!confirm(i18n.deleteConfirm)) return;
        deleteShelf(id);
      });
    });

    updateNextLabel();
    renderPreview();
  }

  function setCaseCount(shelfId, count, debounceSave) {
    shelves = shelves.map(function (s) {
      if (s.id === shelfId) {
        return Object.assign({}, s, { case_count: count });
      }
      return s;
    });
    renderPreview();
    if (!debounceSave) {
      persistCaseCount(shelfId, count);
      return;
    }
    clearTimeout(saveTimer);
    saveTimer = setTimeout(function () {
      persistCaseCount(shelfId, count);
    }, 350);
  }

  function persistCaseCount(shelfId, count) {
    var body = new URLSearchParams();
    body.set("action", "update_shelf");
    body.set("shelf_id", String(shelfId));
    body.set("case_count", String(count));
    fetch("/library/furniture/" + encodeURIComponent(furnitureId) + "/edit", {
      method: "POST",
      headers: {
        "Content-Type": "application/x-www-form-urlencoded",
        "X-Requested-With": "XMLHttpRequest",
      },
      body: body.toString(),
      redirect: "manual",
    }).then(function () {
      setStatus(i18n.saved);
    });
  }

  function deleteShelf(shelfId) {
    var body = new URLSearchParams();
    body.set("action", "delete_shelf");
    body.set("shelf_id", String(shelfId));
    fetch("/library/furniture/" + encodeURIComponent(furnitureId) + "/edit", {
      method: "POST",
      headers: {
        "Content-Type": "application/x-www-form-urlencoded",
        "X-Requested-With": "XMLHttpRequest",
      },
      body: body.toString(),
      redirect: "manual",
    }).then(function () {
      shelves = shelves.filter(function (s) {
        return s.id !== shelfId;
      });
      placements = placements.filter(function (p) {
        return p.shelf_id !== shelfId;
      });
      renderShelfList();
      setStatus(i18n.saved);
    });
  }

  function addShelf() {
    var label = nextLabel();
    var cases = parseInt(newCasesEl && newCasesEl.value ? newCasesEl.value : "10", 10) || 10;
    cases = Math.max(1, Math.min(50, cases));
    var body = new URLSearchParams();
    body.set("action", "add_shelf");
    body.set("label", label);
    body.set("case_count", String(cases));
    fetch("/library/furniture/" + encodeURIComponent(furnitureId) + "/edit", {
      method: "POST",
      headers: {
        "Content-Type": "application/x-www-form-urlencoded",
        "X-Requested-With": "XMLHttpRequest",
      },
      body: body.toString(),
      redirect: "manual",
    })
      .then(function () {
        return fetch("/api/library/furniture/" + encodeURIComponent(furnitureId), {
          headers: { "X-Requested-With": "XMLHttpRequest" },
        });
      })
      .then(function (r) {
        return r.json();
      })
      .then(function (data) {
        shelves = (data.shelves || []).map(function (s) {
          return {
            id: s.id,
            label: s.label,
            case_count: s.case_count,
            sort_order: s.sort_order || 0,
          };
        });
        placements = data.placements || [];
        renderShelfList();
        setStatus(i18n.saved);
      });
  }

  if (addBtn) addBtn.addEventListener("click", addShelf);

  // Use a clearer confirm for delete — pull from data if present later
  renderShelfList();
})();
