(function () {
  var root = document.getElementById("library-grid-root");
  if (!root) return;

  var furnitureId = root.getAttribute("data-furniture-id");
  var panel = document.getElementById("library-case-panel");
  var backdrop = document.getElementById("library-case-backdrop");
  var panelTitle = document.getElementById("library-case-panel-title");
  var panelList = document.getElementById("library-case-panel-list");
  var workQ = document.getElementById("library-work-q");
  var workVol = document.getElementById("library-work-volume");
  var workResults = document.getElementById("library-work-results");
  var volumeWrap = document.getElementById("library-volume-wrap");
  var closeBtn = document.getElementById("library-case-panel-close");

  var i18n = {
    empty: root.getAttribute("data-i18n-empty") || "Empty",
    add: root.getAttribute("data-i18n-add") || "Add",
    remove: root.getAttribute("data-i18n-remove") || "Remove",
    up: root.getAttribute("data-i18n-up") || "Up",
    down: root.getAttribute("data-i18n-down") || "Down",
  };

  var state = { shelves: [], placements: [], active: null, kind: "manga" };
  var searchTimer = null;

  function esc(s) {
    return String(s == null ? "" : s)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  function codeFor(label, caseNum, position) {
    return String(label || "?").charAt(0).toUpperCase() + caseNum + "-" + position;
  }

  function placementsInCase(shelfId, caseNum) {
    return state.placements
      .filter(function (p) {
        return p.shelf_id === shelfId && p.case_num === caseNum;
      })
      .sort(function (a, b) {
        return a.position - b.position;
      });
  }

  function renderGrid() {
    if (!state.shelves.length) {
      root.innerHTML = '<p class="library-empty">—</p>';
      return;
    }
    var html = '<div class="library-grid">';
    state.shelves.forEach(function (shelf) {
      html += '<div class="library-shelf-row-grid">';
      html += '<div class="library-shelf-row-label">' + esc(shelf.label) + "</div>";
      html += '<div class="library-cases">';
      for (var c = 1; c <= shelf.case_count; c++) {
        var items = placementsInCase(shelf.id, c);
        var filled = items.length > 0;
        html +=
          '<button type="button" class="library-case' +
          (filled ? " is-filled" : "") +
          '" data-shelf-id="' +
          shelf.id +
          '" data-case-num="' +
          c +
          '" data-label="' +
          esc(shelf.label) +
          '">';
        html += '<span class="library-case-code">' + esc(codeFor(shelf.label, c, 1).replace(/-\d+$/, "")) + "</span>";
        html +=
          '<span class="library-case-count">' +
          (filled ? items.length : i18n.empty) +
          "</span>";
        if (filled) {
          html += '<div class="library-case-thumbs">';
          for (var i = 0; i < Math.min(items.length, 4); i++) {
            html += "<span></span>";
          }
          html += "</div>";
        }
        html += "</button>";
      }
      html += "</div></div>";
    });
    html += "</div>";
    root.innerHTML = html;
    root.querySelectorAll(".library-case").forEach(function (btn) {
      btn.addEventListener("click", function () {
        openCase(
          parseInt(btn.getAttribute("data-shelf-id"), 10),
          parseInt(btn.getAttribute("data-case-num"), 10),
          btn.getAttribute("data-label")
        );
      });
    });
  }

  function openCase(shelfId, caseNum, label) {
    state.active = { shelfId: shelfId, caseNum: caseNum, label: label };
    panelTitle.textContent = codeFor(label, caseNum, 1).replace(/-\d+$/, "");
    renderPanelList();
    panel.hidden = false;
    panel.setAttribute("aria-hidden", "false");
    backdrop.hidden = false;
    workQ.value = "";
    workResults.innerHTML = "";
    if (workVol) workVol.value = "1";
    setKind(state.kind);
  }

  function closePanel() {
    panel.hidden = true;
    panel.setAttribute("aria-hidden", "true");
    backdrop.hidden = true;
    state.active = null;
  }

  function renderPanelList() {
    if (!state.active) return;
    var items = placementsInCase(state.active.shelfId, state.active.caseNum);
    if (!items.length) {
      panelList.innerHTML = '<p class="library-empty">' + esc(i18n.empty) + "</p>";
      return;
    }
    var html = "";
    items.forEach(function (p) {
      html += '<div class="library-placement-item">';
      if (p.image_path) {
        html += '<img src="' + esc(p.image_path) + '" alt="">';
      } else {
        html += "<span></span>";
      }
      html += '<div class="meta"><strong>' + esc(p.title) + "</strong>";
      html +=
        '<span class="library-code">' +
        esc(p.code) +
        "</span> · " +
        esc(p.media_kind) +
        (p.volume ? " T" + p.volume : "") +
        "</div>";
      html += '<div class="library-placement-actions">';
      html +=
        '<button type="button" class="btn btn-secondary btn-sm" data-move="up" data-id="' +
        p.id +
        '">' +
        esc(i18n.up) +
        "</button>";
      html +=
        '<button type="button" class="btn btn-secondary btn-sm" data-move="down" data-id="' +
        p.id +
        '">' +
        esc(i18n.down) +
        "</button>";
      html +=
        '<button type="button" class="btn btn-danger btn-sm" data-remove="' +
        p.id +
        '">' +
        esc(i18n.remove) +
        "</button>";
      html += "</div></div>";
    });
    panelList.innerHTML = html;
    panelList.querySelectorAll("[data-move]").forEach(function (btn) {
      btn.addEventListener("click", function () {
        patchPlacement(parseInt(btn.getAttribute("data-id"), 10), {
          move: btn.getAttribute("data-move"),
        });
      });
    });
    panelList.querySelectorAll("[data-remove]").forEach(function (btn) {
      btn.addEventListener("click", function () {
        deletePlacement(parseInt(btn.getAttribute("data-remove"), 10));
      });
    });
  }

  function load() {
    return fetch("/api/library/furniture/" + encodeURIComponent(furnitureId), {
      headers: { "X-Requested-With": "XMLHttpRequest" },
    })
      .then(function (r) {
        return r.json();
      })
      .then(function (data) {
        state.shelves = data.shelves || [];
        state.placements = data.placements || [];
        renderGrid();
        if (state.active) renderPanelList();
      });
  }

  function patchPlacement(id, body) {
    fetch("/api/library/placements/" + id, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Requested-With": "XMLHttpRequest",
      },
      body: JSON.stringify(body),
    }).then(function () {
      return load();
    });
  }

  function deletePlacement(id) {
    fetch("/api/library/placements/" + id + "/delete", {
      method: "POST",
      headers: { "X-Requested-With": "XMLHttpRequest" },
    }).then(function () {
      return load();
    });
  }

  function setKind(kind) {
    state.kind = kind;
    document.querySelectorAll(".library-case-add-tabs [data-kind]").forEach(function (b) {
      b.classList.toggle("is-active", b.getAttribute("data-kind") === kind);
    });
    if (volumeWrap) volumeWrap.style.display = kind === "manga" ? "" : "none";
  }

  function searchWorks(q) {
    if (!q) {
      workResults.innerHTML = "";
      return;
    }
    fetch(
      "/api/library/works?kind=" +
        encodeURIComponent(state.kind) +
        "&q=" +
        encodeURIComponent(q),
      { headers: { "X-Requested-With": "XMLHttpRequest" } }
    )
      .then(function (r) {
        return r.json();
      })
      .then(function (data) {
        var results = data.results || [];
        if (!results.length) {
          workResults.innerHTML = "";
          return;
        }
        var html = "";
        results.forEach(function (it) {
          html +=
            '<li><button type="button" data-id="' +
            it.id +
            '" data-volume="' +
            (it.volume || 1) +
            '">' +
            esc(it.title) +
            (it.volume ? " · T" + it.volume : "") +
            "</button></li>";
        });
        workResults.innerHTML = html;
        workResults.querySelectorAll("button").forEach(function (btn) {
          btn.addEventListener("click", function () {
            if (!state.active) return;
            var volume = parseInt(workVol && workVol.value ? workVol.value : "1", 10) || 1;
            if (state.kind === "bd") {
              volume = parseInt(btn.getAttribute("data-volume"), 10) || 1;
            }
            fetch("/api/library/placements", {
              method: "POST",
              headers: {
                "Content-Type": "application/json",
                "X-Requested-With": "XMLHttpRequest",
              },
              body: JSON.stringify({
                shelf_id: state.active.shelfId,
                case_num: state.active.caseNum,
                media_kind: state.kind,
                work_id: parseInt(btn.getAttribute("data-id"), 10),
                volume: volume,
              }),
            }).then(function (r) {
              return r.json().then(function (data) {
                if (!r.ok || (data && data.ok === false)) {
                  alert(data && data.error ? data.error : "Error");
                  return;
                }
                workQ.value = "";
                workResults.innerHTML = "";
                return load();
              });
            });
          });
        });
      });
  }

  document.querySelectorAll(".library-case-add-tabs [data-kind]").forEach(function (btn) {
    btn.addEventListener("click", function () {
      setKind(btn.getAttribute("data-kind"));
      if (workQ.value) searchWorks(workQ.value.trim());
    });
  });

  if (workQ) {
    workQ.addEventListener("input", function () {
      clearTimeout(searchTimer);
      var q = workQ.value.trim();
      searchTimer = setTimeout(function () {
        searchWorks(q);
      }, 220);
    });
  }

  if (closeBtn) closeBtn.addEventListener("click", closePanel);
  if (backdrop) backdrop.addEventListener("click", closePanel);

  load();
})();
