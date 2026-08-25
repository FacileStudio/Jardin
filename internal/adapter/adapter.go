package adapter

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/FacileStudio/Mycelium/internal/cell"
)

// Input is what an adapter turns into machine config: the rule and skill
// files that apply, and the identity of the machine they are being generated
// for.
type Input struct {
	Rules       []cell.NamedFile
	Skills      []cell.NamedFile
	Machine     string
	MachineName string

	// MCPTools reports whether the agent being generated for can call
	// mycelium's MCP tools instead of shelling out to the binary.
	//
	// The adapter is the authority on this, not the caller: whether
	// search_memory reaches the model is a property of the assistant's
	// config format, which is exactly what an adapter knows and install
	// does not. It sets the field on its own copy before rendering.
	//
	// The zero value is the CLI-only agent, so an adapter that declares no
	// MCP server keeps generating what it generated before.
	MCPTools bool
}

// Output is the generated config keyed by absolute target path.
type Output struct {
	Files map[string]string
}

// Adapter is a code-assistant the tool can generate config for.
type Adapter interface {
	Name() string
	Generate(input Input) (*Output, error)
	TargetPaths() []string
}

// HookInstaller is an adapter that can also install its own commit hooks.
type HookInstaller interface {
	InstallHooks() (string, []string, error)
}

var registry = map[string]Adapter{}

// Register adds an adapter to the global registry.
func Register(a Adapter) {
	registry[a.Name()] = a
}

// Get returns the adapter with the given name.
func Get(name string) (Adapter, error) {
	a, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown adapter: %q (available: %s)", name, Available())
	}
	return a, nil
}

// Names returns every registered adapter name, sorted.
//
// Sorted rather than left in registry order because Go randomises map
// iteration per run. Without this the "available" list in Get's error, the
// list in `install --help` and the order `install --all` writes its files all
// shuffle between invocations, which reads as instability in the tool rather
// than as the deliberate non-determinism it actually is.
func Names() []string {
	return slices.Sorted(maps.Keys(registry))
}

// Available returns the registered adapter names as one display string.
func Available() string {
	return strings.Join(Names(), ", ")
}

// All returns every registered adapter, in Names order.
//
// A slice rather than the registry map, and the difference is the point: a
// map hands the caller Go's randomised iteration, which is what made
// `install --all` write its files in a different order every run. Returning
// the sequence already sorted means a caller cannot reintroduce that by
// accident, and leaves no key lookup to get wrong — an absent key would
// yield a nil Adapter and panic on first use.
func All() []Adapter {
	names := Names()
	out := make([]Adapter, 0, len(names))
	for _, name := range names {
		out = append(out, registry[name])
	}
	return out
}
