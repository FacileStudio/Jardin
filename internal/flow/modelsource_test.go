package flow

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FacileStudio/Jardin/internal/config"
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

// helperMarker is printed by the fixture helper as its module body runs. Its
// presence in bun's output is the only trustworthy evidence that bun really
// loaded the helper, rather than eliding an import it decided was unused.
const helperMarker = "HELPER-EXECUTED"

// helperFixture exports a value and a type so a fixture can exercise either a
// value import or a type-only one, and announces itself when it is executed.
const helperFixture = "console.log(\"" + helperMarker + "\");\n" +
	"export const n = 1;\nexport type N = number;\n"

// bunRanHelper runs an entry under bun and reports whether the helper's
// top-level code executed. A fixture bun cannot run at all proves nothing, so
// that is a failure rather than a quiet "did not execute" — counting a broken
// fixture as safe is how a gap hides.
func bunRanHelper(t *testing.T, entry string) bool {
	t.Helper()
	out, err := exec.Command(modelRuntime, entry).CombinedOutput()
	ran := strings.Contains(string(out), helperMarker)
	if err != nil && !ran {
		t.Fatalf("bun could not run %s: %v\n%s", entry, err, out)
	}
	return ran
}

// closureHasHelper reports whether the scanner put the helper in what the pin
// would cover.
func closureHasHelper(t *testing.T, entry string) bool {
	t.Helper()
	sources, err := ModelSources(entry)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range sources {
		if s.Rel == "test/helper.ts" {
			return true
		}
	}
	return false
}

// syntaxFixture installs the helper and an entry using one import syntax, and
// returns the entry's path.
func syntaxFixture(t *testing.T, entry string) string {
	t.Helper()
	writeModelFile(t, "test/helper.ts", helperFixture)
	writeModel(t, "@test/entry", entry)
	path, err := ModelPath("@test/entry")
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// TestClosureCoversEverySyntaxBunExecutes measures bun instead of trusting the
// regex. The pin is a security boundary, and it shipped incomplete twice
// because the pattern was read and believed: v0.16.0 missed require() and a
// backticked specifier, both of which bun runs. So each syntax is executed for
// real, and anything bun loads has to be in the closure — if a future runtime
// or a future edit to importSpecifier makes the two disagree, this fails.
//
// Over-inclusion is not a failure. The scanner hashing a file bun elides is
// conservative; only running code the pin does not cover is a hole.
func TestClosureCoversEverySyntaxBunExecutes(t *testing.T) {
	needsBun(t)
	forms := map[string]string{
		"import from, double quotes": "import { n } from \"./helper\";\nconsole.log(n);\n",
		"import from, single quotes": "import { n } from './helper';\nconsole.log(n);\n",
		"import from, backticks":     "import { n } from `./helper`;\nconsole.log(n);\n",
		"side-effect import":         "import \"./helper\";\n",
		"namespace import":           "import * as h from \"./helper\";\nconsole.log(h.n);\n",
		"export from":                "export { n } from \"./helper\";\n",
		"export star":                "export * from \"./helper\";\n",
		"dynamic import, literal":    "const h = await import(\"./helper\");\nconsole.log(h.n);\n",
		"dynamic import, backticks":  "const h = await import(`./helper`);\nconsole.log(h.n);\n",
		"require":                    "const h = require(\"./helper\");\nconsole.log(h.n);\n",
		"type-only import":           "import type { N } from \"./helper\";\nconst x: N = 1;\nconsole.log(x);\n",
	}

	executed := 0
	for name, body := range forms {
		t.Run(name, func(t *testing.T) {
			t.Setenv("DATA_DIR", t.TempDir())
			entry := syntaxFixture(t, body)

			ran := bunRanHelper(t, entry)
			if ran {
				executed++
			}
			if ran && !closureHasHelper(t, entry) {
				t.Fatalf("bun executes the helper through this syntax and the closure omits it: "+
					"a pin taken here would not cover running code\n%s", body)
			}
		})
	}

	if executed < len(forms)-1 {
		t.Fatalf("only %d of %d fixtures actually loaded the helper; the rest proved nothing, "+
			"so this test would pass whatever the scanner did", executed, len(forms))
	}
}

// TestComputedSpecifierIsTheOneKnownDivergence pins the single case where bun
// runs code the closure does not cover. It cannot be resolved statically and is
// deliberately not guessed at, and both docs/flow-composition.md and the wiki
// say so. If it ever starts resolving, this fails on purpose: the gap closing
// is good news that those two documents would otherwise keep contradicting.
func TestComputedSpecifierIsTheOneKnownDivergence(t *testing.T) {
	needsBun(t)
	t.Setenv("DATA_DIR", t.TempDir())
	entry := syntaxFixture(t, "const which = \"helper\";\nconst h = await import(`./${which}`);\nconsole.log(h.n);\n")

	if !bunRanHelper(t, entry) {
		t.Fatal("the fixture no longer exercises a computed specifier: bun did not load the helper")
	}
	if closureHasHelper(t, entry) {
		t.Fatal("a computed specifier now resolves into the closure — update docs/flow-composition.md " +
			"and bugs/model-trust-is-not-transitive.md, which both call this a known gap, then this test")
	}
}
