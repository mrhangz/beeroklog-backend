package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func corsHandler(origins []string) http.Handler {
	return CORS(origins)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
}

func TestCORSAllowsListedOrigin(t *testing.T) {
	handler := corsHandler([]string{"http://localhost:3000"})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Errorf("Allow-Origin = %q, want http://localhost:3000", got)
	}
}

func TestCORSIgnoresUnlistedOrigin(t *testing.T) {
	handler := corsHandler([]string{"http://localhost:3000"})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin = %q, want empty", got)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (request still served)", rec.Code)
	}
}

func TestCORSPreflightShortCircuits(t *testing.T) {
	handler := CORS([]string{"http://localhost:3000"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("preflight should not reach the handler")
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/reviews/", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("missing Access-Control-Allow-Methods on preflight")
	}
}

func TestCORSWildcard(t *testing.T) {
	handler := corsHandler([]string{"*"})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://anywhere.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Allow-Origin = %q, want *", got)
	}
}
