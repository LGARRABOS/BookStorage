package catalog

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// ErrAnilistRateLimit is returned when AniList rejects requests due to rate limiting.
var ErrAnilistRateLimit = errors.New("anilist rate limit")

// ErrAnilistUpstream is returned for other AniList API failures (network, HTTP, GraphQL).
var ErrAnilistUpstream = errors.New("anilist upstream")

// IsAnilistRateLimit reports whether err is an AniList rate-limit error.
func IsAnilistRateLimit(err error) bool {
	return errors.Is(err, ErrAnilistRateLimit)
}

// AnilistErrorCode maps an AniList error to a stable API error code for clients.
func AnilistErrorCode(err error) string {
	if IsAnilistRateLimit(err) {
		return "rate_limit"
	}
	return "upstream"
}

func classifyAnilistHTTPStatus(code int) error {
	if code == http.StatusTooManyRequests {
		return ErrAnilistRateLimit
	}
	if code < 200 || code >= 300 {
		return fmt.Errorf("%w: http %d", ErrAnilistUpstream, code)
	}
	return nil
}

func anilistGraphQLError(message string) error {
	if isRateLimitMessage(message) {
		return fmt.Errorf("%w: %s", ErrAnilistRateLimit, message)
	}
	return fmt.Errorf("%w: %s", ErrAnilistUpstream, message)
}

func isRateLimitMessage(msg string) bool {
	s := strings.ToLower(strings.TrimSpace(msg))
	return strings.Contains(s, "rate limit") ||
		strings.Contains(s, "too many requests") ||
		strings.Contains(s, "ratelimit")
}

func wrapAnilistTransport(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", ErrAnilistUpstream, err)
}
