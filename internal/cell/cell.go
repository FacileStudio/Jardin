package cell

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/FacileStudio/Hive/internal/config"
)

var brainDirs = []string{
	"brain",
	"brain/bugs",
	"brain/tools",
	"brain/projects",
	"brain/conventions",
	"brain/syntheses",
}

var topDirs = []string{
	"rules",
	"skills",
	"machines",
}

func Init(name string) (string, error) {
	cellPath := filepath.Join(config.CellsDir(), name)
	if _, err := os.Stat(cellPath); err == nil {
		return "", fmt.Errorf("cell %q already exists at %s", name, cellPath)
	}

	for _, d := range append(topDirs, brainDirs...) {
		if err := os.MkdirAll(filepath.Join(cellPath, d), 0755); err != nil {
			return "", fmt.Errorf("failed to create %s: %w", d, err)
		}
	}

	cellCfg := &config.CellConfig{
		Name: name,
	}
	if err := config.SaveCellConfig(cellPath, cellCfg); err != nil {
		return "", fmt.Errorf("failed to write cell.toml: %w", err)
	}

	brainIndex := filepath.Join(cellPath, "brain", "index.md")
	os.WriteFile(brainIndex, []byte("# Brain Index\n"), 0644)

	brainOverview := filepath.Join(cellPath, "brain", "overview.md")
	os.WriteFile(brainOverview, []byte("# Overview\n"), 0644)

	brainLog := filepath.Join(cellPath, "brain", "log.md")
	os.WriteFile(brainLog, []byte("# Log\n\nAppend-only history of brain changes.\nNewest entries go at the bottom.\n"), 0644)

	return cellPath, nil
}

func ListRules(cellPath string) ([]string, error) {
	return listMdFiles(filepath.Join(cellPath, "rules"))
}

func ListSkills(cellPath string) ([]string, error) {
	return listMdFiles(filepath.Join(cellPath, "skills"))
}

func ListMachines(cellPath string) ([]string, error) {
	return listMdFiles(filepath.Join(cellPath, "machines"))
}

func ReadFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func ReadRules(cellPath string, order []string) ([]NamedFile, error) {
	rulesDir := filepath.Join(cellPath, "rules")

	if len(order) > 0 {
		var files []NamedFile
		for _, name := range order {
			path := filepath.Join(rulesDir, name+".md")
			content, err := ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("rule %q: %w", name, err)
			}
			files = append(files, NamedFile{Name: name, Content: content})
		}
		return files, nil
	}

	names, err := listMdFiles(rulesDir)
	if err != nil {
		return nil, err
	}
	var files []NamedFile
	for _, name := range names {
		path := filepath.Join(rulesDir, name+".md")
		content, err := ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("rule %q: %w", name, err)
		}
		files = append(files, NamedFile{Name: name, Content: content})
	}
	return files, nil
}

func ReadSkills(cellPath string) ([]NamedFile, error) {
	skillsDir := filepath.Join(cellPath, "skills")
	names, err := listMdFiles(skillsDir)
	if err != nil {
		return nil, err
	}
	var files []NamedFile
	for _, name := range names {
		path := filepath.Join(skillsDir, name+".md")
		content, err := ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("skill %q: %w", name, err)
		}
		files = append(files, NamedFile{Name: name, Content: content})
	}
	return files, nil
}

func ReadMachine(cellPath, machine string) (string, error) {
	path := filepath.Join(cellPath, "machines", machine+".md")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", nil
	}
	return ReadFile(path)
}

type NamedFile struct {
	Name    string
	Content string
}

func listMdFiles(dir string) ([]string, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".md"))
	}
	sort.Strings(names)
	return names, nil
}
