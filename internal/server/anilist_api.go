package server

import (
	"encoding/json"
	"log"
	"net/http"

	"bookstorage/internal/catalog"
)

func anilistAPIErrorJSON(err error) string {
	return catalog.AnilistErrorCode(err)
}

func writeAnilistUpstreamJSON(w http.ResponseWriter, logPrefix string, err error, payload map[string]any) {
	if payload == nil {
		payload = map[string]any{}
	}
	payload["error"] = anilistAPIErrorJSON(err)
	log.Printf("%s: %v", logPrefix, err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadGateway)
	_ = json.NewEncoder(w).Encode(payload)
}
