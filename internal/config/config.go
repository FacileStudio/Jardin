package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type JardinConfig struct {
	Machine    string   `yaml:"machine,omitempty"`
	URL        string   `yaml:"url,omitempty"`
	Token      string   `yaml:"token,omitempty"`
	Space      string   `yaml:"space,omitempty"`
	UsageToken string   `yaml:"usage_token,omitempty"`
	RuleOrder  []string `yaml:"rule_order,omitempty"`
	Agents     []string `yaml:"agents,omitempty"`
}

// The environment overrides for the server session. Precedence is flag, then
// environment, then the config file: a CI job cannot run an interactive login
// and must not commit a config file, so an env var is the only credential
// channel it has.
//
// URLEnvAlt is the spelling CLI-STANDARD §6.3 gives the whole suite; JARDIN_URL
// is the short form documented for Jardin. Both are read, JARDIN_URL wins.
const (
	TokenEnv  = "JARDIN_TOKEN"
	URLEnv    = "JARDIN_URL"
	URLEnvAlt = "JARDIN_SERVER_URL"
)

// ServerURL resolves the instance to talk to, environment first.
func (c *JardinConfig) ServerURL() string {
	for _, key := range []string{URLEnv, URLEnvAlt} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return strings.TrimRight(value, "/")
		}
	}
	return c.URL
}

// AuthToken resolves the credential to present, environment first.
func (c *JardinConfig) AuthToken() string {
	if value := strings.TrimSpace(os.Getenv(TokenEnv)); value != "" {
		return value
	}
	return c.Token
}

func DataDir() string {
	if dir := os.Getenv("DATA_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".jardin")
}

func ConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".jardin.yml")
}

func MemoryDir() string   { return filepath.Join(DataDir(), "memory") }
func RulesDir() string    { return filepath.Join(DataDir(), "rules") }
func SkillsDir() string   { return filepath.Join(DataDir(), "skills") }
func SessionsDir() string { return filepath.Join(DataDir(), "sessions") }

func MachinesDir() string { return filepath.Join(DataDir(), "machines") }

func LoadJardinConfig() (*JardinConfig, error) {
	path := ConfigPath()
	cfg := &JardinConfig{}
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

func SaveJardinConfig(cfg *JardinConfig) error {
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
