package catalog

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestIsAnilistRateLimit(t *testing.T) {
	t.Parallel()
	if !IsAnilistRateLimit(ErrAnilistRateLimit) {
		t.Fatal("expected sentinel rate limit")
	}
	if !IsAnilistRateLimit(fmt.Errorf("wrap: %w", ErrAnilistRateLimit)) {
		t.Fatal("expected wrapped rate limit")
	}
	if IsAnilistRateLimit(ErrAnilistUpstream) {
		t.Fatal("upstream must not match rate limit")
	}
	if IsAnilistRateLimit(errors.New("other")) {
		t.Fatal("unrelated error must not match")
	}
}

func TestClassifyAnilistHTTPStatus(t *testing.T) {
	t.Parallel()
	if err := classifyAnilistHTTPStatus(http.StatusTooManyRequests); !IsAnilistRateLimit(err) {
		t.Fatalf("429: got %v", err)
	}
	if err := classifyAnilistHTTPStatus(http.StatusOK); err != nil {
		t.Fatalf("200: got %v", err)
	}
	if err := classifyAnilistHTTPStatus(http.StatusBadGateway); !errors.Is(err, ErrAnilistUpstream) {
		t.Fatalf("502: got %v", err)
	}
}

func TestAnilistGraphQLError(t *testing.T) {
	t.Parallel()
	if err := anilistGraphQLError("Too Many Requests."); !IsAnilistRateLimit(err) {
		t.Fatalf("message: got %v", err)
	}
	if err := anilistGraphQLError("Internal server error"); !errors.Is(err, ErrAnilistUpstream) {
		t.Fatalf("generic gql: got %v", err)
	}
}

func TestAnilistErrorCode(t *testing.T) {
	t.Parallel()
	if got := AnilistErrorCode(ErrAnilistRateLimit); got != "rate_limit" {
		t.Fatalf("got %q", got)
	}
	if got := AnilistErrorCode(ErrAnilistUpstream); got != "upstream" {
		t.Fatalf("got %q", got)
	}
}
