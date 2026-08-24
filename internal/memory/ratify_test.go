package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func ratifyDir(t *testing.T, pages map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range pages {
		path := filepath.Join(dir, "memory", filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func standardPage(body string) string {
	return "---\ntitle: A standard\ntype: standard\n---\n\n" + body
}

func standingOf(t *testing.T, dataDir, path string) Standing {
	t.Helper()
	pages, err := NormativePages(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, page := range pages {
		if page.Path == path {
			return page.Standing
		}
	}
	t.Fatalf("no normative page %q in %v", path, pages)
	return ""
}

func ratify(t *testing.T, dataDir string, paths ...string) {
	t.Helper()
	if err := Ratify(dataDir, "lucy", time.Now(), paths); err != nil {
		t.Fatal(err)
	}
}

func TestOnlyStandardTypedPagesAreNormative(t *testing.T) {
	dir := ratifyDir(t, map[string]string{
		"standards/cli.md":  standardPage("rule one"),
		"bugs/a-bug.md":     "---\ntitle: A bug\ntype: bug\n---\n\nnote",
		"no-frontmatter.md": "just prose",
	})
	pages, err := NormativePages(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || pages[0].Path != "standards/cli.md" {
		t.Fatalf("expected only standards/cli.md, got %v", pages)
	}
}

func TestANewStandardIsUnratifiedAndThatIsNotAFailure(t *testing.T) {
	dir := ratifyDir(t, map[string]string{"standards/cli.md": standardPage("rule one")})
	pages, err := NormativePages(dir)
	if err != nil {
		t.Fatal(err)
	}
	if pages[0].Standing != Unratified {
		t.Fatalf("standing = %q, want %q", pages[0].Standing, Unratified)
	}
	if !pages[0].OK() {
		t.Fatal("an unratified page must not be reported as a failure")
	}
}

func TestRatifyThenEditReportsChanged(t *testing.T) {
	dir := ratifyDir(t, map[string]string{"standards/cli.md": standardPage("rule one")})
	ratify(t, dir, "standards/cli.md")
	if got := standingOf(t, dir, "standards/cli.md"); got != Ratified {
		t.Fatalf("after ratify standing = %q, want %q", got, Ratified)
	}
	page := filepath.Join(dir, "memory", "standards", "cli.md")
	if err := os.WriteFile(page, []byte(standardPage("rule one, reversed")), 0644); err != nil {
		t.Fatal(err)
	}
	if got := standingOf(t, dir, "standards/cli.md"); got != Changed {
		t.Fatalf("after edit standing = %q, want %q", got, Changed)
	}
}

// A pin names content, not an event, so putting the accepted bytes back is
// enough to be ratified again. This is what makes `mycelium memory revert` a
// complete answer to a bad edit rather than half of one.
func TestRevertingToTheAcceptedContentRatifiesAgain(t *testing.T) {
	accepted := standardPage("rule one")
	dir := ratifyDir(t, map[string]string{"standards/cli.md": accepted})
	ratify(t, dir, "standards/cli.md")
	page := filepath.Join(dir, "memory", "standards", "cli.md")

	if err := os.WriteFile(page, []byte(standardPage("wrong")), 0644); err != nil {
		t.Fatal(err)
	}
	if got := standingOf(t, dir, "standards/cli.md"); got != Changed {
		t.Fatalf("standing = %q, want %q", got, Changed)
	}
	if err := os.WriteFile(page, []byte(accepted), 0644); err != nil {
		t.Fatal(err)
	}
	if got := standingOf(t, dir, "standards/cli.md"); got != Ratified {
		t.Fatalf("after revert standing = %q, want %q", got, Ratified)
	}
}

// Reverting to a version nobody accepted here is still CHANGED. The pin is a
// record of what a human read, not of the last state the page happened to be in.
func TestRevertingToAnUnacceptedVersionStaysChanged(t *testing.T) {
	dir := ratifyDir(t, map[string]string{"standards/cli.md": standardPage("rule two")})
	ratify(t, dir, "standards/cli.md")
	page := filepath.Join(dir, "memory", "standards", "cli.md")
	if err := os.WriteFile(page, []byte(standardPage("rule one")), 0644); err != nil {
		t.Fatal(err)
	}
	if got := standingOf(t, dir, "standards/cli.md"); got != Changed {
		t.Fatalf("standing = %q, want %q", got, Changed)
	}
}

func TestADeletedStandardIsMissingUntilForgotten(t *testing.T) {
	dir := ratifyDir(t, map[string]string{"standards/cli.md": standardPage("rule one")})
	ratify(t, dir, "standards/cli.md")
	if err := os.Remove(filepath.Join(dir, "memory", "standards", "cli.md")); err != nil {
		t.Fatal(err)
	}
	if got := standingOf(t, dir, "standards/cli.md"); got != Missing {
		t.Fatalf("standing = %q, want %q", got, Missing)
	}
	if err := Forget(dir, []string{"standards/cli.md"}); err != nil {
		t.Fatal(err)
	}
	pages, err := NormativePages(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 0 {
		t.Fatalf("expected no normative pages after forget, got %v", pages)
	}
}

func TestTheRecordNamesWhoAcceptedItAndWhen(t *testing.T) {
	dir := ratifyDir(t, map[string]string{"standards/cli.md": standardPage("rule one")})
	when := time.Date(2026, 8, 24, 11, 0, 0, 0, time.UTC)
	if err := Ratify(dir, "lucy", when, []string{"standards/cli.md"}); err != nil {
		t.Fatal(err)
	}
	pages, err := NormativePages(dir)
	if err != nil {
		t.Fatal(err)
	}
	if pages[0].Machine != "lucy" || pages[0].On != "2026-08-24" {
		t.Fatalf("record = %+v, want lucy on 2026-08-24", pages[0])
	}
}

// A machine that has never ratified anything must not pay for the check on
// every search, so the empty case answers without reading the wiki at all.
func TestChangedPagesIsEmptyAndCheapWithNoPins(t *testing.T) {
	dir := ratifyDir(t, map[string]string{"standards/cli.md": standardPage("rule one")})
	changed, err := ChangedPages(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 {
		t.Fatalf("changed = %v, want none", changed)
	}
}

func TestChangedPagesMarksOnlyThePageThatMoved(t *testing.T) {
	dir := ratifyDir(t, map[string]string{
		"standards/cli.md":  standardPage("rule one"),
		"standards/docs.md": standardPage("rule two"),
	})
	ratify(t, dir, "standards/cli.md", "standards/docs.md")
	page := filepath.Join(dir, "memory", "standards", "cli.md")
	if err := os.WriteFile(page, []byte(standardPage("rule one, edited")), 0644); err != nil {
		t.Fatal(err)
	}
	changed, err := ChangedPages(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !changed["standards/cli.md"] || changed["standards/docs.md"] {
		t.Fatalf("changed = %v, want only standards/cli.md", changed)
	}
}

func TestRatifyRefusesAPathOutsideTheWiki(t *testing.T) {
	dir := ratifyDir(t, map[string]string{"standards/cli.md": standardPage("rule one")})
	if err := Ratify(dir, "lucy", time.Now(), []string{"../rules/20-memory.md"}); err == nil {
		t.Fatal("expected a refusal for a path leaving the wiki")
	}
}

// An unreadable store must say it knows nothing, not report every page as
// unratified: the two look identical downstream and only one is a problem.
func TestACorruptStoreIsAnErrorNotAnEmptyOne(t *testing.T) {
	dir := ratifyDir(t, map[string]string{"standards/cli.md": standardPage("rule one")})
	if err := os.WriteFile(pinsPath(dir), []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NormativePages(dir); err == nil {
		t.Fatal("expected a corrupt store to be an error")
	}
	if _, err := ChangedPages(dir); err == nil {
		t.Fatal("expected a corrupt store to be an error")
	}
}

// The store sits beside the tree and never inside a page, and sync skips every
// path starting with a dot. internal/sync holds the other half of this.
func TestTheStoreIsADotfileBesideTheTree(t *testing.T) {
	dir := ratifyDir(t, nil)
	base := filepath.Base(pinsPath(dir))
	if !strings.HasPrefix(base, ".") {
		t.Fatalf("store %q must be a dotfile or sync will carry it", base)
	}
	if filepath.Dir(pinsPath(dir)) != dir {
		t.Fatalf("store must sit in the data dir, not in memory/")
	}
}

func TestRatifyingDoesNotTouchThePage(t *testing.T) {
	body := standardPage("rule one")
	dir := ratifyDir(t, map[string]string{"standards/cli.md": body})
	ratify(t, dir, "standards/cli.md")
	after, err := os.ReadFile(filepath.Join(dir, "memory", "standards", "cli.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != body {
		t.Fatal("ratifying must not write anything into the page")
	}
}
