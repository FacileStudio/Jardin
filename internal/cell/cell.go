package cell

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/FacileStudio/Ruche/internal/config"
)

func Init() error {
	dirs := []string{
		"memory", "memory/bugs", "memory/tools", "memory/projects",
		"memory/conventions", "memory/syntheses",
		"rules", "skills", "machines",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(config.DataDir(), d), 0755); err != nil {
			return err
		}
	}

	writeIfMissing(filepath.Join(config.MemoryDir(), "index.md"), "# Memory Index\n")
	writeIfMissing(filepath.Join(config.MemoryDir(), "overview.md"), "# Overview\n")
	writeIfMissing(filepath.Join(config.MemoryDir(), "log.md"), "# Log\n\nAppend-only.\n")
	return nil
}

func ListRules() ([]string, error)  { return listMdFiles(config.RulesDir()) }
func ListSkills() ([]string, error) { return listMdFiles(config.SkillsDir()) }

func ReadRules(order []string) ([]NamedFile, error) {
	dir := config.RulesDir()
	names, err := listMdFiles(dir)
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	var files []NamedFile
	add := func(name string) {
		if seen[name] {
			return
		}
		content, err := readFile(filepath.Join(dir, name+".md"))
		if err != nil {
			return
		}
		seen[name] = true
		files = append(files, NamedFile{Name: name, Content: content})
	}

	for _, name := range order {
		add(name)
	}
	for _, name := range names {
		add(name)
	}
	return files, nil
}

func ReadSkills() ([]NamedFile, error) {
	dir := config.SkillsDir()
	names, err := listMdFiles(dir)
	if err != nil {
		return nil, err
	}
	var files []NamedFile
	for _, name := range names {
		content, err := readFile(filepath.Join(dir, name+".md"))
		if err != nil {
			return nil, err
		}
		files = append(files, NamedFile{Name: name, Content: content})
	}
	return files, nil
}

func ReadMachine(machine string) (string, error) {
	path := filepath.Join(config.MachinesDir(), machine+".md")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", nil
	}
	return readFile(path)
}

type NamedFile struct {
	Name    string
	Content string
}

func readFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
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

func writeIfMissing(path, content string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		os.WriteFile(path, []byte(content), 0644)
	}
}
