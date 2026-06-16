package server

import (
	"net/http"
)

type apiReadingSite struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	BaseURL         string `json:"base_url"`
	ProbeStatus     string `json:"probe_status"`
	LastProbeAt     string `json:"last_probe_at,omitempty"`
	ProbeHTTPStatus *int   `json:"probe_http_status,omitempty"`
	ProbeDetail     string `json:"probe_detail,omitempty"`
}

func readingSiteToAPI(s readingSite) apiReadingSite {
	out := apiReadingSite{
		ID:          s.ID,
		Name:        s.Name,
		BaseURL:     s.BaseURL,
		ProbeStatus: s.ProbeStatus,
	}
	if s.LastProbeAt.Valid {
		out.LastProbeAt = s.LastProbeAt.String
	}
	if s.ProbeHTTPStatus.Valid && s.ProbeHTTPStatus.Int64 > 0 {
		v := int(s.ProbeHTTPStatus.Int64)
		out.ProbeHTTPStatus = &v
	}
	if s.ProbeDetail.Valid {
		out.ProbeDetail = s.ProbeDetail.String
	}
	return out
}

func (a *App) HandleAPIReadingSitesList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.apiWriteError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	userID, _ := a.currentUserID(r)

	sites := a.loadUserReadingSites(userID)
	data := make([]apiReadingSite, 0, len(sites))
	for _, site := range sites {
		data = append(data, readingSiteToAPI(site))
	}
	a.apiWriteJSON(w, http.StatusOK, map[string]any{"data": data})
}
