package server

import (
	"context"
	"testing"
)

func TestProbeURL_liveExample(t *testing.T) {
	if testing.Short() {
		t.Skip("network")
	}
	status, code, detail := ProbeURL(context.Background(), "https://example.com/")
	t.Logf("status=%s code=%d detail=%q", status, code, detail)
	if status != ProbeStatusUp {
		t.Fatalf("expected up, got status=%s code=%d detail=%q", status, code, detail)
	}
}
