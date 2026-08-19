package flow

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// The fields a step exposes to the steps that follow it.
const (
	fieldStdout   = "stdout"
	fieldStderr   = "stderr"
	fieldExitCode = "exit_code"
)

// reference is a parsed "<step>.<field>" pointer at an earlier step's output.
type reference struct {
	Step  string
	Field string
}

// output is one step's raw result. It lives only for the length of a run: the
// artifact stores the redacted form, and a later step must receive the real
// value rather than a string of asterisks.
type output struct {
	Stdout    string
	Stderr    string
	ExitCode  int
	StdoutCut bool
	StderrCut bool
	// Ephemeral marks output its step asked to keep off disk. It travels with
	// the value so a later step cannot undo that by consuming it.
	Ephemeral bool
	// Secrets holds what the producing step declared sensitive. A consumer
	// receives raw output, so a credential echoed into it would land in the
	// consumer's artifact unmasked without this.
	Secrets []string
}

// parseReference splits "<step>.<field>", cutting at the last dot so a step
// whose own name contains one still resolves.
func parseReference(ref string) (reference, error) {
	i := strings.LastIndex(ref, ".")
	if i <= 0 || i == len(ref)-1 {
		return reference{}, fmt.Errorf("%q is not a <step>.<field> reference", ref)
	}
	parsed := reference{Step: ref[:i], Field: ref[i+1:]}
	switch parsed.Field {
	case fieldStdout, fieldStderr, fieldExitCode:
		return parsed, nil
	}
	return reference{}, fmt.Errorf("%q asks for %q, but a step exposes only %s, %s and %s",
		ref, parsed.Field, fieldStdout, fieldStderr, fieldExitCode)
}

// resolve reads the values a step needs out of the steps that already ran. The
// caller merges the result into the step's environment; nothing is ever spliced
// into the command string.
//
// Names are visited in sorted order so a step with two bad references always
// reports the same one first.
func resolve(step Step, outputs map[string]output) (map[string]string, []string, error) {
	if len(step.Needs) == 0 {
		return nil, nil, nil
	}
	values := make(map[string]string, len(step.Needs))
	var secret []string
	total := 0
	for _, name := range sortedNames(step.Needs) {
		value, err := resolveOne(step, step.Needs[name], outputs)
		if err != nil {
			return nil, nil, err
		}
		total += len(name) + len(value)
		if total > MaxTotalValueBytes {
			return nil, nil, fmt.Errorf(
				"step %q needs %d bytes of values, over the %d-byte total; the environment it builds would not survive exec",
				step.Name, total, MaxTotalValueBytes)
		}
		values[name] = value
		secret = append(secret, inheritedSecrets(step.Needs[name], outputs, value)...)
	}
	return values, secret, nil
}

// inheritedSecrets reports what must stay masked in the step consuming this
// reference: the value itself when its source kept output off disk, and
// whatever that source declared sensitive. Suppressing only at the source is
// not enough — the value would come straight back through whatever consumed it.
func inheritedSecrets(ref string, outputs map[string]output, value string) []string {
	parsed, err := parseReference(ref)
	if err != nil {
		return nil
	}
	src := outputs[parsed.Step]
	out := append([]string{}, src.Secrets...)
	if src.Ephemeral {
		out = append(out, value)
	}
	return out
}

func resolveOne(step Step, ref string, outputs map[string]output) (string, error) {
	parsed, err := parseReference(ref)
	if err != nil {
		return "", err
	}
	out, ran := outputs[parsed.Step]
	if !ran {
		return "", fmt.Errorf("step %q needs %q, which did not run", step.Name, parsed.Step)
	}
	value, err := out.field(parsed.Field)
	if err != nil {
		return "", fmt.Errorf("step %q needs %s: %w", step.Name, ref, err)
	}
	return value, nil
}

