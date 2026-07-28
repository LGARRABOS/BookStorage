(function () {
  var root = document.getElementById("library-edit-root");
  if (!root) return;

  var furnitureId = root.getAttribute("data-furniture-id");
  var stage = document.getElementById("library-edit-stage");
  var listEl = document.getElementById("library-shelf-list");
  var statusEl = document.getElementById("library-edit-status");
  var nextLabelEl = document.getElementById("library-next-label");
  var newCasesEl = document.getElementById("library-new-cases");
  var newBooksEl = document.getElementById("library-new-books");
  var addBtn = document.getElementById("library-add-shelf-btn");

  var i18n = {
    empty: root.getAttribute("data-i18n-empty") || "empty",
    preview: root.getAttribute("data-i18n-preview") || "Preview",
    saved: root.getAttribute("data-i18n-saved") || "Saved",
    cases: root.getAttribute("data-i18n-cases") || "Slots",
    books: root.getAttribute("data-i18n-books") || "Books / slot",
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

  function clampBooks(n) {
    n = parseInt(n, 10);
    if (isNaN(n)) return 8;
    return Math.max(2, Math.min(24, n));
  }

  function normalizeShelf(s) {
    return {
      id: s.id || s.ID || 0,
      label: s.label || s.Label || "?",
      case_count: s.case_count || s.CaseCount || 1,
      books_per_case: clampBooks(
        s.books_per_case || s.BooksPerCase || 8
      ),
      sort_order: s.sort_order || s.SortOrder || 0,
    };
  }

  shelves = shelves.map(normalizeShelf);

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

  var SPINE_COLORS = [
    "#2563eb", "#dc2626", "#ea580c", "#7c3aed", "#16a34a",
    "#0891b2", "#ca8a04", "#db2777", "#4f46e5", "#0d9488",
    "#b45309", "#4338ca", "#be123c", "#047857",
  ];

  function hashStr(s) {
    var h = 0;
    var str = String(s || "");
    for (var i = 0; i < str.length; i++) {
      h = (h * 31 + str.charCodeAt(i)) >>> 0;
    }
    return h;
  }

  function spineColor(title, id) {
    return SPINE_COLORS[hashStr(String(title || "") + "#" + String(id || "")) % SPINE_COLORS.length];
  }

  function cubbyMetrics(booksPerCase) {
    var cap = clampBooks(booksPerCase);
    var slot = cap <= 4 ? 15 : cap <= 8 ? 12 : cap <= 12 ? 10 : cap <= 16 ? 8 : 7;
    return {
      cap: cap,
      slot: slot,
      w: cap * slot + 16,
      h: Math.round(Math.max(110, (cap * slot + 16) * 1.48)),
    };
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
      var m = cubbyMetrics(shelf.books_per_case);
      html += '<div class="library-iso-row">';
      html += '<div class="library-iso-row-label">' + esc(shelf.label) + "</div>";
      html += '<div class="library-iso-cubbies">';
      for (var c = 1; c <= shelf.case_count; c++) {
        var items = placementsInCase(shelf.id, c);
        var filled = items.length > 0;
        html +=
          '<div class="library-iso-cubby' +
          (filled ? " is-filled" : " is-empty") +
          '" style="--cubby-w:' +
          m.w +
          "px;--cubby-h:" +
          m.h +
          'px" aria-label="' +
          esc(caseCode(shelf.label, c)) +
          '">';
        html += '<span class="library-iso-cubby-box">';
        html +=
          '<span class="library-iso-cubby-code">' +
          esc(caseCode(shelf.label, c)) +
          "</span>";
        if (filled) {
          html += '<span class="library-iso-spines">';
          items.slice(0, m.cap).forEach(function (p) {
            var h = hashStr(String(p.title) + String(p.id));
            html +=
              '<span class="library-iso-spine" style="background-color:' +
              spineColor(p.title, p.id) +
              ";width:" +
              Math.max(5, m.slot - 1) +
              "px;height:" +
              (78 + (h % 18)) +
              '%" title="' +
              esc(p.title || "") +
              '"></span>';
          });
          html += "</span>";
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
      html += '<label class="library-shelf-field">' + esc(i18n.cases);
      html +=
        '<input type="number" class="library-cases-input" min="1" max="50" value="' +
        shelf.case_count +
        '"></label>';
      html += '<label class="library-shelf-field">' + esc(i18n.books);
      html +=
        '<input type="number" class="library-books-input" min="2" max="24" value="' +
        shelf.books_per_case +
        '"></label>';
      html +=
        '<button type="button" class="btn btn-danger btn-sm" data-delete="1">✕</button>';
      html += "</li>";
    });
    listEl.innerHTML = html;

    listEl.querySelectorAll(".library-shelf-editor").forEach(function (row) {
      var id = parseInt(row.getAttribute("data-shelf-id"), 10);
      var casesInput = row.querySelector(".library-cases-input");
      var booksInput = row.querySelector(".library-books-input");
      function sync(debounce) {
        var cases = Math.max(1, Math.min(50, parseInt(casesInput.value, 10) || 1));
        var books = clampBooks(booksInput.value);
        casesInput.value = String(cases);
        booksInput.value = String(books);
        updateShelf(id, cases, books, debounce);
      }
      casesInput.addEventListener("input", function () {
        sync(true);
      });
      booksInput.addEventListener("input", function () {
        sync(true);
      });
      row.querySelector("[data-delete]").addEventListener("click", function () {
        if (!confirm(i18n.deleteConfirm)) return;
        deleteShelf(id);
      });
    });

    updateNextLabel();
    renderPreview();
  }

  function updateShelf(shelfId, caseCount, booksPerCase, debounceSave) {
    shelves = shelves.map(function (s) {
      if (s.id === shelfId) {
        return Object.assign({}, s, {
          case_count: caseCount,
          books_per_case: booksPerCase,
        });
      }
      return s;
    });
    renderPreview();
    if (!debounceSave) {
      persistShelf(shelfId, caseCount, booksPerCase);
      return;
    }
    clearTimeout(saveTimer);
    saveTimer = setTimeout(function () {
      persistShelf(shelfId, caseCount, booksPerCase);
    }, 350);
  }

  function persistShelf(shelfId, caseCount, booksPerCase) {
    var body = new URLSearchParams();
    body.set("action", "update_shelf");
    body.set("shelf_id", String(shelfId));
    body.set("case_count", String(caseCount));
    body.set("books_per_case", String(booksPerCase));
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
    var books = clampBooks(newBooksEl && newBooksEl.value ? newBooksEl.value : 8);
    var body = new URLSearchParams();
    body.set("action", "add_shelf");
    body.set("label", label);
    body.set("case_count", String(cases));
    body.set("books_per_case", String(books));
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
        shelves = (data.shelves || []).map(normalizeShelf);
        placements = data.placements || [];
        renderShelfList();
        setStatus(i18n.saved);
      });
  }

  if (addBtn) addBtn.addEventListener("click", addShelf);

  renderShelfList();
})();