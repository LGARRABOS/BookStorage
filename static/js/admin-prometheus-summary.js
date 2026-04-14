(() => {
  const $ = (id) => document.getElementById(id);

  function setVisible(el, on) {
    if (!el) return;
    el.style.display = on ? "block" : "none";
  }

  async function refresh() {
    const errEl = $("prom-live-err");
    const panel = $("prom-live-panel");
    try {
      const res = await fetch("/api/admin/prometheus/summary", { headers: { Accept: "application/json" } });
      if (!res.ok) return;
      const d = await res.json();

      if (d.invalid_url) {
        setVisible(panel, false);
        if (errEl) {
          errEl.textContent = errEl.dataset.errInvalid || "Invalid Prometheus URL.";
          errEl.style.display = "block";
        }
        return;
      }

      if (!d.reachable) {
        setVisible(panel, false);
        if (errEl) {
          errEl.textContent = d.error || errEl.dataset.errUnreachable || "Unreachable.";
          errEl.style.display = "block";
        }
        return;
      }

      setVisible(panel, true);
      if (errEl) errEl.style.display = "none";

      const scrape = $("prom-scrape-val");
      if (scrape) scrape.textContent = d.scrape_ok ? "OK" : "—";

      const tot = $("prom-total-val");
      if (tot) tot.textContent = d.requests_total ?? "—";

      const rate = $("prom-rate-val");
      if (rate) rate.textContent = d.request_rate_5m ?? "—";
    } catch {
      /* ignore transient errors */
    }
  }

  document.addEventListener("DOMContentLoaded", () => {
    refresh();
    setInterval(refresh, 12000);
  });
})();
