package adapter

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/FacileStudio/Jardin/internal/cell"
)

func TestOpencodeEmitsSkillFilesNotInline(t *testing.T) {
	o := &Opencode{}
	out, err := o.Generate(Input{
		Rules:  []cell.NamedFile{{Name: "r", Content: "RULE BODY"}},
		Skills: []cell.NamedFile{{Name: "vhs", Content: "VHS SKILL BODY"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var agents, skill string
	for p, content := range out.Files {
		if strings.HasSuffix(p, filepath.Join("opencode", "AGENTS.md")) {
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
	if !strings.Contains(agents, "jardin recap") {
		t.Error("AGENTS.md missing recap reminder")
	}
	if strings.Contains(agents, "VHS SKILL BODY") {
		t.Error("skill must NOT be inlined into AGENTS.md")
	}
	if skill != "VHS SKILL BODY" {
		t.Errorf("expected skill file content, got %q", skill)
	}
}

func TestOpencodeTargetPathsUnderConfigDir(t *testing.T) {
	o := &Opencode{}
	paths := o.TargetPaths()
	if len(paths) != 1 {
		t.Fatalf("expected 1 target path, got %d", len(paths))
	}
	if !strings.Contains(paths[0], filepath.Join(".config", "opencode", "AGENTS.md")) {
		t.Fatalf("unexpected target path: %s", paths[0])
	}
}
