// Package flow runs recorded shell procedures and keeps an artifact of every
// execution. A flow is an ordered list of steps stored as YAML; running one
// produces a Run that is written to disk and never synced.
package flow

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	// DefaultTimeout is the per-step timeout in seconds when a step sets none.
	DefaultTimeout = 300
	// MaxTimeout is the largest per-step timeout a flow may ask for.
	MaxTimeout = 3600
	// MaxStreamBytes caps how much of each captured stream a run artifact keeps.
	MaxStreamBytes = 1 << 20
	// Extension is the file extension every flow file carries.
	Extension = ".yml"
)

const (
	// StatusOK marks a run whose steps all succeeded.
	StatusOK = "ok"
	// StatusFailed marks a run stopped by a step that returned non-zero.
	StatusFailed = "failed"
	// StatusTimeout marks a run stopped by a step that exceeded its timeout.
	StatusTimeout = "timeout"
)

// Step is one shell command in a flow. Run is handed to "sh -c" unchanged; no
// interpolation happens anywhere, so values reach a command only through Env.
type Step struct {
	Name         string            `yaml:"name"`
	Run          string            `yaml:"run"`
	Env          map[string]string `yaml:"env,omitempty"`
	Timeout      int               `yaml:"timeout,omitempty"`
	AllowFailure bool              `yaml:"allow_failure,omitempty"`
}

// EffectiveTimeout returns the step's timeout in seconds, applying the default
// when the step sets none.
func (s Step) EffectiveTimeout() int {
	if s.Timeout <= 0 {
		return DefaultTimeout
	}
	return s.Timeout
}

// Flow is a named, ordered list of steps. Path and Checksum describe where the
// flow was read from and are not part of the file format.
type Flow struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Steps       []Step `yaml:"steps"`
	Path        string `yaml:"-"`
	Checksum    string `yaml:"-"`
}

// StepResult records what one step did. Stdout and Stderr are redacted and
// truncated before they reach this struct.
type StepResult struct {
	Name       string `json:"name"`
	ExitCode   int    `json:"exit_code"`
	DurationMS int64  `json:"duration_ms"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	Truncated  bool   `json:"truncated"`
	TimedOut   bool   `json:"timed_out"`
}

// Run records one execution of a flow. FlowChecksum pins which version of the
// flow produced the record, so history stays readable after a flow is edited.
type Run struct {
	Flow         string       `json:"flow"`
	FlowChecksum string       `json:"flow_checksum"`
	Machine      string       `json:"machine"`
	WorkDir      string       `json:"work_dir"`
	StartedAt    time.Time    `json:"started_at"`
	FinishedAt   time.Time    `json:"finished_at"`
	Status       string       `json:"status"`
	Steps        []StepResult `json:"steps"`
	ID           string       `json:"-"`
}

// Duration returns how long the run took.
func (r Run) Duration() time.Duration {
	return r.FinishedAt.Sub(r.StartedAt)
}

// Checksum returns the sha256 of a flow file's bytes, prefixed with its
// algorithm so the stored form stays self-describing.
func Checksum(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// NameFromPath returns the flow name a file at path must declare.
func NameFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// Parse decodes a flow file and validates it. Unknown fields are rejected so a
// typo fails loudly instead of being silently ignored.
func Parse(path string, data []byte) (*Flow, error) {
	var f Flow
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	f.Path = path
	f.Checksum = Checksum(data)
	if err := f.validate(NameFromPath(path)); err != nil {
		return nil, err
	}
	return &f, nil
}

func (f *Flow) validate(stem string) error {
	if f.Name == "" {
		return fmt.Errorf("flow has no name")
	}
	if f.Name != stem {
		return fmt.Errorf("flow is named %q but its file is %q", f.Name, stem)
	}
	if len(f.Steps) == 0 {
		return fmt.Errorf("flow %q has no steps", f.Name)
	}
	return f.validateSteps()
}

func (f *Flow) validateSteps() error {
	seen := make(map[string]bool, len(f.Steps))
	for i, s := range f.Steps {
		if s.Name == "" {
			return fmt.Errorf("step %d has no name", i+1)
		}
		if seen[s.Name] {
			return fmt.Errorf("step %q is declared twice", s.Name)
		}
		seen[s.Name] = true
		if strings.TrimSpace(s.Run) == "" {
			return fmt.Errorf("step %q has nothing to run", s.Name)
		}
		if s.Timeout < 0 {
			return fmt.Errorf("step %q has a negative timeout", s.Name)
		}
		if s.Timeout > MaxTimeout {
			return fmt.Errorf("step %q asks for %ds, over the %ds cap", s.Name, s.Timeout, MaxTimeout)
		}
	}
	return nil
}
