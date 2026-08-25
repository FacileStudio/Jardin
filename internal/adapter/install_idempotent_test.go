package adapter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/FacileStudio/Mycelium/internal/cell"
)

// installInput is the input every adapter is exercised with here, fenced so
// the round trip covers the conditional renderer as well as the merge.
func installInput() Input {
	return Input{
		Rules:   []cell.NamedFile{{Name: "20-memory", Content: fencedRule}},
		Skills:  []cell.NamedFile{{Name: "vhs", Content: "# vhs\n"}},
		Machine: "lucy is the laptop",
	}
}

// writeAll is what cmd.runAdapter does with Output.Files, minus the colour.
func writeAll(t *testing.T, out *Output) {
	t.Helper()
	for path, content := range out.Files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestASecondInstallIsByteIdenticalForEveryAdapter is the regression gate for
// the failure that cost a morning: install merged additively, so a second run
// added its entry beside the first and `mycelium diff` still reported no
// changes because the one it wanted was present.
//
// It runs over the whole registry rather than the adapters that declare a
// server today, so the next adapter to grow an MCP file is covered before
// anybody remembers to add a test for it. Comparing the entire Output.Files
// map catches non-determinism anywhere in Generate, not only in the merge.
func TestASecondInstallIsByteIdenticalForEveryAdapter(t *testing.T) {
	for _, a := range All() {
		t.Run(a.Name(), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Chdir(home)

			first, err := a.Generate(installInput())
			if err != nil {
				t.Fatal(err)
			}
			writeAll(t, first)

			second, err := a.Generate(installInput())
			if err != nil {
				t.Fatal(err)
			}
			writeAll(t, second)

			if len(second.Files) != len(first.Files) {
				t.Fatalf("second install writes %d files, first wrote %d", len(second.Files), len(first.Files))
			}
			for path, content := range first.Files {
				if second.Files[path] != content {
					t.Errorf("%s is not stable across installs:\n--- first\n%s\n--- second\n%s",
						path, content, second.Files[path])
				}
			}
		})
	}
}

// TestASecondInstallLeavesTheFilesOnDiskUntouched is the same property seen
// from where the user sees it: `mycelium diff <agent>` compares generated
// content against the bytes on disk, so equal-but-reformatted is still a
// permanent phantom diff.
func TestASecondInstallLeavesTheFilesOnDiskUntouched(t *testing.T) {
	for _, a := range All() {
		t.Run(a.Name(), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Chdir(home)

			out, err := a.Generate(installInput())
			if err != nil {
				t.Fatal(err)
			}
			writeAll(t, out)

			again, err := a.Generate(installInput())
			if err != nil {
				t.Fatal(err)
			}
			for path, content := range again.Files {
				onDisk, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("%s: %v", path, err)
				}
				if string(onDisk) != content {
					t.Errorf("%s would still show as modified:\n--- on disk\n%s\n--- generated\n%s",
						path, onDisk, content)
				}
			}
		})
	}
}
