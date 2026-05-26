package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
)

func (a *App) HandleTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
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
	a.renderTemplate(w, r, "tools", a.mergeData(r, data))
}
