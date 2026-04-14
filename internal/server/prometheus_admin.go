package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 3 * time.Second}

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
			out.RequestsTotal = strconv.FormatFloat(v, 'f', 0, 64)
		} else {
			out.RequestsTotal = "0"
		}
	} else {
		out.RequestsTotal = "—"
	}

	rateResp, err := prometheusInstantQuery(ctx, client, base, `sum(rate(bookstorage_http_requests_total[5m]))`)
	if err == nil {
		if v, ok := firstScalarVector(rateResp); ok {
			out.RequestRate5m = strconv.FormatFloat(v, 'f', 4, 64)
		} else {
			out.RequestRate5m = "0"
		}
	} else {
		out.RequestRate5m = "—"
	}

	return out
}
