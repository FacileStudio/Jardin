package reports

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeSource(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	return path
}

func TestAddRoundTripsItsMetadataHTML(t *testing.T) {
	dir := t.TempDir()
	src := writeSource(t, dir, "in.html", "<html><head><title>Suite drift</title></head><body>hi</body></html>")
	now := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)

	rep, err := Add(dir, Request{Source: src, Machine: "ruche"}, now)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if !strings.HasPrefix(rep.ID, "2026-08-26-suite-drift-") {
		t.Fatalf("id from <title>: got %q, want prefix 2026-08-26-suite-drift-", rep.ID)
	}

	back, err := Find(dir, "suite-drift")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if back.Title != "Suite drift" || back.Machine != "ruche" {
		t.Fatalf("metadata lost: %+v", back)
	}
	if !back.Created.Equal(now) || !back.Expires.Equal(now.Add(DefaultTTL)) {
		t.Fatalf("times lost: created %v expires %v", back.Created, back.Expires)
	}
}

func TestAddRoundTripsItsMetadataMarkdown(t *testing.T) {
	dir := t.TempDir()
	src := writeSource(t, dir, "spec.md", "# DMS Architecture\n\nSome body text.")
	now := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)

	rep, err := Add(dir, Request{Source: src, Machine: "lucy"}, now)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if !strings.HasPrefix(rep.ID, "2026-08-27-dms-architecture-") {
		t.Fatalf("id from markdown heading: got %q, want prefix 2026-08-27-dms-architecture-", rep.ID)
	}

	back, err := Find(dir, rep.ID)
	if err != nil {
		t.Fatalf("find exact: %v", err)
	}
	if back.Title != "DMS Architecture" || back.Machine != "lucy" {
		t.Fatalf("metadata lost: %+v", back)
	}

	raw, err := os.ReadFile(back.Path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !strings.Contains(string(raw), "type: artifact") {
		t.Fatalf("frontmatter missing type: artifact in %s", string(raw))
	}
}

func TestReAddingReplacesInPlaceWithoutStackingTags(t *testing.T) {
	dir := t.TempDir()
	page := "<html><head><title>Suite drift</title></head><body>%s</body></html>"
	now := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)

	src := writeSource(t, dir, "a.html", strings.Replace(page, "%s", "first", 1))
	firstRep, err := Add(dir, Request{Source: src, Machine: "ruche"}, now)
	if err != nil {
		t.Fatalf("first add: %v", err)
	}
	stamped, err := os.ReadFile(firstRep.Path)
	if err != nil {
		t.Fatalf("read stamped: %v", err)
	}
	again := writeSource(t, dir, "b.html", string(stamped))
	if _, err := Add(dir, Request{Source: again, Title: firstRep.ID, Machine: "lucy"}, now); err != nil {
		t.Fatalf("second add: %v", err)
	}

	all, err := List(dir)
	if err != nil || len(all) != 1 {
		t.Fatalf("want exactly one report, got %d (%v)", len(all), err)
	}
	final, err := os.ReadFile(all[0].Path)
	if err != nil {
		t.Fatalf("read final: %v", err)
	}
	if n := strings.Count(string(final), `name="mycelium-created"`); n != 1 {
		t.Fatalf("metadata stacked: %d created tags", n)
	}
	if all[0].Machine != "lucy" {
		t.Fatalf("re-add kept the old machine: %q", all[0].Machine)
	}
}

func TestSweepTakesExpiredAndLeavesPinned(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	stale := writeSource(t, dir, "stale.html", "<title>Stale</title>")
	kept := writeSource(t, dir, "kept.html", "<title>Kept</title>")

	staleRep, err := Add(dir, Request{Source: stale, Expires: now.Add(-time.Hour)}, now)
	if err != nil {
		t.Fatalf("add stale: %v", err)
	}
	keptRep, err := Add(dir, Request{Source: kept, Pinned: true}, now)
	if err != nil {
		t.Fatalf("add kept: %v", err)
	}

	swept, err := Sweep(dir, now)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if len(swept) != 1 || swept[0] != staleRep.ID {
		t.Fatalf("swept the wrong set: %v", swept)
	}
	all, _ := List(dir)
	if len(all) != 1 || all[0].ID != keptRep.ID {
		t.Fatalf("pinned report did not survive: %+v", all)
	}
}

func TestExternalRefsNamesOnlyWhatCannotResolveFromDisk(t *testing.T) {
	page := `<link href="style.css"><img src="data:image/png;base64,AAA">` +
		`<a href="#top"><script src="https://cdn.example/x.js"></script><img src="./logo.png">`

	refs := ExternalRefs([]byte(page))

	want := map[string]bool{"style.css": true, "./logo.png": true}
	if len(refs) != len(want) {
		t.Fatalf("got %v, want %v", refs, want)
	}
	for _, ref := range refs {
		if !want[ref] {
			t.Fatalf("flagged a reference that resolves: %q", ref)
		}
	}
}

