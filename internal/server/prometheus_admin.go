package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"bookstorage/internal/config"
)

const defaultPrometheusQueryURL = "http://127.0.0.1:9091"

// PrometheusAdminSummary is passed to the admin monitoring template and JSON API.
type PrometheusAdminSummary struct {
	Reachable        bool
	QueryBase        string
	Up               float64
	RequestsTotal    string
	RequestRate5m    string
	Requests2xx      string
	Requests3xx      string
	Requests4xx      string
	Requests5xx      string
	RequestsGet      string
	RequestsPost     string
	ErrorRate5m      string
	LatencyP50       string
	LatencyP95       string
	Error            string
	ScrapeJobHealthy bool
}

type promAPIResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []any             `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

func prometheusQueryBaseForSettings(s *config.Settings) string {
	b := strings.TrimSpace(s.PrometheusQueryURL)
	if b == "" {
		return defaultPrometheusQueryURL
	}
	return strings.TrimRight(b, "/")
}

// prometheusQueryHostAllowed restricts server-side queries to loopback to limit SSRF from admin-triggered fetches.
func prometheusQueryHostAllowed(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	h := strings.ToLower(strings.TrimSpace(u.Hostname()))
	return h == "127.0.0.1" || h == "localhost" || h == "::1"
}

func firstScalarVector(resp *promAPIResponse) (float64, bool) {
	if resp == nil || resp.Status != "success" || len(resp.Data.Result) == 0 {
		return 0, false
	}
	v := resp.Data.Result[0].Value
	if len(v) < 2 {
		return 0, false
	}
	s, ok := v[1].(string)
	if !ok {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

func instantVectorByLabel(resp *promAPIResponse, label string) map[string]float64 {
	m := make(map[string]float64)
	if resp == nil || resp.Status != "success" {
		return m
	}
	for _, pt := range resp.Data.Result {
		lv := strings.TrimSpace(pt.Metric[label])
		if lv == "" {
			lv = "other"
		}
		if len(pt.Value) < 2 {
			continue
		}
		s, ok := pt.Value[1].(string)
		if !ok {
			continue
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			continue
		}
		m[lv] = f
	}
	return m
}

func formatPromCount(f float64) string {
	return strconv.FormatFloat(f, 'f', 0, 64)
}

func formatPromRate(f float64) string {
	return strconv.FormatFloat(f, 'f', 4, 64)
}

func formatPromLatencySeconds(f float64) string {
	if f < 0 || math.IsNaN(f) || math.IsInf(f, 0) {
		return "—"
	}
	if f >= 10 {
		return strconv.FormatFloat(f, 'f', 2, 64) + "s"
	}
	return strconv.FormatFloat(f, 'f', 4, 64) + "s"
}

func classCount(m map[string]float64, cls string) string {
	if v, ok := m[cls]; ok {
		return formatPromCount(v)
	}
	return "0"
}

func prometheusInstantQuery(ctx context.Context, client *http.Client, baseURL, query string) (*promAPIResponse, error) {
	q := url.Values{}
	q.Set("query", query)
	u := strings.TrimRight(baseURL, "/") + "/api/v1/query?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prometheus HTTP %d", res.StatusCode)
	}
	var out promAPIResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if out.Status != "success" {
		return nil, fmt.Errorf("prometheus status %q", out.Status)
	}
	return &out, nil
}

// FetchPrometheusAdminSummary queries the local Prometheus HTTP API (instant queries).
func FetchPrometheusAdminSummary(s *config.Settings) PrometheusAdminSummary {
	out := PrometheusAdminSummary{QueryBase: prometheusQueryBaseForSettings(s)}
	base := out.QueryBase
	if !prometheusQueryHostAllowed(base) {
		out.Error = "invalid_prometheus_url"
		return out
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 5 * time.Second}

	upResp, err := prometheusInstantQuery(ctx, client, base, `up{job="bookstorage"}`)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	out.Reachable = true
	upVal, upOK := firstScalarVector(upResp)
	out.Up = upVal
	out.ScrapeJobHealthy = upOK && upVal >= 1

	totalResp, err := prometheusInstantQuery(ctx, client, base, `sum(bookstorage_http_requests_total)`)
	if err == nil {
		if v, ok := firstScalarVector(totalResp); ok {
			out.RequestsTotal = formatPromCount(v)
		} else {
			out.RequestsTotal = "0"
		}
	} else {
		out.RequestsTotal = "—"
	}

	rateResp, err := prometheusInstantQuery(ctx, client, base, `sum(rate(bookstorage_http_requests_total[5m]))`)
	if err == nil {
		if v, ok := firstScalarVector(rateResp); ok {
			out.RequestRate5m = formatPromRate(v)
		} else {
			out.RequestRate5m = "0"
		}
	} else {
		out.RequestRate5m = "—"
	}

	if byClass, err := prometheusInstantQuery(ctx, client, base, `sum(bookstorage_http_requests_total) by (status_class)`); err == nil {
		m := instantVectorByLabel(byClass, "status_class")
		out.Requests2xx = classCount(m, "2xx")
		out.Requests3xx = classCount(m, "3xx")
		out.Requests4xx = classCount(m, "4xx")
		out.Requests5xx = classCount(m, "5xx")
	} else {
		out.Requests2xx, out.Requests3xx, out.Requests4xx, out.Requests5xx = "—", "—", "—", "—"
	}

	if byMethod, err := prometheusInstantQuery(ctx, client, base, `sum(bookstorage_http_requests_total) by (method)`); err == nil {
		mm := instantVectorByLabel(byMethod, "method")
		out.RequestsGet = classCount(mm, "GET")
		out.RequestsPost = classCount(mm, "POST")
	} else {
		out.RequestsGet, out.RequestsPost = "—", "—"
	}

	if err5, err := prometheusInstantQuery(ctx, client, base, `sum(rate(bookstorage_http_requests_total{status_class="5xx"}[5m]))`); err == nil {
		if v, ok := firstScalarVector(err5); ok {
			out.ErrorRate5m = formatPromRate(v)
		} else {
			out.ErrorRate5m = "0"
		}
	} else {
		out.ErrorRate5m = "—"
	}

	if p50r, err := prometheusInstantQuery(ctx, client, base, `histogram_quantile(0.50, sum(rate(bookstorage_http_request_duration_seconds_bucket[5m])) by (le))`); err == nil {
		if v, ok := firstScalarVector(p50r); ok {
			out.LatencyP50 = formatPromLatencySeconds(v)
		} else {
			out.LatencyP50 = "—"
		}
	} else {
		out.LatencyP50 = "—"
	}

	if p95r, err := prometheusInstantQuery(ctx, client, base, `histogram_quantile(0.95, sum(rate(bookstorage_http_request_duration_seconds_bucket[5m])) by (le))`); err == nil {
		if v, ok := firstScalarVector(p95r); ok {
			out.LatencyP95 = formatPromLatencySeconds(v)
		} else {
			out.LatencyP95 = "—"
		}
	} else {
		out.LatencyP95 = "—"
	}

	return out
}
