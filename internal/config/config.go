package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// MyceliumConfig is the persisted client configuration: which server and space
// to sync, the credential, and the order rules are read in.
type MyceliumConfig struct {
	Machine    string `yaml:"machine,omitempty"`
	URL        string `yaml:"url,omitempty"`
	Token      string `yaml:"token,omitempty"`
	Space      string `yaml:"space,omitempty"`
	UsageToken string `yaml:"usage_token,omitempty"`
	// VectorSearch opts this machine into semantic memory search. It is off by
	// default: the server half needs an embedding model, and a machine whose
	// server has none should not pay a round trip to be told so on every query.
	VectorSearch bool              `yaml:"vector_search,omitempty"`
	RuleOrder    []string          `yaml:"rule_order,omitempty"`
	Agents       []string          `yaml:"agents,omitempty"`
	Consolidate  ConsolidateConfig `yaml:"consolidate,omitempty"`
}

// ConsolidateConfig tunes the daemon's episodic-to-semantic consolidation
// stage. Enabled is a pointer so an absent key keeps the default (on) while an
// explicit false turns the stage off.
type ConsolidateConfig struct {
	JudgeModel string `yaml:"judge_model,omitempty"`
	Enabled    *bool  `yaml:"enabled,omitempty"`
}

// The environment overrides for the server session. Precedence is flag, then
// environment, then the config file: a CI job cannot run an interactive login
// and must not commit a config file, so an env var is the only credential
// channel it has.
//
// URLEnvAlt is the spelling CLI-STANDARD §6.3 gives the whole suite; MYCELIUM_URL
// is the short form documented for Mycelium. Both are read, MYCELIUM_URL wins.
const (
	TokenEnv        = "MYCELIUM_TOKEN"
	SpaceEnv        = "MYCELIUM_SPACE"
	VectorSearchEnv = "MYCELIUM_VECTOR_SEARCH"
	URLEnv          = "MYCELIUM_URL"
	URLEnvAlt       = "MYCELIUM_SERVER_URL"
)

// SpaceID resolves the active space ID, environment first.
func (c *MyceliumConfig) SpaceID() string {
	if value := strings.TrimSpace(os.Getenv(SpaceEnv)); value != "" {
		if value == "none" || value == "common" {
			return ""
		}
		return value
	}
	if c.Space == "none" || c.Space == "common" {
		return ""
	}
	return c.Space
}

// ServerURL resolves the instance to talk to, environment first.
// SemanticEnabled reports whether this machine should ask the server for
// semantic results. It is off unless asked for: the server half needs an
// embedding model, and a machine whose server has none should not pay a round
// trip to be told so on every query.
func (c *MyceliumConfig) SemanticEnabled() bool {
	if value := strings.TrimSpace(os.Getenv(VectorSearchEnv)); value != "" {
		return value == "1" || strings.EqualFold(value, "true")
	}
	return c.VectorSearch
}

// JudgeModel returns the local Ollama model used by the consolidation judge;
// empty means fallback mode, where every heuristic candidate is accepted.
func (c *MyceliumConfig) JudgeModel() string {
	return strings.TrimSpace(c.Consolidate.JudgeModel)
}

// ConsolidateEnabled reports whether the daemon should run the consolidation
// stage. Absent from the config file means on.
func (c *MyceliumConfig) ConsolidateEnabled() bool {
	if c.Consolidate.Enabled != nil {
		return *c.Consolidate.Enabled
	}
	return true
}

func (c *MyceliumConfig) ServerURL() string {
	for _, key := range []string{URLEnv, URLEnvAlt} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return strings.TrimRight(value, "/")
		}
	}
	return c.URL
}

// AuthToken resolves the credential to present, environment first.
func (c *MyceliumConfig) AuthToken() string {
	if value := strings.TrimSpace(os.Getenv(TokenEnv)); value != "" {
		return value
	}
	return c.Token
}

// DataDir is the root of the local memory store, overridable via DATA_DIR.
func DataDir() string {
	if dir := os.Getenv("DATA_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".mycelium")
}

// ConfigPath is where the client configuration file lives.
func ConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".mycelium.yml")
}

// MemoryDir returns the memory store's root.
func MemoryDir() string { return filepath.Join(DataDir(), "memory") }

// RulesDir returns the directory holding rule files.
func RulesDir() string { return filepath.Join(DataDir(), "rules") }

// SkillsDir returns the directory holding skill files.
func SkillsDir() string { return filepath.Join(DataDir(), "skills") }

// FlowsDir returns the directory holding flow files.
func FlowsDir() string { return filepath.Join(DataDir(), "flows") }

// RunsDir returns the directory holding flow run artifacts.
func RunsDir() string { return filepath.Join(DataDir(), "runs") }

// ModelsDir returns the directory holding typed model extensions. Unlike runs,
// these sync: they are code, which is why running one needs a trust pin.
func ModelsDir() string { return filepath.Join(DataDir(), "extensions", "models") }

// SessionsDir returns the directory holding session data.
func SessionsDir() string { return filepath.Join(DataDir(), "sessions") }

// MachinesDir returns the directory holding machine profiles.
func MachinesDir() string { return filepath.Join(DataDir(), "machines") }

// LoadMyceliumConfig reads the config file, returning an empty config when no
// file exists yet.
func LoadMyceliumConfig() (*MyceliumConfig, error) {
	path := ConfigPath()
	cfg := &MyceliumConfig{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	return cfg, nil
}

// SaveMyceliumConfig writes the config file with owner-only permissions.
func SaveMyceliumConfig(cfg *MyceliumConfig) error {
	path := ConfigPath()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}

// MachineName is what this machine calls itself: the configured name, the
// hostname when none is set, and "unknown" when even that fails. It never
// returns an empty string, because it names the author of a journal commit and
// the owner of a claim, and both read as corrupt rather than absent when blank.
func MachineName() string {
	if cfg, err := LoadMyceliumConfig(); err == nil && strings.TrimSpace(cfg.Machine) != "" {
		return strings.TrimSpace(cfg.Machine)
	}
	if host, err := os.Hostname(); err == nil && host != "" {
		return host
	}
	return "unknown"
}
