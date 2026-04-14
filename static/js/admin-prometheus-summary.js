(() => {
  const $ = (id) => document.getElementById(id);

  function setVisible(el, on) {
    if (!el) return;
    el.style.display = on ? "block" : "none";
  }

  function setText(id, v) {
    const el = $(id);
    if (el) el.textContent = v ?? "—";
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

      setText("prom-scrape-val", d.scrape_ok ? "OK" : "—");
      setText("prom-total-val", d.requests_total);
      setText("prom-rate-val", d.request_rate_5m);
      setText("prom-err-rate-val", d.error_rate_5m);
      setText("prom-2xx-val", d.requests_2xx);
      setText("prom-3xx-val", d.requests_3xx);
      setText("prom-4xx-val", d.requests_4xx);
      setText("prom-5xx-val", d.requests_5xx);
      setText("prom-get-val", d.requests_get);
      setText("prom-post-val", d.requests_post);
      setText("prom-p50-val", d.latency_p50);
      setText("prom-p95-val", d.latency_p95);
    } catch {
      /* ignore transient errors */
    }
  }

  document.addEventListener("DOMContentLoaded", () => {
    refresh();
    setInterval(refresh, 12000);
  });
})();
