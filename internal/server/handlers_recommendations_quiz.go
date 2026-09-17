package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"bookstorage/internal/recommend"
)

// HandleRecommendationQuiz returns extra AniList suggestions from a short questionnaire.
func (a *App) HandleRecommendationQuiz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var req recommend.QuizRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid", "results": []any{}})
		return
	}
	if !recommend.QuizReady(req) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid", "results": []any{}})
		return
	}

	dismissedIDs := map[int]struct{}{}
	if dismissed, err := loadDismissedRecommendations(a.DB, userID, "anilist"); err == nil {
		for idStr := range dismissed {
			if id, err := strconv.Atoi(strings.TrimSpace(idStr)); err == nil && id > 0 {
				dismissedIDs[id] = struct{}{}
			}
		}
	} else {
		log.Printf("dismissed recommendations: %v", err)
	}
	cfg := recommend.DefaultForUserConfig()
	cfg.DismissedIDs = dismissedIDs
	list, err := recommend.ForQuiz(a.DB, int64(userID), req, cfg)
	if err != nil {
		writeAnilistUpstreamJSON(w, "recommendation quiz", err, map[string]any{"results": []any{}})
		return
	}
	if list == nil {
		list = []recommend.Suggestion{}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"results": list,
		"lane":    strings.ToLower(strings.TrimSpace(req.Lane)),
	})
}
