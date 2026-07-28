package server

// Package server implements the BookStorage HTTP application (HTML + REST API).
//
// File layout (same package, no subpackages):
//
//	handlers_core.go     — App struct, NewApp, template loading
//	handlers_render.go   — renderTemplate, auth middleware, view mode
//	handlers_*.go        — HTML handlers by domain (auth, dashboard, anime, bd, admin…)
//	routes_*.go          — Register*Routes
//	paths_*.go           — URL path constants
//	*_row.go             — DB row types + scanners
//	import_export_*.go   — import/export (parse vs handlers when split)
//	*_cover_*.go         — cover enrichment jobs
//	webhooks_*.go        — outbound webhooks (store / worker / handlers)
//	api*.go              — JSON API
//
// See docs/architecture-backend.md for contributor guidance.
