(function () {
  var root = document.getElementById("library-grid-root");
  if (!root) return;

  var furnitureId = root.getAttribute("data-furniture-id");
  var stage = document.getElementById("library-iso-stage");
  var panel = document.getElementById("library-case-panel");
  var panelTitle = document.getElementById("library-case-panel-title");
  var panelList = document.getElementById("library-case-panel-list");
  var panelPlaceholder = document.getElementById("library-panel-placeholder");
  var caseAdd = document.getElementById("library-case-add");
  var workQ = document.getElementById("library-work-q");
  var workVol = document.getElementById("library-work-volume");
  var workResults = document.getElementById("library-work-results");
  var volumeWrap = document.getElementById("library-volume-wrap");
  var closeBtn = document.getElementById("library-case-panel-close");
  var countEl = document.getElementById("library-iso-count");
  var zoomIn = document.getElementById("library-zoom-in");
  var zoomOut = document.getElementById("library-zoom-out");

  var i18n = {
    empty: root.getAttribute("data-i18n-empty") || "empty",
    remove: root.getAttribute("data-i18n-remove") || "Remove",
    up: root.getAttribute("data-i18n-up") || "↑",
    down: root.getAttribute("data-i18n-down") || "↓",
    pick: root.getAttribute("data-i18n-pick") || "Pick a cubby",
    panelTitle: root.getAttribute("data-i18n-panel-title") || "Slot",
    volumes: root.getAttribute("data-i18n-volumes") || "volumes",
  };

  var SPINE_COLORS = [
    "#3b82f6", "#ef4444", "#f97316", "#a855f7", "#22c55e",
    "#06b6d4", "#eab308", "#ec4899", "#6366f1", "#14b8a6",
  ];

  var state = {
    shelves: [],
    placements: [],
    active: null,
    kind: "manga",
    zoom: 1,
  };
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

  function caseCode(label, caseNum) {
    return String(label || "?").charAt(0).toUpperCase() + caseNum;
  }

  function spineColor(title, id) {
    var h = 0;
    var s = String(title || "") + String(id || "");
    for (var i = 0; i < s.length; i++) {
      h = (h * 31 + s.charCodeAt(i)) >>> 0;
    }
    return SPINE_COLORS[h % SPINE_COLORS.length];
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

  function maxCases() {
    var n = 0;
    state.shelves.forEach(function (s) {
      if (s.case_count > n) n = s.case_count;
    });
    return n;
  }

  function applyZoom() {
    var world = stage.querySelector(".library-iso-world");
    if (world) {
      world.style.transform = "scale(" + state.zoom + ")";
    }
  }

  function renderGrid() {
    if (!state.shelves.length) {
      stage.innerHTML = '<p class="library-empty">—</p>';
      return;
    }

    var html = '<div class="library-iso-world"><div class="library-iso-grid">';
    state.shelves.forEach(function (shelf) {
      html += '<div class="library-iso-row">';
      html += '<div class="library-iso-row-label">' + esc(shelf.label) + "</div>";
      html += '<div class="library-iso-cubbies">';
      for (var c = 1; c <= shelf.case_count; c++) {
        var items = placementsInCase(shelf.id, c);
        var filled = items.length > 0;
        var selected =
          state.active &&
          state.active.shelfId === shelf.id &&
          state.active.caseNum === c;
        html +=
          '<button type="button" class="library-iso-cubby' +
          (filled ? " is-filled" : "") +
          (selected ? " is-selected" : "") +
          '" data-shelf-id="' +
          shelf.id +
          '" data-case-num="' +
          c +
          '" data-label="' +
          esc(shelf.label) +
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
          items.slice(0, 8).forEach(function (p) {
            var style = "background:" + spineColor(p.title, p.id) + ";";
            if (p.image_path) {
              style =
                "background-image:linear-gradient(90deg,rgba(0,0,0,.25),transparent 40%),url('" +
                esc(p.image_path) +
                "');background-size:cover;background-position:center;";
            }
            html +=
              '<span class="library-iso-spine' +
              (p.image_path ? " has-cover" : "") +
              '" style="' +
              style +
              '" title="' +
              esc(p.title) +
              '"></span>';
          });
          html += "</span>";
        } else {
          html +=
            '<span class="library-iso-empty-hint">' + esc(i18n.empty) + "</span>";
        }
        html += "</span></button>";
      }
      html += "</div></div>";
    });
    html += "</div>";

    var cols = maxCases();
    if (cols > 0) {
      html += '<div class="library-iso-axis" aria-hidden="true">';
      for (var i = 1; i <= cols; i++) {
        html += "<span>" + i + "</span>";
      }
      html += "</div>";
    }
    html += "</div>";

    stage.innerHTML = html;
    applyZoom();

    stage.querySelectorAll(".library-iso-cubby").forEach(function (btn) {
      btn.addEventListener("click", function () {
        openCase(
          parseInt(btn.getAttribute("data-shelf-id"), 10),
          parseInt(btn.getAttribute("data-case-num"), 10),
          btn.getAttribute("data-label")
        );
      });
    });

    if (countEl) {
      countEl.textContent = String(state.placements.length);
    }
  }

  function setPanelOpen(open) {
    if (panelPlaceholder) panelPlaceholder.hidden = open;
    if (panelList) panelList.hidden = !open;
    if (caseAdd) caseAdd.hidden = !open;
    if (closeBtn) {
      closeBtn.hidden = !open;
    }
    if (panel) {
      panel.classList.toggle("is-docked-closed", false);
    }
  }

  function openCase(shelfId, caseNum, label) {
    state.active = { shelfId: shelfId, caseNum: caseNum, label: label };
    panelTitle.textContent = caseCode(label, caseNum);
    setPanelOpen(true);
    renderPanelList();
    renderGrid();
    workQ.value = "";
    workResults.innerHTML = "";
    if (workVol) workVol.value = "1";
    setKind(state.kind);
    if (window.matchMedia("(max-width: 900px)").matches && panel) {
      panel.scrollIntoView({ behavior: "smooth", block: "nearest" });
    }
  }

  function closePanel() {
    state.active = null;
    panelTitle.textContent = i18n.panelTitle;
    setPanelOpen(false);
    if (panelList) panelList.innerHTML = "";
    renderGrid();
  }

  function renderPanelList() {
    if (!state.active || !panelList) return;
    var items = placementsInCase(state.active.shelfId, state.active.caseNum);
    if (!items.length) {
      panelList.innerHTML =
        '<p class="library-empty">' + esc(i18n.empty) + "</p>";
      return;
    }
    var html = "";
    items.forEach(function (p) {
      html += '<div class="library-placement-item">';
      if (p.image_path) {
        html += '<img src="' + esc(p.image_path) + '" alt="">';
      } else {
        html +=
          '<span style="width:36px;height:52px;border-radius:4px;background:' +
          spineColor(p.title, p.id) +
          '"></span>';
      }
      html += '<div class="meta"><strong>' + esc(p.title) + "</strong>";
      html +=
        '<div class="sub"><span class="library-code">' +
        esc(p.code) +
        "</span> · " +
        esc(p.media_kind) +
        (p.volume ? " T" + p.volume : "") +
        "</div></div>";
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
            var volume =
              parseInt(workVol && workVol.value ? workVol.value : "1", 10) || 1;
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

  if (zoomIn) {
    zoomIn.addEventListener("click", function () {
      state.zoom = Math.min(1.6, state.zoom + 0.1);
      applyZoom();
    });
  }
  if (zoomOut) {
    zoomOut.addEventListener("click", function () {
      state.zoom = Math.max(0.55, state.zoom - 0.1);
      applyZoom();
    });
  }

  setPanelOpen(false);
  load();
})();
