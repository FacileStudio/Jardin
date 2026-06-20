package adapter

import (
	"fmt"

	"github.com/FacileStudio/Hive/internal/cell"
)

type Input struct {
	Rules       []cell.NamedFile
	Skills      []cell.NamedFile
	Machine     string
	CellName    string
	MachineName string
}

type Output struct {
	Files map[string]string
}

type Adapter interface {
	Name() string
	Generate(input Input) (*Output, error)
	TargetPaths() []string
}

var registry = map[string]Adapter{}

func Register(a Adapter) {
	registry[a.Name()] = a
}

func Get(name string) (Adapter, error) {
	a, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown adapter: %q (available: %s)", name, Available())
	}
	return a, nil
}

func Available() string {
	var names []string
	for name := range registry {
		names = append(names, name)
	}
	return fmt.Sprintf("%v", names)
}

func All() map[string]Adapter {
	return registry
}
