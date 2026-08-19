package flow

import "fmt"

// dependencies returns, for every step, the steps it waits on.
//
// A step that declares no depends_on inherits the dependency its position
// already implied: the step written before it. That is what makes parallelism
// opt-in — every flow written before this existed keeps running in file order,
// one step at a time. Declaring depends_on (even as an empty list) replaces
// that with exactly the edges asked for.
//
// A value dependency is a dependency: needing an earlier step's output is an
// edge whether or not depends_on repeats it.
func dependencies(f *Flow) map[string][]string {
	deps := make(map[string][]string, len(f.Steps))
	for i, step := range f.Steps {
		waits := make(map[string]string, len(step.DependsOn)+len(step.Needs))
		if step.DependsOn == nil {
			if i > 0 {
				waits[f.Steps[i-1].Name] = ""
			}
		}
		for _, name := range step.DependsOn {
			waits[name] = ""
		}
		for _, ref := range step.Needs {
			if parsed, err := parseReference(ref); err == nil {
				waits[parsed.Step] = ""
			}
		}
		delete(waits, step.Name)
		deps[step.Name] = sortedNames(waits)
	}
	return deps
}

// validateGraph refuses a flow whose steps cannot all run: a dependency on a
// step that does not exist, a step waiting on itself, or a cycle.
func validateGraph(f *Flow) error {
	known := make(map[string]bool, len(f.Steps))
	for _, step := range f.Steps {
		known[step.Name] = true
	}
	for _, step := range f.Steps {
		for _, name := range step.DependsOn {
			if name == step.Name {
				return fmt.Errorf("step %q depends on itself", step.Name)
			}
			if !known[name] {
				return fmt.Errorf("step %q depends on %q, which is not a step in this flow", step.Name, name)
			}
		}
	}
	return findCycle(f)
}

// findCycle walks the graph depth first and names the loop it closes, so the
// error points at the steps to edit rather than announcing that one exists.
func findCycle(f *Flow) error {
	deps := dependencies(f)
	const (
		unvisited = 0
		onStack   = 1
		settled   = 2
	)
	state := make(map[string]int, len(f.Steps))
	var path []string

	var walk func(name string) error
	walk = func(name string) error {
		state[name] = onStack
		path = append(path, name)
		for _, next := range deps[name] {
			switch state[next] {
			case onStack:
				return fmt.Errorf("steps %s form a cycle and none of them could run", cycleFrom(path, next))
			case unvisited:
				if err := walk(next); err != nil {
					return err
				}
			}
		}
		path = path[:len(path)-1]
		state[name] = settled
		return nil
	}

	for _, step := range f.Steps {
		if state[step.Name] != unvisited {
			continue
		}
		if err := walk(step.Name); err != nil {
			return err
		}
	}
	return nil
}

func cycleFrom(path []string, start string) string {
	from := 0
	for i, name := range path {
		if name == start {
			from = i
			break
		}
	}
	loop := append(append([]string{}, path[from:]...), start)
	out := ""
	for i, name := range loop {
		if i > 0 {
			out += " → "
		}
		out += fmt.Sprintf("%q", name)
	}
	return out
}
