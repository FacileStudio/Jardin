package mcpserver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FacileStudio/Mycelium/internal/config"
	"github.com/FacileStudio/Mycelium/internal/flow"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// isolate points every path at temporary directories. Nothing in this file may
// reach the real ~/.mycelium: the tests write flow files and trust pins, and
// one of them would execute a step if the gate it checks ever broke.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("DATA_DIR", t.TempDir())
}

// writeFlow puts a one-step flow on disk. The step is "true" so that a broken
// trust gate still runs nothing of consequence.
func writeFlow(t *testing.T, name, step string) {
	t.Helper()
	dir := config.FlowsDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := "name: " + name + "\nsteps:\n  - name: one\n    run: " + step + "\n"
	if err := os.WriteFile(filepath.Join(dir, name+flow.Extension), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// callRunFlow asks for a flow by name and returns the raw result, errors and all.
func callRunFlow(t *testing.T, name string) *mcp.CallToolResult {
	t.Helper()
	res, err := connect(t).CallTool(context.Background(), &mcp.CallToolParams{
		Name: "run_flow", Arguments: map[string]any{"name": name},
	})
	if err != nil {
		t.Fatalf("run_flow returned a protocol error, want a tool execution error: %v", err)
	}
	return res
}

// The refusal a model is most likely to meet. It has to come back as a tool
// execution error, which the client hands to the model, rather than a protocol
// error it never sees — and it has to name the command, because the model
// cannot lift this one itself and needs to know what to ask a human for.
func TestRunFlowRefusesAnUntrustedFlowWithTheCommandThatLiftsTheRefusal(t *testing.T) {
	isolate(t)
	writeFlow(t, "gate", "true")

	res := callRunFlow(t, "gate")
	if !res.IsError {
		t.Fatal("run_flow ran a flow no human pinned on this machine")
	}
	text := resultText(res)
	for _, want := range []string{`"gate" is not trusted on this machine`, "mycelium flow trust gate"} {
		if !strings.Contains(text, want) {
			t.Errorf("refusal = %q, want it to contain %q", text, want)
		}
	}
}

// A pin covers exact content, not a name. Editing a trusted flow, or having it
// edited on another machine and synced here, has to reopen the gate.
func TestRunFlowRefusesAFlowThatChangedSinceItWasPinned(t *testing.T) {
	isolate(t)
	writeFlow(t, "gate", "true")
	pinned, err := flow.Load("gate")
	if err != nil {
		t.Fatal(err)
	}
	if err := flow.Trust(pinned); err != nil {
		t.Fatal(err)
	}
	writeFlow(t, "gate", "true # edited after the pin")

	res := callRunFlow(t, "gate")
	if !res.IsError {
		t.Fatal("run_flow ran a flow that changed after it was approved")
	}
	text := resultText(res)
	for _, want := range []string{"changed since this machine approved it", "mycelium flow trust gate"} {
		if !strings.Contains(text, want) {
			t.Errorf("refusal = %q, want it to contain %q", text, want)
		}
	}
}

// Trust reaches the model as a field it can branch on. Reporting it only inside
// a rendered line would make a model parse a table built for a terminal, and
// the answer decides whether calling run_flow is worth a turn at all.
func TestListFlowsReportsTrustAsAFieldRatherThanAColumnOfText(t *testing.T) {
	isolate(t)
	writeFlow(t, "pinned", "true")
	writeFlow(t, "unpinned", "true")
	f, err := flow.Load("pinned")
	if err != nil {
		t.Fatal(err)
	}
	if err := flow.Trust(f); err != nil {
		t.Fatal(err)
	}

	res, err := connect(t).CallTool(context.Background(), &mcp.CallToolParams{Name: "list_flows"})
	if err != nil {
		t.Fatalf("list_flows: %v", err)
	}
	var out listFlowsOutput
	decode(t, res, &out)

	want := map[string]flowSummary{
		"pinned":   {Name: "pinned", Steps: 1, Trust: trustTrusted, Runnable: true},
		"unpinned": {Name: "unpinned", Steps: 1, Trust: trustNotPinned, Runnable: false},
	}
	if len(out.Flows) != len(want) {
		t.Fatalf("list_flows returned %d flows, want %d", len(out.Flows), len(want))
	}
	for _, got := range out.Flows {
		if got != want[got.Name] {
			t.Errorf("%s = %+v, want %+v", got.Name, got, want[got.Name])
		}
	}
}

// A flow that ran and failed is a result, not an error. The SDK drops
// structured output as soon as a handler returns an error, and the per-step
// exit codes and stderr it would drop are exactly what the model called this
// tool to read.
func TestRunOutputKeepsAFailedRunReadableInsteadOfCollapsingIt(t *testing.T) {
	started := time.Now().UTC()
	run := &flow.Run{
		Flow: "gate", Status: flow.StatusFailed, StartedAt: started,
		FinishedAt: started.Add(2 * time.Second),
		Steps: []flow.StepResult{
			{Name: "one", ExitCode: 0, Stdout: "fine\n"},
			{Name: "two", ExitCode: 3, Stderr: "boom\n"},
		},
	}

	out := runOutput(run, "/tmp/runs/gate/one.json", nil)
	if out.Status != flow.StatusFailed {
		t.Errorf("Status = %q, want %q", out.Status, flow.StatusFailed)
	}
	if len(out.Steps) != 2 || out.Steps[1].ExitCode != 3 || out.Steps[1].Stderr != "boom\n" {
		t.Errorf("Steps = %+v, want both steps with the failure's exit code and stderr", out.Steps)
	}
	if out.DurationMS != 2000 {
		t.Errorf("DurationMS = %d, want 2000", out.DurationMS)
	}
}

// An unwritten artifact must be said out loud. The steps already ran, so
// discarding the record of what they did would be worse than reporting a run
// nobody can look up later.
func TestRunOutputSaysSoWhenTheRunCouldNotBeRecorded(t *testing.T) {
	run := &flow.Run{Flow: "gate", Status: flow.StatusOK, Steps: []flow.StepResult{{Name: "one"}}}

	out := runOutput(run, "/tmp/runs/gate/one.json", errors.New("read-only file system"))
	if out.Artifact != "" {
		t.Errorf("Artifact = %q, want it empty when the write failed", out.Artifact)
	}
	if !strings.Contains(out.Note, "read-only file system") {
		t.Errorf("Note = %q, want it to carry the save failure", out.Note)
	}
}
