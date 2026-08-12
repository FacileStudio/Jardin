package adapter

import (
	"fmt"

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

// Available returns the registered adapter names.
func Available() string {
	var names []string
	for name := range registry {
		names = append(names, name)
	}
	return fmt.Sprintf("%v", names)
}

// All returns the registry itself.
func All() map[string]Adapter {
	return registry
}
