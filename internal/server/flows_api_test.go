package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func writeUnder(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const oneStepFlow = "name: deploy\nsteps:\n  - name: build\n    run: 'true'\n"

// The dashboard signs in as a person, not a machine: its token carries an email
// and reaches admin scope through .users.json, which is a different branch of
// auth than a sync token takes. Both must reach the common tree, or content the
// CLI lists is invisible in the browser with nothing saying why.
func TestAdminSessionListsFlows(t *testing.T) {
	srv := New(t.TempDir(), "pw")
	writeUnder(t, srv.DataDir, "flows/deploy.yml", oneStepFlow)
	token := sessionFor(t, srv, "owner@example.test", true)

	rec := spReq(t, srv.Handler(), "GET", "/api/flows", token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var names []string
	if err := json.Unmarshal(rec.Body.Bytes(), &names); err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "deploy" {
		t.Fatalf("flows = %v, want [deploy]", names)
	}
}

// A denial must be a denial. Answering 200 with an empty list would render as
// "there is nothing here", which is indistinguishable from an empty tree and
// sends whoever hits it looking for a sync bug that does not exist.
func TestNonAdminUserIsDeniedRatherThanShownAnEmptyList(t *testing.T) {
	srv := New(t.TempDir(), "pw")
	writeUnder(t, srv.DataDir, "flows/deploy.yml", oneStepFlow)
	token := sessionFor(t, srv, "guest@example.test", false)

	for _, path := range []string{"/api/flows", "/api/models", "/api/rules", "/api/skills"} {
		rec := spReq(t, srv.Handler(), "GET", path, token, "")
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s = %d, want 403 — an empty 200 reads as an empty tree", path, rec.Code)
		}
	}
}

func TestFlowDetailCarriesRawAndParsedSteps(t *testing.T) {
	srv := New(t.TempDir(), "pw")
	writeUnder(t, srv.DataDir, "flows/deploy.yml",
		"name: deploy\nsteps:\n  - name: build\n    run: 'true'\n  - name: check\n    type: \"@acme/probe\"\n")
	token := sessionFor(t, srv, "owner@example.test", true)

	rec := spReq(t, srv.Handler(), "GET", "/api/flows/deploy", token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var detail FlowDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.ParseError != "" {
		t.Fatalf("parse error = %q, want none", detail.ParseError)
	}
	if detail.Raw == "" {
		t.Error("raw yaml is empty")
	}
	if len(detail.Summary.Steps) != 2 {
		t.Fatalf("%d steps, want 2", len(detail.Summary.Steps))
	}
	if detail.Summary.Steps[0].Kind != "run" || detail.Summary.Steps[1].Kind != "type" {
		t.Errorf("kinds = %q, %q; want run, type", detail.Summary.Steps[0].Kind, detail.Summary.Steps[1].Kind)
	}
	if detail.Summary.Steps[1].Type != "@acme/probe" {
		t.Errorf("typed step lost its type: %q", detail.Summary.Steps[1].Type)
	}
}

// A flow that stopped parsing is exactly the one its author needs to open, so
// the request answers with the raw file and the reason rather than failing.
func TestUnparseableFlowStillRenders(t *testing.T) {
	srv := New(t.TempDir(), "pw")
	writeUnder(t, srv.DataDir, "flows/broken.yml", "name: broken\nsteps: [::nonsense\n")
	token := sessionFor(t, srv, "owner@example.test", true)

	rec := spReq(t, srv.Handler(), "GET", "/api/flows/broken", token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with the reason", rec.Code)
	}
	var detail FlowDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.ParseError == "" {
		t.Error("a broken flow reported no parse error")
	}
	if detail.Raw == "" {
		t.Error("a broken flow must still hand back its raw text")
	}
}

// _lib holds the helper every model imports. It is code, but it is not a model,
// and listing it as one would offer a type name no flow can run.
func TestModelsListSkipsTheSharedLibrary(t *testing.T) {
	srv := New(t.TempDir(), "pw")
	writeUnder(t, srv.DataDir, "extensions/models/acme/http-check.ts", "export {};\n")
	writeUnder(t, srv.DataDir, "extensions/models/_lib/defineModel.ts", "export {};\n")
	writeUnder(t, srv.DataDir, "extensions/models/package.json", "{}\n")
	token := sessionFor(t, srv, "owner@example.test", true)

	rec := spReq(t, srv.Handler(), "GET", "/api/models", token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var models []ModelInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &models); err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].Type != "@acme/http-check" {
		t.Fatalf("models = %+v, want only @acme/http-check", models)
	}
}

// The models read route takes a wildcard, because a type name carries slashes.
// A wildcard is also the shape that walks out of the tree if nothing stops it.
func TestModelReadRefusesToLeaveTheModelsRoot(t *testing.T) {
	srv := New(t.TempDir(), "pw")
	writeUnder(t, srv.DataDir, "tokens.json", `{"secret":"do not serve me"}`)
	writeUnder(t, srv.DataDir, "extensions/models/acme/ok.ts", "export {};\n")
	token := sessionFor(t, srv, "owner@example.test", true)

	for _, escape := range []string{"../../tokens.json", "..%2f..%2ftokens.json", "/etc/passwd"} {
		rec := spReq(t, srv.Handler(), "GET", "/api/models/"+escape, token, "")
		if rec.Code == http.StatusOK {
			t.Errorf("%q was served: %s", escape, rec.Body.String())
		}
	}
}
