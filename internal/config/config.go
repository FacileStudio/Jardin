package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type RucheConfig struct {
	Machine   string   `yaml:"machine,omitempty"`
	URL       string   `yaml:"url,omitempty"`
	Token     string   `yaml:"token,omitempty"`
	RuleOrder []string `yaml:"rule_order,omitempty"`
}

func DataDir() string {
	if dir := os.Getenv("DATA_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ruche")
}

func ConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ruche.yml")
}

func BrainDir() string  { return filepath.Join(DataDir(), "brain") }
func RulesDir() string  { return filepath.Join(DataDir(), "rules") }
func SkillsDir() string { return filepath.Join(DataDir(), "skills") }
func MachinesDir() string { return filepath.Join(DataDir(), "machines") }

func LoadRucheConfig() (*RucheConfig, error) {
	path := ConfigPath()
	cfg := &RucheConfig{}
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

func SaveRucheConfig(cfg *RucheConfig) error {
	path := ConfigPath()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
