package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	docs "github.com/FacileStudio/Mycelium/internal/documentation"
	"github.com/FacileStudio/tronc/apiref"
)

func TestEveryRouteIsDocumented(t *testing.T) {
	server := &Server{Log: slog.Default(), DataDir: t.TempDir()}

	missing := apiref.Undocumented(server.Handler(), docs.Reference())
	if len(missing) > 0 {
		t.Errorf("routes missing from the API registry: %v", missing)
	}
}

func TestRegistryIsComplete(t *testing.T) {
	if issues := apiref.Incomplete(
		docs.Reference(),
		"/auth/logout",
		"/auth/oidc",
		"/auth/oidc/callback",
		"/spaces/{id}/leave",
	); len(issues) > 0 {
		t.Errorf("incomplete documentation routes: %v", issues)
	}
}

func TestReferenceIsServed(t *testing.T) {
	server := &Server{Log: slog.Default(), DataDir: t.TempDir()}
	handler := server.Handler()

	page := httptest.NewRecorder()
	handler.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/docs", nil))
	if page.Code != http.StatusOK {
		t.Fatalf("GET /docs = %d, want 200", page.Code)
	}
	if !strings.Contains(page.Body.String(), `data-url="/docs/openapi.json"`) {
		t.Errorf("the reference page does not point at the spec: %s", page.Body.String())
	}

	spec := httptest.NewRecorder()
	handler.ServeHTTP(spec, httptest.NewRequest(http.MethodGet, "/docs/openapi.json", nil))
	if spec.Code != http.StatusOK {
		t.Fatalf("GET /docs/openapi.json = %d, want 200", spec.Code)
	}
	var document struct {
		OpenAPI string `json:"openapi"`
		Info    struct {
			Title string `json:"title"`
		} `json:"info"`
		Paths map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(spec.Body.Bytes(), &document); err != nil {
		t.Fatalf("the spec is not JSON: %v", err)
	}
	if !strings.HasPrefix(document.OpenAPI, "3.1") {
		t.Errorf("openapi = %q, want 3.1.x", document.OpenAPI)
	}
	if document.Info.Title != docs.Reference().Title {
		t.Errorf("info.title = %q, want %q", document.Info.Title, docs.Reference().Title)
	}
	if len(document.Paths) == 0 {
		t.Error("the spec documents no paths")
	}
}
