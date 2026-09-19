package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlePasswordResetOpen(t *testing.T) {
	a := &API{}
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/reset?token=abc-123", nil)
	rec := httptest.NewRecorder()
	if err := a.handlePasswordResetOpen(rec, req); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `href="parkxchange://auth/reset?token=abc-123"`) {
		t.Fatalf("missing deep link in body: %s", body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("content-type %q", ct)
	}
}

func TestHandlePasswordResetOpenRequiresToken(t *testing.T) {
	a := &API{}
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/reset", nil)
	rec := httptest.NewRecorder()
	err := a.handlePasswordResetOpen(rec, req)
	if err == nil {
		t.Fatal("expected error for missing token")
	}
}
