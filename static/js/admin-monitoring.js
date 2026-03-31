(() => {
  const $ = (id) => document.getElementById(id);

  function fmtSeconds(s) {
    s = Math.max(0, Math.floor(s || 0));
    const d = Math.floor(s / 86400);
    s -= d * 86400;
    const h = Math.floor(s / 3600);
    s -= h * 3600;
    const m = Math.floor(s / 60);
    s -= m * 60;
    const parts = [];
    if (d) parts.push(`${d}j`);
    if (h || d) parts.push(`${h}h`);
    if (m || h || d) parts.push(`${m}m`);
    parts.push(`${s}s`);
    return parts.join(" ");
  }

  function fmtBytes(b) {
    b = Number(b || 0);
    const units = ["B", "KB", "MB", "GB", "TB"];
    let u = 0;
    while (b >= 1024 && u < units.length - 1) {
      b /= 1024;
      u++;
    }
    return `${b.toFixed(u ? 1 : 0)} ${units[u]}`;
  }

  async function refresh() {
    const err = $("mon-error");
    try {
      const res = await fetch("/api/admin/monitoring", { headers: { "Accept": "application/json" } });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const d = await res.json();

      $("mon-uptime").textContent = fmtSeconds(d.uptime_seconds);
      $("mon-req-total").textContent = d.requests_total ?? 0;
      const by = d.requests_by || {};
      $("mon-req-2xx").textContent = by["2xx"] ?? 0;
      $("mon-req-3xx").textContent = by["3xx"] ?? 0;
      $("mon-req-4xx").textContent = by["4xx"] ?? 0;
      $("mon-req-5xx").textContent = by["5xx"] ?? 0;

      const lat = d.latency_ms || {};
      $("mon-lat-p95").textContent = (lat.p95 ?? 0).toFixed(0);
      $("mon-lat-avg").textContent = (lat.avg ?? 0).toFixed(0);
      $("mon-lat-max").textContent = (lat.max ?? 0).toFixed(0);

      const rt = d.runtime || {};
      $("mon-go").textContent = rt.goroutines ?? 0;
      $("mon-heap").textContent = fmtBytes(rt.heap_alloc_bytes ?? 0);
      $("mon-gc").textContent = rt.num_gc ?? 0;

      if (err) err.style.display = "none";
    } catch (e) {
      if (err) {
        err.textContent = `Erreur monitoring: ${e?.message || e}`;
        err.style.display = "block";
      }
    }
  }

  document.addEventListener("DOMContentLoaded", () => {
    refresh();
    setInterval(refresh, 3000);
  });
})();

