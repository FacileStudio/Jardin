package server

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/FacileStudio/Jardin/internal/flow"
	apierrors "github.com/FacileStudio/tronc/errors"
	"github.com/FacileStudio/tronc/httpjson"
)

// FlowStepSummary is one step of a parsed flow, as shown in the dashboard.
type FlowStepSummary struct {
	Name      string            `json:"name"`
	Kind      string            `json:"kind"`
	Type      string            `json:"type,omitempty"`
	DependsOn []string          `json:"depends_on,omitempty"`
	Needs     map[string]string `json:"needs,omitempty"`
}

// FlowSummary is a flow's structure, derived from its YAML rather than hand
// re-parsed by the client.
type FlowSummary struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Steps       []FlowStepSummary `json:"steps"`
}

// FlowDetail is what /flows/{name} answers: the raw file plus its parsed
// structure. A flow that no longer parses still has to render — trust and run
// history are per-machine and never reach the server, but a broken flow is a
// thing an author still needs to see and fix.
type FlowDetail struct {
	Raw        string       `json:"raw"`
	Summary    *FlowSummary `json:"summary,omitempty"`
	ParseError string       `json:"parse_error,omitempty"`
}

func (s *Server) flowsList(w http.ResponseWriter, r *http.Request) {
	root, ok := s.scopeRoot(w, r)
	if !ok {
		return
	}
	httpjson.WriteJSON(w, http.StatusOK, listNamesWithExt(filepath.Join(root, "flows"), flow.Extension))
}

func (s *Server) flowGet(w http.ResponseWriter, r *http.Request) {
	name, ok := safeName(pathParam(r, "name"))
	if !ok {
		httpjson.WriteError(w, apierrors.Invalid("invalid name"))
		return
	}
	root, rootOK := s.scopeRoot(w, r)
	if !rootOK {
		return
	}
	path := filepath.Join(root, "flows", name+flow.Extension)
	data, err := os.ReadFile(path)
	if err != nil {
		httpjson.WriteError(w, apierrors.NotFound("not found"))
		return
	}
	detail := FlowDetail{Raw: string(data)}
	parsed, err := flow.Parse(path, data)
	if err != nil {
		detail.ParseError = err.Error()
	} else {
		detail.Summary = summarizeFlow(parsed)
	}
	httpjson.WriteJSON(w, http.StatusOK, detail)
}

func summarizeFlow(f *flow.Flow) *FlowSummary {
	steps := make([]FlowStepSummary, 0, len(f.Steps))
	for _, step := range f.Steps {
		kind := "run"
		if step.Type != "" {
			kind = "type"
		}
		steps = append(steps, FlowStepSummary{
			Name:      step.Name,
			Kind:      kind,
			Type:      step.Type,
			DependsOn: step.DependsOn,
			Needs:     step.Needs,
		})
	}
	return &FlowSummary{Name: f.Name, Description: f.Description, Steps: steps}
}
