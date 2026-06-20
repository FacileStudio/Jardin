package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type RucheConfig struct {
	ActiveCell string    `yaml:"active_cell,omitempty"`
	Machine    string    `yaml:"machine,omitempty"`
	URL    string    `yaml:"url,omitempty"`
	Token  string    `yaml:"token,omitempty"`
	Cells      []CellRef `yaml:"cells,omitempty"`
}

type CellRef struct {
	Name string `yaml:"name"`
	Path string `yaml:"path"`
}

type CellConfig struct {
	Name               string   `yaml:"name"`
	Description        string   `yaml:"description,omitempty"`
	RuleOrder          []string `yaml:"rule_order,omitempty"`
	LayerCells         []string `yaml:"layer_cells,omitempty"`
	PerceptionEndpoint string   `yaml:"perception_endpoint,omitempty"`
	WorkspaceID        string   `yaml:"perception_workspace_id,omitempty"`
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

func CellsDir() string {
	return filepath.Join(DataDir(), "cells")
}

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

func LoadCellConfig(cellPath string) (*CellConfig, error) {
	path := filepath.Join(cellPath, "cell.yml")
	cfg := &CellConfig{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("cell.yml not found in %s", cellPath)
		}
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	return cfg, nil
}

func SaveCellConfig(cellPath string, cfg *CellConfig) error {
	path := filepath.Join(cellPath, "cell.yml")
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (h *RucheConfig) FindCell(name string) *CellRef {
	for i := range h.Cells {
		if h.Cells[i].Name == name {
			return &h.Cells[i]
		}
	}
	return nil
}

func (h *RucheConfig) ActiveCellPath() (string, error) {
	if h.ActiveCell == "" {
		return "", fmt.Errorf("no active cell — run 'ruche use <cell>' first")
	}
	ref := h.FindCell(h.ActiveCell)
	if ref == nil {
		return "", fmt.Errorf("cell %q not found", h.ActiveCell)
	}
	return ref.Path, nil
}

func (h *RucheConfig) AddCell(name, path string) {
	if h.FindCell(name) != nil {
		return
	}
	h.Cells = append(h.Cells, CellRef{Name: name, Path: path})
}
