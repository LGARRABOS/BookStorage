package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
)

func toolsImportReportData(r *http.Request) map[string]any {
	data := map[string]any{}
	if enc := r.URL.Query().Get("import_report"); enc != "" {
		raw, err := base64.RawURLEncoding.DecodeString(enc)
		if err == nil {
			var rep ImportReport
			if json.Unmarshal(raw, &rep) == nil {
				data["ImportReport"] = rep
			}
		}
	}
	if r.URL.Query().Get("error") == "import" {
		data["ImportError"] = true
	}
	if v := strings.TrimSpace(r.URL.Query().Get("csv_imported")); v != "" {
		data["CSVImportCount"] = v
	}
	return data
}

// HandleToolsIndex renders the hub-level Tools landing page (/tools).
func (a *App) HandleToolsIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	a.renderTemplate(w, r, "tools", a.mergeData(r, map[string]any{}))
}

// HandleToolsManga renders manga/webtoon tools (export, import, duplicates, CSV assistant).
func (a *App) HandleToolsManga(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	a.renderTemplate(w, r, "tools_manga", a.mergeData(r, toolsImportReportData(r)))
}

// HandleToolsAnime renders anime tools (export + import).
func (a *App) HandleToolsAnime(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	a.renderTemplate(w, r, "tools_anime", a.mergeData(r, toolsImportReportData(r)))
}

// HandleToolsBd renders bande dessinée tools (export + import).
func (a *App) HandleToolsBd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	a.renderTemplate(w, r, "tools_bd", a.mergeData(r, toolsImportReportData(r)))
}
