package catalog

// Cover lookup clients for bande dessinée enrichment (used by internal/server):
//
//	bnf_covers.go      — BnF Couvertures by ISBN/EAN
//	google_books.go    — Google Books (optional API key)
//	openlibrary_bd.go  — Open Library search + ISBN covers
//	openlibrary_errors.go
//
// Prefer ISBN lookups; title search is a fallback only (see server bd_cover_resolve.go).
