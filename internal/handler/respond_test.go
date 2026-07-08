package handler

import (
	"net/http/httptest"
	"testing"
)

func TestParsePage(t *testing.T) {
	tests := []struct {
		url         string
		wantPage    int
		wantPerPage int
	}{
		{"/", 1, 20},
		{"/?page=3&per_page=50", 3, 50},
		{"/?page=0&per_page=0", 1, 20},
		{"/?page=-1&per_page=101", 1, 20},
		{"/?page=abc&per_page=xyz", 1, 20},
		{"/?per_page=100", 1, 100},
	}

	for _, tt := range tests {
		req := httptest.NewRequest("GET", tt.url, nil)
		page, perPage := parsePage(req)
		if page != tt.wantPage || perPage != tt.wantPerPage {
			t.Errorf("parsePage(%q) = (%d, %d), want (%d, %d)",
				tt.url, page, perPage, tt.wantPage, tt.wantPerPage)
		}
	}
}

func TestPhotoURLPath(t *testing.T) {
	if got := photoURLPath("photos/abc.jpg"); got != "/api/photos/photos/abc.jpg" {
		t.Errorf("photoURLPath = %q", got)
	}
}
