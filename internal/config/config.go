package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type MyceliumConfig struct {
	Machine   string   `yaml:"machine,omitempty"`
	URL       string   `yaml:"url,omitempty"`
	Token     string   `yaml:"token,omitempty"`
	RuleOrder []string `yaml:"rule_order,omitempty"`
	Agents    []string `yaml:"agents,omitempty"`
}

func DataDir() string {
	if dir := os.Getenv("DATA_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".mycelium")
}

func ConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".mycelium.yml")
}

func MemoryDir() string   { return filepath.Join(DataDir(), "memory") }
func RulesDir() string    { return filepath.Join(DataDir(), "rules") }
func SkillsDir() string   { return filepath.Join(DataDir(), "skills") }
func SessionsDir() string { return filepath.Join(DataDir(), "sessions") }

func MachinesDir() string { return filepath.Join(DataDir(), "machines") }

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

func SaveMyceliumConfig(cfg *MyceliumConfig) error {
	path := ConfigPath()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
