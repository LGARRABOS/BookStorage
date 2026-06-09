package catalog

import (
	"encoding/json"
	"net/http"
	"strings"
)

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
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
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
