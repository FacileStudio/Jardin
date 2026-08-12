package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/FacileStudio/Jardin/internal/sessions"
)

func TestClaimsListReturnsJSON(t *testing.T) {
	s := New(t.TempDir(), "secret")
	h := s.Handler()
	token := loginAs(t, h, "secret", "lucy")

	if err := sessions.SaveClaim(s.DataDir, &sessions.Claim{
		Project:   "Jardin",
		Machine:   "lucy",
		Agent:     "pi",
		Task:      "add claims UI",
		StartedAt: time.Now(),
		UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/claims", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"project":"Jardin"`) {
		t.Fatalf("missing claim in body: %s", rec.Body.String())
	}
}

func TestClaimsReleaseRemovesClaim(t *testing.T) {
	s := New(t.TempDir(), "secret")
	h := s.Handler()
	token := loginAs(t, h, "secret", "lucy")

	if err := sessions.SaveClaim(s.DataDir, &sessions.Claim{
		Project:   "Jardin",
		Machine:   "lucy",
		Agent:     "pi",
		Task:      "add claims UI",
		StartedAt: time.Now(),
		UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("DELETE", "/api/claims/Jardin/lucy/pi", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}

	claim, err := sessions.LoadClaim(s.DataDir, "Jardin", "lucy", "pi")
	if err != nil {
		t.Fatal(err)
	}
	if claim != nil {
		t.Fatal("claim should be released")
	}
}

func TestClaimsReleaseMissingIsNotError(t *testing.T) {
	s := New(t.TempDir(), "secret")
	h := s.Handler()
	token := loginAs(t, h, "secret", "lucy")

	req := httptest.NewRequest("DELETE", "/api/claims/Jardin/lucy/pi", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d, body %s", rec.Code, rec.Body.String())
	}
}
