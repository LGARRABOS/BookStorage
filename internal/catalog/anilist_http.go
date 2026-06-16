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
// The caller must close resp.Body (typically via defer right after a successful Do/Post).
func decodeAnilistResponse(resp *http.Response, out any) error {
	if err := classifyAnilistHTTPStatus(resp.StatusCode); err != nil {
		return err
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, anilistMaxResponseBytes)).Decode(out); err != nil {
		return wrapAnilistTransport(err)
	}
	return nil
}

// anilistPostAndDecode POSTs to the AniList API and decodes the JSON response.
func anilistPostAndDecode(body []byte, out any) error {
	resp, err := anilistPost(body)
	if err != nil {
		return wrapAnilistTransport(err)
	}
	defer func() { _ = resp.Body.Close() }()
	return decodeAnilistResponse(resp, out)
}

func firstGraphQLError(messages []string) error {
	if len(messages) == 0 {
		return nil
	}
	return anilistGraphQLError(messages[0])
}
