package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func flatTestModel() ModelID {
	return ModelID{Name: "test-embed", Digest: "sha256:aaa", Dims: 3}
}

func flatEntry(key, path string, line int, vec Vector) Entry {
	return Entry{Key: key, Path: path, Heading: key, Line: line, Hash: "hash-" + key, Vector: vec}
}

func flatOpen(t *testing.T, dir string, model ModelID) *FlatStore {
	t.Helper()
	store, err := OpenFlatStore(dir, model)
	if err != nil {
		t.Fatalf("OpenFlatStore: %v", err)
	}
	return store
}

func flatSeed(t *testing.T, store *FlatStore) {
	t.Helper()
	entries := []Entry{
		flatEntry("a.md#1", "a.md", 1, Vector{1, 0, 0}),
		flatEntry("a.md#9", "a.md", 9, Vector{0, 1, 0}),
		flatEntry("b.md#1", "b.md", 1, Vector{0, 0, 1}),
	}
	if err := store.Upsert(entries); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
}

func TestFlatStoreRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "index")
	store := flatOpen(t, dir, flatTestModel())
	flatSeed(t, store)
	if err := store.Upsert([]Entry{flatEntry("a.md#1", "a.md", 4, Vector{1, 1, 0})}); err != nil {
		t.Fatalf("Upsert replace: %v", err)
	}

	reopened := flatOpen(t, dir, flatTestModel())
	if got := len(reopened.Hashes()); got != 3 {
		t.Fatalf("hashes after reopen = %d, want 3", got)
	}
	before := store.Nearest(Vector{1, 1, 0}, 0)
	after := reopened.Nearest(Vector{1, 1, 0}, 0)
	if len(before) != len(after) {
		t.Fatalf("result count %d != %d", len(before), len(after))
	}
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("result %d = %+v, want %+v", i, after[i], before[i])
		}
	}
	if after[0].Key != "a.md#1" || after[0].Line != 4 {
		t.Fatalf("upsert did not replace in place: %+v", after[0])
	}
	if reopened.Model() != flatTestModel() {
		t.Fatalf("model = %+v", reopened.Model())
	}
}

func TestFlatStoreModelMismatchStartsEmpty(t *testing.T) {
	dir := t.TempDir()
	flatSeed(t, flatOpen(t, dir, flatTestModel()))

	other := ModelID{Name: "test-embed", Digest: "sha256:bbb", Dims: 3}
	store := flatOpen(t, dir, other)
	if got := len(store.Hashes()); got != 0 {
		t.Fatalf("hashes = %d, want 0 after model change", got)
	}
	if got := store.Nearest(Vector{1, 0, 0}, 0); len(got) != 0 {
		t.Fatalf("nearest = %d results, want 0", len(got))
	}
	if store.Model() != other {
		t.Fatalf("model = %+v, want %+v", store.Model(), other)
	}
}

func TestFlatStoreDeletePaths(t *testing.T) {
	dir := t.TempDir()
	store := flatOpen(t, dir, flatTestModel())
	flatSeed(t, store)
	if err := store.DeletePaths([]string{"a.md", "missing.md"}); err != nil {
		t.Fatalf("DeletePaths: %v", err)
	}

	hashes := flatOpen(t, dir, flatTestModel()).Hashes()
	if len(hashes) != 1 {
		t.Fatalf("hashes = %v, want only b.md#1", hashes)
	}
	if _, ok := hashes["b.md#1"]; !ok {
		t.Fatalf("b.md#1 was dropped: %v", hashes)
	}
	if err := store.Upsert([]Entry{flatEntry("a.md#1", "a.md", 1, Vector{1, 0, 0})}); err != nil {
		t.Fatalf("Upsert after delete: %v", err)
	}
	if got := len(store.Nearest(Vector{1, 0, 0}, 0)); got != 2 {
		t.Fatalf("nearest = %d results, want 2", got)
	}
}

func TestFlatStoreNearestRanksAndBreaksTiesOnKey(t *testing.T) {
	store := flatOpen(t, t.TempDir(), flatTestModel())
	entries := []Entry{
		flatEntry("z.md#1", "z.md", 1, Vector{1, 0, 0}),
		flatEntry("a.md#1", "a.md", 1, Vector{1, 0, 0}),
		flatEntry("m.md#1", "m.md", 1, Vector{2, 0, 0}),
		flatEntry("q.md#1", "q.md", 1, Vector{0, 1, 0}),
	}
	if err := store.Upsert(entries); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	want := []string{"a.md#1", "m.md#1", "z.md#1", "q.md#1"}
	for run := 0; run < 20; run++ {
		got := store.Nearest(Vector{1, 0, 0}, 0)
		if len(got) != len(want) {
			t.Fatalf("run %d: %d results, want %d", run, len(got), len(want))
		}
		for i, key := range want {
			if got[i].Key != key {
				t.Fatalf("run %d: rank %d = %s, want %s", run, i, got[i].Key, key)
			}
		}
		if got[0].Score <= got[3].Score {
			t.Fatalf("run %d: scores not descending: %v", run, got)
		}
	}
}

func TestFlatStoreNearestLimit(t *testing.T) {
	store := flatOpen(t, t.TempDir(), flatTestModel())
	flatSeed(t, store)
	if got := store.Nearest(Vector{1, 0, 0}, 2); len(got) != 2 {
		t.Fatalf("limit 2 returned %d results", len(got))
	}
	if got := store.Nearest(Vector{1, 0, 0}, 99); len(got) != 3 {
		t.Fatalf("limit above size returned %d results", len(got))
	}
	if got := store.Nearest(Vector{1, 0, 0}, -1); len(got) != 3 {
		t.Fatalf("negative limit returned %d results", len(got))
	}
}

func TestFlatStoreEmptyIsHarmless(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "never-created")
	store := flatOpen(t, dir, flatTestModel())
	if got := store.Nearest(Vector{1, 0, 0}, 5); len(got) != 0 {
		t.Fatalf("empty store returned %d results", len(got))
	}
	if got := store.Nearest(nil, 0); len(got) != 0 {
		t.Fatalf("nil query returned %d results", len(got))
	}
	if len(store.Hashes()) != 0 {
		t.Fatalf("empty store has hashes")
	}
	if err := store.DeletePaths([]string{"a.md"}); err != nil {
		t.Fatalf("DeletePaths on empty store: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("directory created without a write: %v", err)
	}
}

func TestFlatStoreFileModes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "index")
	flatSeed(t, flatOpen(t, dir, flatTestModel()))

	info, err := os.Stat(filepath.Join(dir, flatIndexFile))
	if err != nil {
		t.Fatalf("stat index: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("index mode = %v, want 0600", info.Mode().Perm())
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if dirInfo.Mode().Perm() != 0700 {
		t.Fatalf("dir mode = %v, want 0700", dirInfo.Mode().Perm())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("temp files left behind: %v", entries)
	}
}