// field returns one output field as a string. A truncated stream is an error
// rather than a shortened value: passing on the first megabyte of something as
// if it were the whole thing corrupts the run quietly.
func (o output) field(name string) (string, error) {
	switch name {
	case fieldStdout:
		return streamValue(o.Stdout, o.StdoutCut, fieldStdout)
	case fieldStderr:
		return streamValue(o.Stderr, o.StderrCut, fieldStderr)
	case fieldExitCode:
		return strconv.Itoa(o.ExitCode), nil
	}
	return "", fmt.Errorf("unknown field %q", name)
}

// streamValue prepares a captured stream for the next step. It drops trailing
// line endings, the way "$(...)" does in a shell, and leaves the rest of the
// value untouched.
func streamValue(value string, cut bool, stream string) (string, error) {
	if cut {
		return "", fmt.Errorf("%s was truncated at %d bytes, so it is not the whole value", stream, MaxStreamBytes)
	}
	value = strings.TrimRight(value, "\r\n")
	if strings.ContainsRune(value, 0) {
		return "", fmt.Errorf("%s contains a NUL byte, which cannot travel in an environment variable", stream)
	}
	if len(value) > MaxValueBytes {
		return "", fmt.Errorf(
			"%s is %d bytes, over the %d-byte limit for a chained value; write it to a file and pass the path instead",
			stream, len(value), MaxValueBytes)
	}
	return value, nil
}

// referencedSteps returns the steps whose output some later step asks for.
// Nothing else needs to be kept in memory once it has run.
func referencedSteps(f *Flow) map[string]bool {
	referenced := make(map[string]bool)
	for _, step := range f.Steps {
		for _, ref := range step.Needs {
			if parsed, err := parseReference(ref); err == nil {
				referenced[parsed.Step] = true
			}
		}
	}
	return referenced
}

// stepEnvOf merges a step's declared environment with the values resolved from
// earlier steps. The two cannot collide: validateNeeds refuses a flow where a
// name appears in both.
func stepEnvOf(step Step, resolved map[string]string) map[string]string {
	if len(resolved) == 0 {
		return step.Env
	}
	merged := make(map[string]string, len(step.Env)+len(resolved))
	for name, value := range step.Env {
		merged[name] = value
	}
	for name, value := range resolved {
		merged[name] = value
	}
	return merged
}

func redactMap(values map[string]string, redact func(string) string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for name, value := range values {
		out[name] = redact(value)
	}
	return out
}

func sortedNames(m map[string]string) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// validateNeeds checks one step's references against every step in the flow.
// Needing an output is a dependency, so the graph orders the two steps and the
// cycle check refuses the case that cannot be ordered at all.
func validateNeeds(step Step, known map[string]bool) error {
	for _, name := range sortedNames(step.Needs) {
		if err := validateNeed(step, name, known); err != nil {
			return err
		}
	}
	return nil
}

func validateNeed(step Step, name string, known map[string]bool) error {
	if err := validEnvName(name); err != nil {
		return fmt.Errorf("step %q: %w", step.Name, err)
	}
	if name == tokenEnvVar {
		return fmt.Errorf("step %q may not bind %s", step.Name, tokenEnvVar)
	}
	if _, clash := step.Env[name]; clash {
		return fmt.Errorf("step %q sets %s in both env and needs", step.Name, name)
	}
	parsed, err := parseReference(step.Needs[name])
	if err != nil {
		return fmt.Errorf("step %q: %w", step.Name, err)
	}
	if parsed.Step == step.Name {
		return fmt.Errorf("step %q needs its own output", step.Name)
	}
	if !known[parsed.Step] {
		return fmt.Errorf("step %q needs %q, which is not a step in this flow", step.Name, parsed.Step)
	}
	return nil
}

// validEnvName accepts what a POSIX shell will let a step read back as $NAME.
func validEnvName(name string) error {
	if name == "" {
		return fmt.Errorf("a need with no name cannot reach a step")
	}
	for i, r := range name {
		letter := r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		digit := r >= '0' && r <= '9'
		if letter || (digit && i > 0) {
			continue
		}
		return fmt.Errorf("%q is not a usable environment variable name", name)
	}
	return nil
}
