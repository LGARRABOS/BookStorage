package catalog

import "errors"

// ErrOpenLibraryRateLimit is returned when Open Library responds with HTTP 429.
var ErrOpenLibraryRateLimit = errors.New("openlibrary rate limit")

// IsOpenLibraryRateLimit reports whether err is an Open Library rate-limit error.
func IsOpenLibraryRateLimit(err error) bool {
	return errors.Is(err, ErrOpenLibraryRateLimit)
}
