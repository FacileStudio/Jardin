package mcpserver

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/FacileStudio/Mycelium/internal/config"
	"github.com/FacileStudio/Mycelium/internal/flow"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// listFlowsInput is empty: the inventory takes no arguments.
type listFlowsInput struct{}

// flowSummary describes one flow. Runnable is trust read back as the question
// the caller actually has, so nobody has to know that "CHANGED" and "not
// pinned" both mean run_flow will refuse.
type flowSummary struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Steps       int    `json:"steps" jsonschema:"how many steps the flow runs"`
	Trust       string `json:"trust" jsonschema:"trusted, not pinned, CHANGED or unknown"`
	Runnable    bool   `json:"runnable" jsonschema:"true when run_flow will execute this flow rather than refuse it"`
}

// listFlowsOutput carries the inventory and, separately, the files it could not
// read.
type listFlowsOutput struct {
	Flows      []flowSummary `json:"flows"`
	Unreadable string        `json:"unreadable,omitempty" jsonschema:"flow files that failed to parse and are absent from the list"`
}

// runFlowInput is a flow name and nothing else. See runFlowTool for why there
// is no second field.
type runFlowInput struct {
	Name string `json:"name" jsonschema:"the flow to run, as reported by list_flows"`
}

// stepOutcome is what one step did. The three booleans separate causes that
// exit code -1 alone would merge into one bucket.
type stepOutcome struct {
	Name       string `json:"name"`
	ExitCode   int    `json:"exit_code"`
	DurationMS int64  `json:"duration_ms"`
	Stdout     string `json:"stdout,omitempty"`
	Stderr     string `json:"stderr,omitempty"`
	TimedOut   bool   `json:"timed_out,omitempty"`
	Skipped    bool   `json:"skipped,omitempty" jsonschema:"the step was fine, something it depends on was not"`
	NotStarted bool   `json:"not_started,omitempty" jsonschema:"the step never ran"`
}

// runFlowOutput is the run record, trimmed to what a model can act on.
type runFlowOutput struct {
	Flow       string        `json:"flow"`
	Status     string        `json:"status" jsonschema:"ok, failed, timeout or unresolved"`
	DurationMS int64         `json:"duration_ms"`
	Artifact   string        `json:"artifact,omitempty" jsonschema:"path of the run record written on this machine"`
	Note       string        `json:"note,omitempty" jsonschema:"why the run never started, or why it could not be recorded"`
	Steps      []stepOutcome `json:"steps"`
}

// listFlows reports every flow with its trust state.
//
// A flow file that fails to parse is named in Unreadable rather than failing
// the call, which is how flow.List already behaves and for the same reason: a
// broken flow must be loud without taking every working flow down with it.
func listFlows(_ context.Context, _ *mcp.CallToolRequest, _ listFlowsInput) (*mcp.CallToolResult, listFlowsOutput, error) {
	flows, listErr := flow.List()
	out := listFlowsOutput{Flows: make([]flowSummary, 0, len(flows))}
	for _, f := range flows {
		state := flow.TrustState(f)
		out.Flows = append(out.Flows, flowSummary{
			Name: f.Name, Description: f.Description, Steps: len(f.Steps),
			Trust: state, Runnable: state == flow.TrustTrusted,
		})
	}
	if listErr != nil {
		out.Unreadable = listErr.Error()
	}
	return nil, out, nil
}

// runFlow executes a pinned flow and returns what each step did.
//
// A flow that ran and failed is a result, not an error: its per-step exit codes
// and stderr are the whole point of calling this, and the SDK drops structured
// output the moment a handler returns an error. Only a refusal to start, which
// carries nothing to read, comes back as a tool execution error.
func runFlow(ctx context.Context, _ *mcp.CallToolRequest, in runFlowInput) (*mcp.CallToolResult, runFlowOutput, error) {
	f, err := flow.Load(strings.TrimSpace(in.Name))
	if err != nil {
		return nil, runFlowOutput{}, err
	}
	trusted, err := flow.IsTrusted(f)
	if err != nil || !trusted {
		return nil, runFlowOutput{}, untrusted(f, err)
	}
	dir, err := os.Getwd()
	if err != nil {
		return nil, runFlowOutput{}, err
	}
	run := flow.Execute(ctx, f, flow.Options{WorkDir: dir, Machine: config.MachineName()})
	path, saveErr := flow.SaveRun(run)
	return nil, runOutput(run, path, saveErr), nil
}

// runOutput converts a run record, folding a failed save into Note. Losing the
// artifact is worth saying and is not worth discarding the run over: the steps
// already happened either way.
func runOutput(r *flow.Run, artifact string, saveErr error) runFlowOutput {
	var notes []string
	if r.Error != "" {
		notes = append(notes, r.Error)
	}
	if saveErr != nil {
		notes = append(notes, "the run was not recorded: "+saveErr.Error())
		artifact = ""
	}
	out := runFlowOutput{
		Flow: r.Flow, Status: r.Status, DurationMS: r.Duration().Milliseconds(),
		Artifact: artifact, Note: strings.Join(notes, "; "),
		Steps: make([]stepOutcome, 0, len(r.Steps)),
	}
	for _, s := range r.Steps {
		out.Steps = append(out.Steps, stepOutcome{
			Name: s.Name, ExitCode: s.ExitCode, DurationMS: s.DurationMS,
			Stdout: s.Stdout, Stderr: s.Stderr, TimedOut: s.TimedOut,
			Skipped: s.Skipped, NotStarted: s.NotStarted,
		})
	}
	return out
}

// untrusted explains a refusal in the terms that lift it. cause is whatever
// stopped the trust check, or nil when the check simply said no.
//
// It is an ordinary Go error, which the SDK packs into a CallToolResult with
// IsError set: a tool execution error handed back to the model, not a protocol
// error that stops at the client. The spec draws that line so the model can
// correct itself, and here it cannot, because only a human at a terminal can
// pin a flow. So the text names the exact command and says who has to run it.

// runFlowByName returns a handler that runs a specific flow by name, without
// requiring the caller to pass the name as an argument. Each runtime tool
// generated by flowToTool calls this, so an agent never reaches a flow it
// cannot name.
func runFlowByName(name string) func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, runFlowOutput, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, runFlowOutput, error) {
		return runFlow(ctx, nil, runFlowInput{Name: name})
	}
}

func untrusted(f *flow.Flow, cause error) error {
	pinned, err := flow.TrustedChecksum(f.Name)
	if cause == nil {
		cause = err
	}
	if cause != nil {
		return fmt.Errorf("flow %q cannot run: this machine's trust pins are unreadable (%v); "+
			"a human must repair %s", f.Name, cause, flow.TrustPath())
	}
	if pinned == "" {
		return fmt.Errorf("flow %q is not trusted on this machine; a human must read it and run: "+
			"mycelium flow trust %s", f.Name, f.Name)
	}
	return fmt.Errorf("flow %q changed since this machine approved it (approved %s, now %s); "+
		"a human must read it again and run: mycelium flow trust %s", f.Name, pinned, f.Checksum, f.Name)
}
