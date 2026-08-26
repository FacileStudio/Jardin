package adapter

import (
	"strings"

	"github.com/FacileStudio/Mycelium/internal/cell"
)

const (
	fenceOpenPrefix = "<!-- agent:"
	fenceOpenSuffix = " -->"
	fenceClose      = "<!-- /agent -->"
)

const (
	audienceMCP = "mcp"
	audienceCLI = "cli"
)

// audienceFor names the fence selector a rule renders for.
//
// Two audiences and no more: an agent that can call mycelium's MCP tools, and
// one that can only shell out to the binary. The distinction exists because a
// rule telling a model to both call search_memory and run `mycelium memory
// search` teaches it the tool is optional, which is how the start gate got
// skipped in the first place.
func audienceFor(mcpTools bool) string {
	if mcpTools {
		return audienceMCP
	}
	return audienceCLI
}

// fenceSelector extracts the audience from an opening marker line.
func fenceSelector(line string) (string, bool) {
	if !strings.HasPrefix(line, fenceOpenPrefix) || !strings.HasSuffix(line, fenceOpenSuffix) {
		return "", false
	}
	body := line[len(fenceOpenPrefix) : len(line)-len(fenceOpenSuffix)]
	return strings.TrimSpace(body), true
}

// knownAudience reports whether a selector is one this renderer understands.
func knownAudience(selector string) bool {
	return selector == audienceMCP || selector == audienceCLI
}

// renderFences strips the conditional markers from a rule body and keeps only
// the passages addressed to the given audience.
//
// The markers are HTML comments in the shape the corpus already uses for
// `<!-- lang:fr -->`: a rule file stays valid markdown, renders unchanged in
// an editor, and nobody has a second syntax to learn. An opening
// `<!-- agent:mcp -->` or `<!-- agent:cli -->` runs until `<!-- /agent -->`
// or the end of the file, and the markers themselves never reach the output.
//
// An unrecognised selector keeps its passage rather than dropping it. A typo
// must not silently delete an instruction from every generated config: a rule
// the model reads twice is noise it can survive, a rule it never reads is a
// behaviour change with nothing to grep for.
func renderFences(content, audience string) string {
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	keep, dropped := true, false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if selector, ok := fenceSelector(trimmed); ok {
			keep = selector == audience || !knownAudience(selector)
			dropped = true
			continue
		}
		if trimmed == fenceClose {
			keep, dropped = true, true
			continue
		}
		if !keep {
			dropped = true
			continue
		}
		if line == "" && dropped && (len(out) == 0 || out[len(out)-1] == "") {
			continue
		}
		out = append(out, line)
		dropped = false
	}

	return strings.Join(out, "\n")
}

// renderRule returns one rule's body rendered for this input's audience,
// trimmed and ready to drop into a generated file.
//
// The unit is one rule because cursor writes one file per rule and everyone
// else writes them joined. Handing cursor a slice to index into the rule list
// with worked only while the two stayed the same length, which is a property
// nothing states and nothing checks.
func renderRule(input Input, rule cell.NamedFile) string {
	return strings.TrimSpace(renderFences(rule.Content, audienceFor(input.MCPTools)))
}

// ruleSections returns every rule body rendered for this input's audience,
// ready to join into a generated file.
//
// Shared rather than repeated per adapter: eight adapters loop over
// input.Rules, and a fence honoured by seven of them is a rule that reaches
// one agent in both forms with nothing failing.
func ruleSections(input Input) []string {
	out := make([]string, 0, len(input.Rules))
	for _, rule := range input.Rules {
		out = append(out, renderRule(input, rule))
	}
	return out
}
