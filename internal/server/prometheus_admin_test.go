package server

import (
	"encoding/json"
	"testing"

	"bookstorage/internal/config"
)

func TestPrometheusQueryHostAllowed(t *testing.T) {
	tests := []struct {
		u    string
		want bool
	}{
		{"http://127.0.0.1:9091", true},
		{"http://localhost:9091/", true},
		{"http://[::1]:9091", true},
		{"https://127.0.0.1:9091", true},
		{"http://10.0.0.1:9091", false},
		{"http://evil.example/", false},
		{"ftp://127.0.0.1:9091", false},
		{"", false},
		{"http://", false},
	}
	for _, tc := range tests {
		if got := prometheusQueryHostAllowed(tc.u); got != tc.want {
			t.Errorf("%q: got %v want %v", tc.u, got, tc.want)
		}
	}
}

func TestInstantVectorByLabel(t *testing.T) {
	raw := `{"status":"success","data":{"resultType":"vector","result":[
		{"metric":{"status_class":"2xx"},"value":[1,"42"]},
		{"metric":{"status_class":"4xx"},"value":[1,"3"]}
	]}}`
	var resp promAPIResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatal(err)
	}
	m := instantVectorByLabel(&resp, "status_class")
	if m["2xx"] != 42 || m["4xx"] != 3 {
		t.Fatalf("map=%v", m)
	}
}

func TestPrometheusQueryBaseForSettings(t *testing.T) {
	s := &config.Settings{}
	if g := prometheusQueryBaseForSettings(s); g != defaultPrometheusQueryURL {
		t.Fatalf("empty: got %q", g)
	}
	s.PrometheusQueryURL = "http://localhost:9091/"
	if g := prometheusQueryBaseForSettings(s); g != "http://localhost:9091" {
		t.Fatalf("trim: got %q", g)
	}
}
