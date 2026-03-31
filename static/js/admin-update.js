(() => {
  const $ = (id) => document.getElementById(id);

  async function confirmUpdate() {
    const msg = window.__UPDATE_CONFIRM__ || "Confirmer ?";
    if (typeof window.showConfirm === "function") {
      return await window.showConfirm(msg);
    }
    return window.confirm(msg);
  }

  function setBusy(busy) {
    $("btn-update-latest").disabled = busy;
    $("btn-update-latest-major").disabled = busy;
  }

  function showStatus(text, ok) {
    const el = $("update-status");
    el.textContent = text;
    el.style.display = "block";
    el.style.color = ok === true ? "var(--success, #16a34a)" : ok === false ? "#dc2626" : "var(--text-secondary)";
  }

  function showOutput(text) {
    const el = $("update-output");
    if (!text) {
      el.style.display = "none";
      return;
    }
    el.textContent = text;
    el.style.display = "block";
  }

  async function run(endpoint) {
    if (!(await confirmUpdate())) return;
    setBusy(true);
    showStatus(window.__UPDATE_IN_PROGRESS__ || "En cours...", null);
    showOutput("");
    try {
      const res = await fetch(endpoint, {
        method: "POST",
        headers: { "Accept": "application/json" },
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok || !data.ok) {
        showStatus(window.__UPDATE_ERROR__ || "Erreur", false);
      } else {
        showStatus(window.__UPDATE_SUCCESS__ || "OK", true);
      }
      showOutput(data.output || "");
    } catch (e) {
      showStatus(`${window.__UPDATE_ERROR__ || "Erreur"}: ${e?.message || e}`, false);
    } finally {
      setBusy(false);
    }
  }

  document.addEventListener("DOMContentLoaded", () => {
    $("btn-update-latest").addEventListener("click", () => run("/api/admin/update/latest"));
    $("btn-update-latest-major").addEventListener("click", () => run("/api/admin/update/latest-major"));
  });
})();

