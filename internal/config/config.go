package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type HiveConfig struct {
	ActiveCell string    `toml:"active_cell"`
	Machine    string    `toml:"machine"`
	SyncURL    string    `toml:"sync_url,omitempty"`
	SyncToken  string    `toml:"sync_token,omitempty"`
	Cells      []CellRef `toml:"cells"`
}

type CellRef struct {
	Name string `toml:"name"`
	Path string `toml:"path"`
}

type CellConfig struct {
	Name               string   `toml:"name"`
	Description        string   `toml:"description,omitempty"`
	RuleOrder          []string `toml:"rule_order,omitempty"`
	LayerCells         []string `toml:"layer_cells,omitempty"`
	PerceptionEndpoint string   `toml:"perception_endpoint,omitempty"`
	WorkspaceID        string   `toml:"perception_workspace_id,omitempty"`
}

func HiveDir() string {
	if dir := os.Getenv("HIVE_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hive")
}

func HiveConfigPath() string {
	return filepath.Join(HiveDir(), "hive.toml")
}

func CellsDir() string {
	return filepath.Join(HiveDir(), "cells")
}

func LoadHiveConfig() (*HiveConfig, error) {
	path := HiveConfigPath()
	cfg := &HiveConfig{}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}
	_, err := toml.DecodeFile(path, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to load %s: %w", path, err)
	}
	return cfg, nil
}

func SaveHiveConfig(cfg *HiveConfig) error {
	path := HiveConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
}

func LoadCellConfig(cellPath string) (*CellConfig, error) {
	path := filepath.Join(cellPath, "cell.toml")
	cfg := &CellConfig{}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("cell.toml not found in %s", cellPath)
	}
	_, err := toml.DecodeFile(path, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to load %s: %w", path, err)
	}
	return cfg, nil
}

func SaveCellConfig(cellPath string, cfg *CellConfig) error {
	path := filepath.Join(cellPath, "cell.toml")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
}

func (h *HiveConfig) FindCell(name string) *CellRef {
	for i := range h.Cells {
		if h.Cells[i].Name == name {
			return &h.Cells[i]
		}
	}
	return nil
}

func (h *HiveConfig) ActiveCellPath() (string, error) {
	if h.ActiveCell == "" {
		return "", fmt.Errorf("no active cell — run 'hive use <cell>' first")
	}
	ref := h.FindCell(h.ActiveCell)
	if ref == nil {
		return "", fmt.Errorf("cell %q not found", h.ActiveCell)
	}
	return ref.Path, nil
}

func (h *HiveConfig) AddCell(name, path string) {
	if h.FindCell(name) != nil {
		return
	}
	h.Cells = append(h.Cells, CellRef{Name: name, Path: path})
}
