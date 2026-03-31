package server

import (
	"math"
	"runtime"
	"sort"
	"sync"
	"time"
)

type Monitoring struct {
	startedAt time.Time
	version   string
	env       string

	mu sync.Mutex

	// Request metrics (best-effort, low cost).
	totalRequests uint64
	byClass       map[string]uint64 // 2xx/3xx/4xx/5xx/other

	// Rolling latency window (ms).
	latencyMs []float64
	latIdx    int
	latFilled bool
}

func NewMonitoring(version, env string) *Monitoring {
	return &Monitoring{
		startedAt: time.Now(),
		version:   version,
		env:       env,
		byClass:   map[string]uint64{},
		latencyMs: make([]float64, 180), // ~ last 180 requests
		latIdx:    0,
		latFilled: false,
	}
}

func statusClass(code int) string {
	switch {
	case code >= 200 && code <= 299:
		return "2xx"
	case code >= 300 && code <= 399:
		return "3xx"
	case code >= 400 && code <= 499:
		return "4xx"
	case code >= 500 && code <= 599:
		return "5xx"
	default:
		return "other"
	}
}

func (m *Monitoring) Observe(status int, dur time.Duration) {
	if m == nil {
		return
	}
	ms := float64(dur.Milliseconds())
	if ms < 0 {
		ms = 0
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalRequests++
	m.byClass[statusClass(status)]++

	m.latencyMs[m.latIdx] = ms
	m.latIdx++
	if m.latIdx >= len(m.latencyMs) {
		m.latIdx = 0
		m.latFilled = true
	}
}

type MonitoringSnapshot struct {
	StartedAtUnix int64  `json:"started_at_unix"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	Version       string `json:"version"`
	Environment   string `json:"environment"`

	RequestsTotal uint64            `json:"requests_total"`
	RequestsBy    map[string]uint64 `json:"requests_by"`

	LatencyMs struct {
		Count int     `json:"count"`
		Min   float64 `json:"min"`
		Avg   float64 `json:"avg"`
		P95   float64 `json:"p95"`
		Max   float64 `json:"max"`
	} `json:"latency_ms"`

	Runtime struct {
		GoRoutines int    `json:"goroutines"`
		Alloc      uint64 `json:"alloc_bytes"`
		Sys        uint64 `json:"sys_bytes"`
		HeapAlloc  uint64 `json:"heap_alloc_bytes"`
		HeapSys    uint64 `json:"heap_sys_bytes"`
		NumGC      uint32 `json:"num_gc"`
		LastGCUnix int64  `json:"last_gc_unix"`
	} `json:"runtime"`
}

func (m *Monitoring) Snapshot() MonitoringSnapshot {
	var snap MonitoringSnapshot
	now := time.Now()
	if m != nil {
		snap.StartedAtUnix = m.startedAt.Unix()
		snap.UptimeSeconds = int64(now.Sub(m.startedAt).Seconds())
		snap.Version = m.version
		snap.Environment = m.env
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	snap.Runtime.GoRoutines = runtime.NumGoroutine()
	snap.Runtime.Alloc = mem.Alloc
	snap.Runtime.Sys = mem.Sys
	snap.Runtime.HeapAlloc = mem.HeapAlloc
	snap.Runtime.HeapSys = mem.HeapSys
	snap.Runtime.NumGC = mem.NumGC
	if mem.LastGC != 0 {
		snap.Runtime.LastGCUnix = int64(mem.LastGC / 1e9)
	}

	if m == nil {
		snap.RequestsBy = map[string]uint64{}
		return snap
	}

	m.mu.Lock()
	snap.RequestsTotal = m.totalRequests
	snap.RequestsBy = map[string]uint64{}
	for k, v := range m.byClass {
		snap.RequestsBy[k] = v
	}

	// Copy latency window in order-independent way.
	var vals []float64
	if m.latFilled {
		vals = append(vals, m.latencyMs...)
	} else {
		vals = append(vals, m.latencyMs[:m.latIdx]...)
	}
	m.mu.Unlock()

	computeLatencyStats(&snap, vals)
	return snap
}

func computeLatencyStats(snap *MonitoringSnapshot, vals []float64) {
	n := len(vals)
	snap.LatencyMs.Count = n
	if n == 0 {
		return
	}
	minV := math.Inf(1)
	maxV := 0.0
	sum := 0.0
	for _, v := range vals {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
		sum += v
	}
	snap.LatencyMs.Min = minV
	snap.LatencyMs.Max = maxV
	snap.LatencyMs.Avg = sum / float64(n)

	sorted := make([]float64, 0, n)
	sorted = append(sorted, vals...)
	sort.Float64s(sorted)
	p95Idx := int(math.Ceil(0.95*float64(n))) - 1
	if p95Idx < 0 {
		p95Idx = 0
	}
	if p95Idx >= n {
		p95Idx = n - 1
	}
	snap.LatencyMs.P95 = sorted[p95Idx]
}
