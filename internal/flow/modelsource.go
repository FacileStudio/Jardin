package flow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/FacileStudio/Jardin/internal/config"
)

// ModelSource is one file a model runs: its entry, or something the entry
// reaches through imports without leaving the models root.
type ModelSource struct {
	Rel  string
	Path string
	Data []byte
}

// importSpecifier matches the module string in an import or export statement, a
// dynamic import(), and a require() — bun runs all three, so all three pull code
// in and all three have to be hashed. Backticks count: a template literal with
// no substitution is an ordinary static specifier.
//
// It is deliberately generous. A false positive resolves to no file and is
// dropped, while a miss leaves running code out of the checksum, which is the
// whole failure this exists to prevent.
var importSpecifier = regexp.MustCompile(
	"\\b(?:import|require)\\s*\\(\\s*[\"'`]([^\"'`]+)[\"'`]\\s*\\)" +
		"|\\b(?:import|export)\\b[^\"'`();]*?[\"'`]([^\"'`]+)[\"'`]")

func specifiers(src []byte) []string {
	var out []string
	for _, match := range importSpecifier.FindAllSubmatch(src, -1) {
		for _, group := range match[1:] {
			if len(group) > 0 {
				out = append(out, string(group))
			}
		}
	}
	return out
}

// subpathImports reads the "imports" map the models root may declare, so
// "#lib/defineModel" resolves the way bun resolves it. Conditional targets are
// objects rather than strings; those are skipped rather than guessed at.
func subpathImports(root string) map[string]string {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return nil
	}
	var pkg struct {
		Imports map[string]json.RawMessage `json:"imports"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return nil
	}
	out := make(map[string]string, len(pkg.Imports))
	for pattern, raw := range pkg.Imports {
		var target string
		if json.Unmarshal(raw, &target) == nil {
			out[pattern] = target
		}
	}
	return out
}

// matchSubpath applies one "#name/*" pattern the way the imports map defines
// it, substituting what the star captured into the target.
func matchSubpath(spec string, imports map[string]string) (string, bool) {
	if target, ok := imports[spec]; ok {
		return target, true
	}
	for pattern, target := range imports {
		star := strings.Index(pattern, "*")
		if star < 0 {
			continue
		}
		prefix, suffix := pattern[:star], pattern[star+1:]
		if !strings.HasPrefix(spec, prefix) || !strings.HasSuffix(spec, suffix) {
			continue
		}
		captured := spec[star : len(spec)-len(suffix)]
		return strings.Replace(target, "*", captured, 1), true
	}
	return "", false
}

// existingFile applies the extension resolution a bare specifier relies on.
func existingFile(base string) (string, bool) {
	for _, candidate := range []string{base, base + modelExt, filepath.Join(base, "index"+modelExt)} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
	}
	return "", false
}

func within(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// resolveImport turns one specifier into a file inside root, or reports that
// there is nothing in the tree to hash. A bare specifier is bun's own runtime
// or a package and is not ours to pin; a specifier that resolves to a real file
// outside the models root is refused, because that is code the trust gate does
// not cover being pulled into something it does.
func resolveImport(fromDir, spec, root string, imports map[string]string) (string, bool, error) {
	var candidate string
	switch {
	case strings.HasPrefix(spec, "#"):
		target, ok := matchSubpath(spec, imports)
		if !ok {
			return "", false, nil
		}
		candidate = filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(target, "./")))
	case strings.HasPrefix(spec, "./"), strings.HasPrefix(spec, "../"):
		candidate = filepath.Join(fromDir, filepath.FromSlash(spec))
	case filepath.IsAbs(spec):
		candidate = filepath.FromSlash(spec)
	default:
		return "", false, nil
	}
	found, ok := existingFile(candidate)
	if !ok {
		return "", false, nil
	}
	if !within(found, root) {
		return "", false, fmt.Errorf("import %q resolves to %s, outside %s", spec, found, root)
	}
	return found, true, nil
}

// ModelSources returns every file inside the models root that the entry reaches
// through imports — entry first, the rest sorted by path so the order is the
// same on every machine.
//
// Static resolution cannot follow a computed specifier such as
// import(`./${name}`). One is visible in the entry file a person reads before
// pinning, which is where that case is caught; it is not silently covered here.
func ModelSources(entry string) ([]ModelSource, error) {
	root := config.ModelsDir()
	imports := subpathImports(root)
	seen := map[string]bool{}
	queue := []string{entry}

	var out []ModelSource
	for len(queue) > 0 {
		path := queue[0]
		queue = queue[1:]
		if seen[path] {
			continue
		}
		seen[path] = true

		src, imported, err := readSource(path, root, imports)
		if err != nil {
			return nil, err
		}
		out = append(out, src)
		queue = append(queue, imported...)
	}

	rest := out[1:]
	sort.Slice(rest, func(i, j int) bool { return rest[i].Rel < rest[j].Rel })
	return out, nil
}

// readSource reads one file of the closure and resolves the in-tree files it
// imports. Duplicates in the returned list are harmless: the caller drops a
// path it has already visited when it comes off the queue.
func readSource(path, root string, imports map[string]string) (ModelSource, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ModelSource{}, nil, err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return ModelSource{}, nil, err
	}
	var imported []string
	for _, spec := range specifiers(data) {
		resolved, ok, err := resolveImport(filepath.Dir(path), spec, root, imports)
		if err != nil {
			return ModelSource{}, nil, err
		}
		if ok {
			imported = append(imported, resolved)
		}
	}
	return ModelSource{Rel: filepath.ToSlash(rel), Path: path, Data: data}, imported, nil
}

// checksumSources hashes the whole closure — each file's path and bytes, in a
// fixed order — so editing a helper moves the sum exactly as editing the entry
// does. The length is written between them so two files cannot be rearranged
// into the same digest.
func checksumSources(sources []ModelSource) string {
	h := sha256.New()
	for _, s := range sources {
		fmt.Fprintf(h, "%s\x00%d\x00", s.Rel, len(s.Data))
		h.Write(s.Data)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}
