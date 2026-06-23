package adapter

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/FacileStudio/Ruche/internal/cell"
)

func TestCodexEmitsSkillFilesNotInline(t *testing.T) {
	c := &Codex{}
	out, err := c.Generate(Input{
		Rules:  []cell.NamedFile{{Name: "r", Content: "RULE BODY"}},
		Skills: []cell.NamedFile{{Name: "vhs", Content: "VHS SKILL BODY"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var agents, skill string
	for p, content := range out.Files {
		if strings.HasSuffix(p, filepath.Join(".codex", "AGENTS.md")) {
			agents = content
		}
		if strings.HasSuffix(p, filepath.Join("skills", "vhs", "SKILL.md")) {
			skill = content
		}
	}
	if agents == "" {
		t.Fatal("no AGENTS.md emitted")
	}
	if !strings.Contains(agents, "RULE BODY") {
		t.Error("AGENTS.md missing rules")
	}
	if strings.Contains(agents, "VHS SKILL BODY") {
		t.Error("skill must NOT be inlined into AGENTS.md")
	}
	if skill != "VHS SKILL BODY" {
		t.Errorf("expected skill file content, got %q", skill)
	}
}
