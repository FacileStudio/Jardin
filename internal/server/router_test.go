package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func errorCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not an error envelope: %q", w.Body.String())
	}
	return body.Error.Code
}

func TestHealthAnswersAtRootAndUnderAPI(t *testing.T) {
	h := New(t.TempDir(), "secret").Handler()

	for _, path := range []string{"/health", "/ready", "/api/health", "/api/ready"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != http.StatusOK {
			t.Errorf("GET %s: got %d, want 200 (body %s)", path, w.Code, w.Body.String())
		}
	}
}

func TestUnknownAPIRouteAnswersEnvelopeNotSPA(t *testing.T) {
	h := New(t.TempDir(), "secret").Handler()
	h.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("<!doctype html>"))
	}))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/nope", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown API route: got %d, want 404 (body %s)", w.Code, w.Body.String())
	}
	if code := errorCode(t, w); code != "not_found" {
		t.Fatalf("unknown API route: code %q, want not_found", code)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("DELETE", "/api/auth/config", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("wrong method: got %d, want 405", w.Code)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/spaces", nil))
	if w.Body.String() != "<!doctype html>" {
		t.Fatalf("client route must reach the SPA, got %q", w.Body.String())
	}
}

// chi matches on the raw path as soon as a request carries any percent-encoding
// and hands parameters back still encoded, where the http.ServeMux this router
// replaced decoded them. Every guard downstream reads decoded values.
func TestPathParamIsDecodedLikeServeMux(t *testing.T) {
	cases := map[string]string{
		"/x/myrule":               "myrule",
		"/x/yann%40facile.studio": "yann@facile.studio",
		"/x/a%2Fb":                "a/b",
		"/x/..%2f..%2fpasswd":     "../../passwd",
	}
	for target, want := range cases {
		router := New(t.TempDir(), "").Handler()
		var got string
		router.Get("/x/{name}", func(_ http.ResponseWriter, r *http.Request) {
			got = pathParam(r, "name")
		})
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", target, nil))
		if got != want {
			t.Errorf("%s: pathParam = %q, want %q", target, got, want)
		}
	}
}

func TestPanicAnswersTheErrorEnvelope(t *testing.T) {
	h := New(t.TempDir(), "").Handler()
	h.Get("/boom", func(http.ResponseWriter, *http.Request) { panic("kaboom") })

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/boom", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("panicking handler: got %d, want 500", w.Code)
	}
	if code := errorCode(t, w); code != "internal" {
		t.Fatalf("panicking handler: code %q, want internal", code)
	}
}
