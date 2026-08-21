package adapter

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/FacileStudio/Mycelium/internal/cell"
)

// TestGeminiWritesSkillsAsFiles pins the one property that keeps GEMINI.md
// small. Gemini sends every line of it on every turn, so a skill inlined
// there is paid for permanently while one under ~/.gemini/skills/ costs its
// name and description until the model activates it.
//
// Worth a test because the regression is invisible: inlining still produces a
// working config, it just silently multiplies the always-on prompt by the
// number of installed skills, and nothing fails.
func TestGeminiWritesSkillsAsFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	out, err := (&Gemini{}).Generate(Input{
		Rules:  []cell.NamedFile{{Name: "r", Content: "RULE BODY"}},
		Skills: []cell.NamedFile{{Name: "vhs", Content: "VHS SKILL BODY"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	geminiMd := filepath.Join(home, ".gemini", "GEMINI.md")
	skillMd := filepath.Join(home, ".gemini", "skills", "vhs", "SKILL.md")
	written := slices.Sorted(maps.Keys(out.Files))

	if out.Files[skillMd] != "VHS SKILL BODY" {
		t.Errorf("no skill at %s, got %v", skillMd, written)
	}
	if !strings.Contains(out.Files[geminiMd], "RULE BODY") {
		t.Error("GEMINI.md missing rules")
	}
	if strings.Contains(out.Files[geminiMd], "VHS SKILL BODY") {
		t.Error("skill must NOT be inlined into GEMINI.md")
	}
}
