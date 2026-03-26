package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"bookstorage/internal/recommend"
)

type dismissedRecommendation struct {
	Source     string
	ExternalID string
}

func loadDismissedRecommendations(db *sql.DB, userID int, source string) (map[string]struct{}, error) {
	rows, err := db.Query(
		`SELECT external_id FROM dismissed_recommendations WHERE user_id = ? AND source = ?`,
		userID, source,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]struct{})
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		id = strings.TrimSpace(id)
		if id != "" {
			out[id] = struct{}{}
		}
	}
	return out, rows.Err()
}

func filterDismissedSuggestions(res *recommend.ForUserResult, dismissedAnilistIDs map[string]struct{}) {
	if res == nil || len(res.Results) == 0 || len(dismissedAnilistIDs) == 0 {
		return
	}
	out := res.Results[:0]
	for _, s := range res.Results {
		if s.AnilistID <= 0 {
			out = append(out, s)
			continue
		}
		if _, ok := dismissedAnilistIDs[strconv.Itoa(s.AnilistID)]; ok {
			continue
		}
		out = append(out, s)
	}
	res.Results = out
}

func (a *App) HandleDismissRecommendation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	var req struct {
		Source    string `json:"source"`
		AnilistID int    `json:"anilist_id"`
	}

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/json") {
		_ = json.NewDecoder(r.Body).Decode(&req)
	} else {
		_ = r.ParseForm()
		req.Source = r.FormValue("source")
		if v := strings.TrimSpace(r.FormValue("anilist_id")); v != "" {
			n, _ := strconv.Atoi(v)
			req.AnilistID = n
		}
	}

	if strings.TrimSpace(req.Source) == "" {
		req.Source = "anilist"
	}
	if req.Source != "anilist" || req.AnilistID <= 0 {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "invalid"})
		return
	}

	ext := strconv.Itoa(req.AnilistID)
	_, err := a.DB.Exec(
		`INSERT OR IGNORE INTO dismissed_recommendations (user_id, source, external_id) VALUES (?, ?, ?)`,
		userID, req.Source, ext,
	)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "db"})
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}
