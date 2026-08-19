package flow

import (
	"fmt"
	"strconv"
	"strings"
)

// The fields a step exposes to the steps that follow it.
const (
	FieldStdout   = "stdout"
	FieldStderr   = "stderr"
	FieldExitCode = "exit_code"
)

// Reference is a parsed "<step>.<field>" pointer at an earlier step's output.
type Reference struct {
	Step  string
	Field string
}

// Output is one step's raw result. It lives only for the length of a run: the
// artifact stores the redacted form, and a later step must receive the real
// value rather than a string of asterisks.
type Output struct {
	Stdout    string
	Stderr    string
	ExitCode  int
	StdoutCut bool
	StderrCut bool
}

// ParseReference splits "<step>.<field>", cutting at the last dot so a step
// whose own name contains one still resolves.
func ParseReference(ref string) (Reference, error) {
	i := strings.LastIndex(ref, ".")
	if i <= 0 || i == len(ref)-1 {
		return Reference{}, fmt.Errorf("%q is not a <step>.<field> reference", ref)
	}
	parsed := Reference{Step: ref[:i], Field: ref[i+1:]}
	switch parsed.Field {
	case FieldStdout, FieldStderr, FieldExitCode:
		return parsed, nil
	}
	return Reference{}, fmt.Errorf("%q asks for %q, but a step exposes only %s, %s and %s",
		ref, parsed.Field, FieldStdout, FieldStderr, FieldExitCode)
}

// Resolve reads the values a step needs out of the steps that already ran. The
// caller merges the result into the step's environment; nothing is ever spliced
// into the command string.
func Resolve(step Step, outputs map[string]Output) (map[string]string, error) {
	if len(step.Needs) == 0 {
		return nil, nil
	}
	values := make(map[string]string, len(step.Needs))
	for name, ref := range step.Needs {
		parsed, err := ParseReference(ref)
		if err != nil {
			return nil, err
		}
		out, ran := outputs[parsed.Step]
		if !ran {
			return nil, fmt.Errorf("step %q needs %q, which did not run", step.Name, parsed.Step)
		}
		value, err := out.field(parsed.Field)
		if err != nil {
			return nil, fmt.Errorf("step %q needs %s: %w", step.Name, ref, err)
		}
		values[name] = value
	}
	return values, nil
}

// field returns one output field as a string. A truncated stream is an error
// rather than a shortened value: passing on the first megabyte of something as
// if it were the whole thing corrupts the run quietly.
func (o Output) field(name string) (string, error) {
	switch name {
	case FieldStdout:
		return streamValue(o.Stdout, o.StdoutCut, FieldStdout)
	case FieldStderr:
		return streamValue(o.Stderr, o.StderrCut, FieldStderr)
	case FieldExitCode:
		return strconv.Itoa(o.ExitCode), nil
	}
	return "", fmt.Errorf("unknown field %q", name)
}

// streamValue prepares a captured stream for the next step. It drops the
// trailing newline a command leaves behind, matching what "$(...)" does in a
// shell, and leaves the rest of the value untouched.
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

// validateNeeds checks one step's references against the steps declared before
// it. A forward reference is refused rather than deferred: steps run in order,
// so a later step's output cannot exist by the time this one starts.
func validateNeeds(step Step, earlier map[string]bool) error {
	for name, ref := range step.Needs {
		if err := validEnvName(name); err != nil {
			return fmt.Errorf("step %q: %w", step.Name, err)
		}
		if name == tokenEnvVar {
			return fmt.Errorf("step %q may not bind %s", step.Name, tokenEnvVar)
		}
		if _, clash := step.Env[name]; clash {
			return fmt.Errorf("step %q sets %s in both env and needs", step.Name, name)
		}
		parsed, err := ParseReference(ref)
		if err != nil {
			return fmt.Errorf("step %q: %w", step.Name, err)
		}
		if parsed.Step == step.Name {
			return fmt.Errorf("step %q needs its own output", step.Name)
		}
		if !earlier[parsed.Step] {
			return fmt.Errorf("step %q needs %q, which does not run before it", step.Name, parsed.Step)
		}
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
