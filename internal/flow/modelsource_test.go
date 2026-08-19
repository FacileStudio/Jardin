package flow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FacileStudio/Mycelium/internal/config"
)

// writeModelFile puts an arbitrary file in the models root — a helper, a
// package.json — where writeModel only writes an entry for a type name.
func writeModelFile(t *testing.T, rel, body string) {
	t.Helper()
	path := filepath.Join(config.ModelsDir(), filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The pin has to cover what the model imports, not just the file it names.
// Every model written with the defineModel helper hands its argv, its stdin and
// the call into execute() to a file in _lib — so a pin that stopped at the entry
// left the outermost layer of every model editable under an approval that still
// read clean.
func TestEditingAnImportedHelperLosesThePin(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	writeModelFile(t, "package.json", `{"imports":{"#lib/*":"./_lib/*.ts"}}`)
	writeModelFile(t, "_lib/run.ts", "export const tag = 'original';\n")
	writeModel(t, "@test/importer", "import { tag } from '#lib/run';\nconsole.log(tag);\n")

	trustModel(t, "@test/importer")
	if _, err := LoadModel("@test/importer"); err != nil {
		t.Fatalf("a freshly pinned model was refused: %v", err)
	}

	writeModelFile(t, "_lib/run.ts", "export const tag = 'swapped';\n")

	_, err := LoadModel("@test/importer")
	if err == nil {
		t.Fatal("an edited helper ran under the model's old pin")
	}
	if !strings.Contains(err.Error(), "changed since") {
		t.Errorf("error = %q, want it to say the model changed", err)
	}
}

// A relative import is covered the same way a subpath one is.
func TestClosureFollowsRelativeImports(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	writeModelFile(t, "test/near.ts", "export const n = 1;\n")
	writeModel(t, "@test/rel", "import { n } from './near';\nconsole.log(n);\n")

	m, err := InspectModel("@test/rel")
	if err != nil {
		t.Fatal(err)
	}
	var rels []string
	for _, s := range m.Sources {
		rels = append(rels, s.Rel)
	}
	if len(rels) != 2 || rels[0] != "test/rel.ts" || rels[1] != "test/near.ts" {
		t.Fatalf("closure = %v, want the entry first then test/near.ts", rels)
	}
}

// A bare specifier is bun's own runtime or a package: not a file in this tree,
// so it is not hashed and must not be mistaken for a missing import.
func TestBareSpecifiersAreNotPartOfTheClosure(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	writeModel(t, "@test/bare", "import { $ } from 'bun';\nconsole.log($);\n")

	m, err := InspectModel("@test/bare")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Sources) != 1 {
		t.Fatalf("closure has %d files, want only the entry", len(m.Sources))
	}
}

// bun runs require() and a backticked specifier exactly as it runs a plain
// import, so both pull code in and both have to be hashed. They were missed
// when the closure first shipped, which left the pin bypassable through two
// ordinary syntaxes — the very hole the closure exists to close.
func TestClosureCoversRequireAndBacktickSpecifiers(t *testing.T) {
	for name, entry := range map[string]string{
		"require":         "const h = require('./near');\nconsole.log(h);\n",
		"backtick import": "const h = await import(`./near`);\nconsole.log(h);\n",
		"backtick from":   "import { n } from `./near`;\nconsole.log(n);\n",
		"double quoted":   "import { n } from \"./near\";\nconsole.log(n);\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("DATA_DIR", t.TempDir())
			writeModelFile(t, "test/near.ts", "export const n = 1;\n")
			writeModel(t, "@test/entry", entry)

			m, err := InspectModel("@test/entry")
			if err != nil {
				t.Fatal(err)
			}
			if len(m.Sources) != 2 {
				t.Fatalf("closure has %d file(s), want the entry and test/near.ts", len(m.Sources))
			}
		})
	}
}

// A computed specifier cannot be resolved statically. It must not be guessed at
// either: resolving it to something arbitrary would pin the wrong file and read
// as though the closure were complete.
func TestComputedSpecifierIsNotGuessed(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	writeModelFile(t, "test/near.ts", "export const n = 1;\n")
	writeModel(t, "@test/computed", "const which = 'near';\nawait import(`./${which}`);\n")

	m, err := InspectModel("@test/computed")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Sources) != 1 {
		t.Fatalf("closure has %d files, want only the entry", len(m.Sources))
	}
}
