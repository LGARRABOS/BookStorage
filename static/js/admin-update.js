(() => {
  const $ = (id) => document.getElementById(id);

  async function confirmUpdate() {
    const msg = window.__UPDATE_CONFIRM__ || "Confirmer ?";
    if (typeof window.showConfirm === "function") {
      return await window.showConfirm(msg, {
        title: window.__UPDATE_CONFIRM_TITLE__ || "Confirmer",
        okLabel: window.__UPDATE_CONFIRM_OK__ || "OK",
        cancelLabel: window.__UPDATE_CONFIRM_CANCEL__ || "Annuler",
        okClass: "btn-primary",
      });
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

  async function fetchStatus() {
    const res = await fetch("/api/admin/update/status", {
      method: "GET",
      headers: { "Accept": "application/json" },
    });
    return await res.json().catch(() => ({}));
  }

  function summarize(data) {
    const extra = data && data.tag ? ` (${data.tag})` : "";
    return { extra, msg: data?.message || "", out: data?.output || "", cmd: data?.command || "" };
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
      if (data && data.message === "already_up_to_date") {
        const extra = data.tag ? ` (${data.tag})` : "";
        showStatus((window.__UPDATE_ALREADY__ || "Déjà à jour.") + extra, true);
        showOutput("");
        return;
      }
      if (!res.ok || !data.ok) {
        const extra = data && (data.message || data.tag) ? ` (${[data.message, data.tag].filter(Boolean).join(" / ")})` : "";
        showStatus((window.__UPDATE_ERROR__ || "Erreur") + extra, false);
        showOutput(data.output || "");
        return;
      } else {
        // Update runs in background; the service may restart, so we poll status.
        const extra = data && data.tag ? ` (${data.tag})` : "";
        showStatus((window.__UPDATE_IN_PROGRESS__ || "En cours...") + extra, null);
      }

      let attempts = 0;
      while (attempts < 60) {
        await new Promise((r) => setTimeout(r, 2000));
        const st = await fetchStatus().catch(() => ({}));
        if (!st || st.running === true) {
          attempts++;
          continue;
        }
        const last = st.last || {};
        if (last && last.message === "already_up_to_date") {
          const { extra } = summarize(last);
          showStatus((window.__UPDATE_ALREADY__ || "Déjà à jour.") + extra, true);
          showOutput("");
          return;
        }
        if (last && last.ok) {
          const { extra, out, cmd } = summarize(last);
          showStatus((window.__UPDATE_SUCCESS__ || "OK") + extra, true);
          showOutput([cmd ? `command: ${cmd}` : "", out].filter(Boolean).join("\n\n"));
          return;
        }
        const { extra, out, cmd, msg } = summarize(last);
        showStatus((window.__UPDATE_ERROR__ || "Erreur") + ` (${[msg, last.tag].filter(Boolean).join(" / ")})`, false);
        showOutput([cmd ? `command: ${cmd}` : "", out].filter(Boolean).join("\n\n"));
        return;
      }

      showStatus(window.__UPDATE_IN_PROGRESS__ || "En cours...", null);
      showOutput("Le service redémarre peut-être encore. Réessaie dans quelques instants ou recharge la page.");
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

