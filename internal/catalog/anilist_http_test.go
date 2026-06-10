package catalog

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestDecodeAnilistResponseLimitsBodySize(t *testing.T) {
	t.Parallel()
	huge := strings.Repeat("x", anilistMaxResponseBytes+1024)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(huge)),
	}
	var out map[string]any
	err := decodeAnilistResponse(resp, &out)
	if err == nil {
		t.Fatal("expected error for oversized response body")
	}
}
