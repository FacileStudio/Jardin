package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/FacileStudio/Jardin/internal/config"
)

const (
	// modelPin namespaces a model's trust pin so it cannot collide with a flow
	// of the same name, and so Prune knows which file to look for.
	modelPin = "model:"
	// modelExt is the one extension a model may have. Models are TypeScript run
	// by bun, which is the extension format the core does not have to become.
	modelExt = ".ts"
	// modelRuntime executes a model. It is not a dependency of jardin: a flow
	// that declares no type never looks for it.
	modelRuntime = "bun"
	// describeTimeout bounds the preflight call. Every step has a timeout; the
	// phase that runs model code before any step had none, so a model that hung
	// in describe hung the whole run with nothing left to stop it.
	describeTimeout = 30 * time.Second
)

// Model is a typed step implementation resolved on this machine.
type Model struct {
	Type     string
	Path     string
	Checksum string
}

// Schema is what a model says about itself. jardin never reads the TypeScript;
// it reads this, which is the point of the split — an agent writing a flow
// needs the arguments, not the implementation.
type Schema struct {
	Type      string              `json:"type"`
	Version   string              `json:"version"`
	Arguments map[string]Argument `json:"arguments"`
	Outputs   []string            `json:"outputs"`
}

// Argument declares one input a model accepts.
type Argument struct {
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Enum     []string `json:"enum,omitempty"`
}

// ModelPath resolves a type name to a file under the models directory. The name
// is a path fragment, so it is checked for escaping rather than trusted: a type
// of "../../.ssh/id_rsa" must not resolve.
func ModelPath(typeName string) (string, error) {
	clean := strings.TrimPrefix(typeName, "@")
	if clean == "" || strings.HasPrefix(clean, "/") || strings.Contains(clean, "..") {
		return "", fmt.Errorf("%q is not a usable model type", typeName)
	}
	root := config.ModelsDir()
	full := filepath.Join(root, filepath.FromSlash(clean)+modelExt)
	if !strings.HasPrefix(full, root+string(filepath.Separator)) {
		return "", fmt.Errorf("model type %q resolves outside %s", typeName, root)
	}
	return full, nil
}

// LoadModel resolves a type and refuses one this machine has not approved. A
// model is code that arrives over sync, so it is pinned exactly like a flow —
// distributing prose an agent reads and distributing code a machine runs are
// not the same risk.
func LoadModel(typeName string) (*Model, error) {
	path, err := ModelPath(typeName)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no model %q on this machine (looked in %s)", typeName, config.ModelsDir())
		}
		return nil, err
	}
	m := &Model{Type: typeName, Path: path, Checksum: Checksum(data)}
	pinned, err := TrustedChecksum(modelPin + typeName)
	if err != nil {
		return nil, err
	}
	if pinned == "" {
		return nil, fmt.Errorf("model %q is not trusted on this machine; read %s then run: jardin flow trust-model %s",
			typeName, path, typeName)
	}
	if pinned != m.Checksum {
		return nil, fmt.Errorf("model %q changed since it was approved here; read %s then re-approve it", typeName, path)
	}
	return m, nil
}

// TrustModel pins a model's current bytes on this machine.
func TrustModel(m *Model) error {
	pins, err := readTrust()
	if err != nil {
		return err
	}
	pins[modelPin+m.Type] = m.Checksum
	return writeTrust(pins)
}

// Describe asks a model what it accepts. It runs the model with one argument
// and reads JSON back, so adding a model needs no jardin release.
func Describe(ctx context.Context, m *Model) (*Schema, error) {
	ctx, cancel := context.WithTimeout(ctx, describeTimeout)
	defer cancel()
	out, err := runtimeOutput(ctx, m, "describe", nil)
	if err != nil {
		return nil, err
	}
	var schema Schema
	if err := json.Unmarshal(out, &schema); err != nil {
		return nil, fmt.Errorf("model %q did not describe itself as JSON: %w", m.Type, err)
	}
	return &schema, nil
}

