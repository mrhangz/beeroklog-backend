package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIssueAndVerifyRoundtrip(t *testing.T) {
	auth := NewAuth("test-secret")
	pair, err := auth.Issue("user-123", 15*time.Minute, 720*time.Hour)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if pair.ExpiresIn != int64((15 * time.Minute).Seconds()) {
		t.Errorf("ExpiresIn = %d, want %d", pair.ExpiresIn, 900)
	}

	var gotUserID string
	handler := auth.Verify(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = GetUserID(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if gotUserID != "user-123" {
		t.Errorf("user id = %q, want user-123", gotUserID)
	}
}

func TestVerifyRejectsRefreshToken(t *testing.T) {
	auth := NewAuth("test-secret")
	pair, err := auth.Issue("user-123", 15*time.Minute, 720*time.Hour)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	handler := auth.Verify(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be reached with a refresh token")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+pair.RefreshToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestValidateRefreshAcceptsOnlyRefreshTokens(t *testing.T) {
	auth := NewAuth("test-secret")
	pair, err := auth.Issue("user-123", 15*time.Minute, 720*time.Hour)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	userID, err := auth.ValidateRefresh(pair.RefreshToken)
	if err != nil {
		t.Fatalf("ValidateRefresh(refresh): %v", err)
	}
	if userID != "user-123" {
		t.Errorf("user id = %q, want user-123", userID)
	}

	if _, err := auth.ValidateRefresh(pair.AccessToken); err == nil {
		t.Error("ValidateRefresh accepted an access token")
	}
}

func TestVerifyRejectsWrongSecret(t *testing.T) {
	other := NewAuth("other-secret")
	pair, err := other.Issue("user-123", 15*time.Minute, 720*time.Hour)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	auth := NewAuth("test-secret")
	handler := auth.Verify(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be reached with a forged token")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestVerifyRejectsExpiredToken(t *testing.T) {
	auth := NewAuth("test-secret")
	pair, err := auth.Issue("user-123", -time.Minute, 720*time.Hour)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	handler := auth.Verify(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be reached with an expired token")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestVerifyRejectsMissingHeader(t *testing.T) {
	auth := NewAuth("test-secret")
	handler := auth.Verify(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be reached without a token")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}
