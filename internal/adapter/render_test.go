package adapter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FacileStudio/Mycelium/internal/cell"
)

const fencedRule = `## Operating loop

Search before rediscovering.

<!-- agent:mcp -->
Call ` + "`search_memory`" + ` with the keywords.
<!-- /agent -->
<!-- agent:cli -->
Run ` + "`mycelium memory search \"<keywords>\"`" + `.
<!-- /agent -->

Open only the 1-3 most relevant pages.
`

// fixtureRule writes the fenced rule to a temp file and reads it back, so the
// renderer is exercised against bytes off disk rather than a string literal
// the compiler already normalised. The real rules live in ~/.mycelium/rules
// and are never touched by a test.
func fixtureRule(t *testing.T) cell.NamedFile {
	t.Helper()
	path := filepath.Join(t.TempDir(), "20-memory.md")
	if err := os.WriteFile(path, []byte(fencedRule), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return cell.NamedFile{Name: "20-memory", Content: string(data)}
}

// TestAToolCapableAgentIsToldToCallTheToolAndNeverTheCommand is the whole
// point of the mechanism: the two instructions are mutually exclusive, and a
// model shown both learns that the tool is optional.
func TestAToolCapableAgentIsToldToCallTheToolAndNeverTheCommand(t *testing.T) {
	got := ruleSections(Input{Rules: []cell.NamedFile{fixtureRule(t)}, MCPTools: true})[0]

	if !strings.Contains(got, "search_memory") {
		t.Errorf("tool-capable agent lost the tool instruction:\n%s", got)
	}
	if strings.Contains(got, "mycelium memory search") {
		t.Errorf("tool-capable agent was also given the command form:\n%s", got)
	}
}

// TestACLIOnlyAgentIsToldToRunTheCommandAndNeverTheTool is the other
// direction, and it is the default: MCPTools is the zero value, so an adapter
// that declares no server keeps generating what it generated before.
func TestACLIOnlyAgentIsToldToRunTheCommandAndNeverTheTool(t *testing.T) {
	got := ruleSections(Input{Rules: []cell.NamedFile{fixtureRule(t)}})[0]

	if !strings.Contains(got, "mycelium memory search") {
		t.Errorf("CLI-only agent lost the command instruction:\n%s", got)
	}
	if strings.Contains(got, "search_memory") {
		t.Errorf("CLI-only agent was also given the tool form:\n%s", got)
	}
}

// TestFenceMarkersNeverReachTheGeneratedFile guards the half that is easy to
// forget: a marker left in the output is markdown the model reads as content.
func TestFenceMarkersNeverReachTheGeneratedFile(t *testing.T) {
	for _, mcp := range []bool{true, false} {
		got := ruleSections(Input{Rules: []cell.NamedFile{fixtureRule(t)}, MCPTools: mcp})[0]
		if strings.Contains(got, "<!-- agent:") || strings.Contains(got, fenceClose) {
			t.Errorf("MCPTools=%v left a marker in the output:\n%s", mcp, got)
		}
		if strings.Contains(got, "\n\n\n") {
			t.Errorf("MCPTools=%v left a hole where the other branch was:\n%q", mcp, got)
		}
		if !strings.Contains(got, "Open only the 1-3 most relevant pages.") {
			t.Errorf("MCPTools=%v dropped unfenced text:\n%s", mcp, got)
		}
	}
}

// TestAnUnknownSelectorKeepsItsPassage pins the fail-open choice. A typo that
// deletes an instruction from every generated config leaves nothing to grep
// for; one that shows it to both audiences is noise anybody can see.
func TestAnUnknownSelectorKeepsItsPassage(t *testing.T) {
	got := renderFences("<!-- agent:codex -->\nkeep me\n<!-- /agent -->", audienceCLI)
	if got != "keep me" {
		t.Errorf("renderFences dropped an unknown selector's passage: %q", got)
	}
}

// TestUnfencedRuleTextSurvivesByteForByte is the regression that matters for
// the other seven adapters: the renderer now sits in every Generate, so rule
// text with no markers must come out exactly as it went in, double blank
// lines and all.
func TestUnfencedRuleTextSurvivesByteForByte(t *testing.T) {
	body := "# Rules\n\nOne.\n\n\nTwo, after a deliberate gap.\n"
	if got := renderFences(body, audienceMCP); got != body {
		t.Errorf("renderFences rewrote unfenced text:\ngot  %q\nwant %q", got, body)
	}
}