// ValidateArguments checks a step's arguments against the model's schema.
// Unknown arguments are refused rather than ignored, for the same reason
// unknown YAML fields are: a typo that is quietly dropped is worse than one
// that stops the run.
func (s *Schema) ValidateArguments(stepName string, with map[string]any) error {
	for name, arg := range s.Arguments {
		value, given := with[name]
		if !given {
			if arg.Required {
				return fmt.Errorf("step %q is missing the required argument %q", stepName, name)
			}
			continue
		}
		if err := arg.check(name, value); err != nil {
			return fmt.Errorf("step %q: %w", stepName, err)
		}
	}
	for name := range with {
		if _, known := s.Arguments[name]; !known {
			return fmt.Errorf("step %q passes %q, which %s does not accept", stepName, name, s.Type)
		}
	}
	return nil
}

func (a Argument) check(name string, value any) error {
	switch a.Type {
	case "string":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("%q must be a string", name)
		}
		return a.checkEnum(name, text)
	case "number":
		if _, ok := value.(float64); !ok {
			if _, isInt := value.(int); !isInt {
				return fmt.Errorf("%q must be a number", name)
			}
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%q must be true or false", name)
		}
	}
	return nil
}

func (a Argument) checkEnum(name, value string) error {
	if len(a.Enum) == 0 {
		return nil
	}
	for _, allowed := range a.Enum {
		if value == allowed {
			return nil
		}
	}
	return fmt.Errorf("%q must be one of %s", name, strings.Join(a.Enum, ", "))
}

// runtimeOutput executes a model verb and returns its stdout.
func runtimeOutput(ctx context.Context, m *Model, verb string, input []byte) ([]byte, error) {
	if _, err := exec.LookPath(modelRuntime); err != nil {
		return nil, fmt.Errorf("model %q needs %s on PATH, which this machine does not have", m.Type, modelRuntime)
	}
	cmd := exec.CommandContext(ctx, modelRuntime, m.Path, verb)
	if input != nil {
		cmd.Stdin = strings.NewReader(string(input))
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("model %q failed to %s: %w: %s", m.Type, verb, err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// preflight resolves and checks every model a flow uses before the run starts.
// This is the whole point of typing a step: an argument that is missing or
// misspelled stops the flow while nothing has happened yet, rather than three
// steps in, after the side effects.
func preflight(ctx context.Context, f *Flow) (map[string]*Model, error) {
	models := make(map[string]*Model)
	for _, step := range f.Steps {
		if step.Type == "" || models[step.Type] != nil {
			continue
		}
		m, err := resolveModel(ctx, f, step.Type)
		if err != nil {
			return nil, err
		}
		models[step.Type] = m
	}
	return models, nil
}

// resolveModel loads one model, asks it what it accepts, and checks every step
// that uses it against the answer.
func resolveModel(ctx context.Context, f *Flow, typeName string) (*Model, error) {
	m, err := LoadModel(typeName)
	if err != nil {
		return nil, err
	}
	schema, err := Describe(ctx, m)
	if err != nil {
		return nil, err
	}
	for _, step := range f.Steps {
		if step.Type != typeName {
			continue
		}
		if err := schema.ValidateArguments(step.Name, step.With); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// modelInput is the JSON a model reads on stdin.
type modelInput struct {
	Arguments map[string]any    `json:"arguments"`
	Env       map[string]string `json:"env,omitempty"`
}

// modelCommand builds the process for a typed step. Arguments travel as JSON on
// stdin rather than as flags, so a value containing a quote, a newline or a
// leading dash is data the whole way down.
//
// A step with no arguments still sends an empty object. A nil map marshals to
// null, and a model that reads a field off it crashes before it runs a line of
// its own logic — the payload has to match the shape every model is written
// against, not the shape the Go value happens to have.
func modelCommand(ctx context.Context, m *Model, step Step, env map[string]string) (*exec.Cmd, error) {
	args := step.With
	if args == nil {
		args = map[string]any{}
	}
	payload, err := json.Marshal(modelInput{Arguments: args, Env: env})
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, modelRuntime, m.Path, "execute")
	cmd.Stdin = strings.NewReader(string(payload))
	return cmd, nil
}
