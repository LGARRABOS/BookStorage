package server

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "bookstorage",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Total HTTP requests processed (excludes /metrics scrape path).",
		},
		[]string{"method", "status_class"},
	)
	httpDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "bookstorage",
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "HTTP request latencies in seconds (excludes /metrics scrape path).",
			Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{"method", "status_class"},
	)
)

func httpStatusClass(code int) string {
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

func shortHTTPMethod(m string) string {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions:
		return m
	default:
		return "OTHER"
	}
}

// RecordHTTPMetrics updates Prometheus counters/histograms for one completed request.
func RecordHTTPMetrics(method string, status int, dur time.Duration) {
	sm := shortHTTPMethod(method)
	sc := httpStatusClass(status)
	httpRequests.WithLabelValues(sm, sc).Inc()
	httpDuration.WithLabelValues(sm, sc).Observe(dur.Seconds())
}

func secureStringEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// metricsRemoteHost uses only the direct TCP remote address (never X-Forwarded-For)
// so clients cannot spoof loopback to reach /metrics without a token.
func metricsRemoteHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}

func (a *App) metricsRequestAuthorized(r *http.Request) bool {
	token := strings.TrimSpace(a.Settings.MetricsToken)
	if token != "" {
		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		if fields := strings.Fields(auth); len(fields) == 2 && strings.EqualFold(fields[0], "Bearer") {
			got := strings.TrimSpace(fields[1])
			if secureStringEqual(got, token) {
				return true
			}
		}
		q := strings.TrimSpace(r.URL.Query().Get("token"))
		if q != "" && secureStringEqual(q, token) {
			return true
		}
		return false
	}
	ip := net.ParseIP(metricsRemoteHost(r))
	return ip != nil && ip.IsLoopback()
}

// HandleMetrics serves Prometheus text exposition for GET /metrics.
func (a *App) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !a.metricsRequestAuthorized(r) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	promhttp.HandlerFor(
		prometheus.DefaultGatherer,
		promhttp.HandlerOpts{EnableOpenMetrics: false},
	).ServeHTTP(w, r)
}
