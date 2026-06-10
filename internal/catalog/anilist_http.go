package catalog

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const anilistMaxResponseBytes = 4 << 20
const anilistMinInterval = 250 * time.Millisecond

var (
	alMu         sync.Mutex
	alLastCall   time.Time
	alHTTPClient = &http.Client{Timeout: anilistTimeout}
)

func anilistThrottle() {
	alMu.Lock()
	defer alMu.Unlock()
	if elapsed := time.Since(alLastCall); elapsed < anilistMinInterval {
		time.Sleep(anilistMinInterval - elapsed)
	}
	alLastCall = time.Now()
}

type anilistGraphQLErrorItem struct {
	Message string `json:"message"`
}

func anilistErrorMessages(items []anilistGraphQLErrorItem) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, e := range items {
		if m := strings.TrimSpace(e.Message); m != "" {
			out = append(out, m)
		}
	}
	return out
}

// decodeAnilistResponse checks HTTP status and decodes JSON into out.
func decodeAnilistResponse(resp *http.Response, out any) error {
	defer func() { _ = resp.Body.Close() }()
	if err := classifyAnilistHTTPStatus(resp.StatusCode); err != nil {
		return err
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, anilistMaxResponseBytes)).Decode(out); err != nil {
		return wrapAnilistTransport(err)
	}
	return nil
}

func firstGraphQLError(messages []string) error {
	if len(messages) == 0 {
		return nil
	}
	return anilistGraphQLError(messages[0])
}
