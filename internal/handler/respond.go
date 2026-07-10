package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
)

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("respondJSON: encode: %v", err)
	}
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

func parsePage(r *http.Request) (page, perPage int) {
	page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ = strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	return
}

// parseSort returns a safe SQL column expression for feed ordering.
// Allowed values: created_at (default), tasted_at.
func parseSort(r *http.Request) string {
	switch r.URL.Query().Get("sort") {
	case "tasted_at":
		return "r.tasted_at"
	default:
		return "r.created_at"
	}
}
