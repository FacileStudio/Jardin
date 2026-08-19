package flow

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const probeModel = `const schema = {
  type: "@test/probe",
  version: "1.0.0",
  arguments: {
    message: { type: "string", required: true },
    times: { type: "number", required: false },
    mood: { type: "string", required: false, enum: ["loud", "quiet"] },
  },
  outputs: ["echoed"],
};

const verb = process.argv[2];
if (verb === "describe") {
  console.log(JSON.stringify(schema));
} else {
  const input = JSON.parse(await Bun.stdin.text());
  console.log(JSON.stringify({ echoed: input.arguments.message }));
}
`

// writeModel installs a model and pins it, which is the state a machine is in
// after a person has read and approved it.
func writeModel(t *testing.T, typeName, body string) {
	t.Helper()
	path, err := ModelPath(typeName)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func trustModel(t *testing.T, typeName string) {
	t.Helper()
	path, _ := ModelPath(typeName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := TrustModel(&Model{Type: typeName, Path: path, Checksum: Checksum(data)}); err != nil {
		t.Fatal(err)
	}
}

func needsBun(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath(modelRuntime); err != nil {
		t.Skipf("%s is not installed", modelRuntime)
	}
}

// A model type is not a path: it must not be able to name a file outside the
// models directory.
func TestModelPathRefusesEscapes(t *testing.T) {
	for _, bad := range []string{"../../.ssh/id_rsa", "/etc/passwd", "", "@", "a/../../b"} {
		if _, err := ModelPath(bad); err == nil {
			t.Errorf("%q resolved to a path instead of being refused", bad)
		}
	}
}

// A model is code that arrives over sync. Running one unreviewed is the thing
// the trust gate exists to stop.
func TestUntrustedModelIsRefused(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	writeModel(t, "@test/probe", probeModel)

	_, err := LoadModel("@test/probe")
	if err == nil {
		t.Fatal("an unpinned model loaded")
	}
	if !strings.Contains(err.Error(), "not trusted") {
		t.Errorf("error = %q, want it to say the model is not trusted", err)
	}
}

func TestEditedModelLosesItsPin(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	writeModel(t, "@test/probe", probeModel)
	trustModel(t, "@test/probe")
	writeModel(t, "@test/probe", probeModel+"\n// changed\n")

	_, err := LoadModel("@test/probe")
	if err == nil {
		t.Fatal("an edited model still ran on its old pin")
	}
	if !strings.Contains(err.Error(), "changed since") {
		t.Errorf("error = %q, want it to say the model changed", err)
	}
}

// The roadmap's acceptance criterion: an invalid argument fails before a single
// step runs.
func TestBadArgumentStopsTheRunBeforeAnyStep(t *testing.T) {
	needsBun(t)
	t.Setenv("DATA_DIR", t.TempDir())
	writeModel(t, "@test/probe", probeModel)
	trustModel(t, "@test/probe")

	f := &Flow{Name: "t", Steps: []Step{
		{Name: "first", Run: "echo this-must-not-run"},
		{Name: "typed", Type: "@test/probe", With: map[string]any{"times": 3.0}},
	}}
	run := Execute(context.Background(), f, Options{})

	if run.Status != StatusUnresolved {
		t.Fatalf("status = %q, want %q", run.Status, StatusUnresolved)
	}
	if len(run.Steps) != 0 {
		t.Fatalf("ran %d steps, want none — preflight must stop the run first", len(run.Steps))
	}
	if !strings.Contains(run.Error, "message") {
		t.Errorf("run error = %q, want it to name the missing argument", run.Error)
	}
}

func TestUnknownArgumentIsRefused(t *testing.T) {
	needsBun(t)
	t.Setenv("DATA_DIR", t.TempDir())
	writeModel(t, "@test/probe", probeModel)
	trustModel(t, "@test/probe")

	f := &Flow{Name: "t", Steps: []Step{
		{Name: "typed", Type: "@test/probe", With: map[string]any{"message": "hi", "mesage": "typo"}},
	}}
	run := Execute(context.Background(), f, Options{})
	if !strings.Contains(run.Error, "mesage") {
		t.Errorf("run error = %q, want it to name the argument that is not accepted", run.Error)
	}
}

func TestEnumArgumentIsChecked(t *testing.T) {
	needsBun(t)
	t.Setenv("DATA_DIR", t.TempDir())
	writeModel(t, "@test/probe", probeModel)
	trustModel(t, "@test/probe")

	f := &Flow{Name: "t", Steps: []Step{
		{Name: "typed", Type: "@test/probe", With: map[string]any{"message": "hi", "mood": "sideways"}},
	}}
	run := Execute(context.Background(), f, Options{})
	if !strings.Contains(run.Error, "loud") {
		t.Errorf("run error = %q, want it to list the allowed values", run.Error)
	}
}

// A typed step runs its model and its JSON output chains like any other stdout.
func TestTypedStepRunsAndChains(t *testing.T) {
	needsBun(t)
	t.Setenv("DATA_DIR", t.TempDir())
	writeModel(t, "@test/probe", probeModel)
	trustModel(t, "@test/probe")

	f := &Flow{Name: "t", Steps: []Step{
		{Name: "typed", Type: "@test/probe", With: map[string]any{"message": "hello"}},
		{Name: "consume", Needs: map[string]string{"OUT": "typed.stdout"}, Run: `printf '%s' "$OUT"`},
	}}
	run := Execute(context.Background(), f, Options{})

	if run.Status != StatusOK {
		t.Fatalf("status = %q, error %q, stderr %q", run.Status, run.Error, run.Steps[0].Stderr)
	}
	if got := run.Steps[1].Stdout; !strings.Contains(got, `"echoed":"hello"`) {
		t.Errorf("the model's output did not chain: %q", got)
	}
}

func TestDescribeReadsTheSchema(t *testing.T) {
	needsBun(t)
	t.Setenv("DATA_DIR", t.TempDir())
	writeModel(t, "@test/probe", probeModel)
	trustModel(t, "@test/probe")

	m, err := LoadModel("@test/probe")
	if err != nil {
		t.Fatal(err)
	}
	schema, err := Describe(context.Background(), m)
	if err != nil {
		t.Fatal(err)
	}
	if schema.Type != "@test/probe" || schema.Version != "1.0.0" {
		t.Errorf("schema = %+v", schema)
	}
	if !schema.Arguments["message"].Required {
		t.Error("message should be required")
	}
}

// A step is a shell command or a model, never both.
func TestStepCannotBeBothShellAndModel(t *testing.T) {
	_, err := Parse("t.yml", []byte("name: t\nsteps:\n  - name: a\n    run: 'true'\n    type: '@test/probe'\n"))
	if err == nil || !strings.Contains(err.Error(), "one or the other") {
		t.Fatalf("error = %v, want a refusal naming both fields", err)
	}
}

func TestModelPinSurvivesPruningDeletedFlows(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	writeModel(t, "@test/probe", probeModel)
	trustModel(t, "@test/probe")

	if _, err := Prune(); err != nil {
		t.Fatal(err)
	}
	pinned, err := TrustedChecksum(modelPin + "@test/probe")
	if err != nil {
		t.Fatal(err)
	}
	if pinned == "" {
		t.Fatal("pruning flow pins removed the model pin")
	}
}
