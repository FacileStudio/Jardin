package memory

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range files {
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

func wikiWithPage(t *testing.T) string {
	t.Helper()
	return writeDir(t, map[string]string{
		"tools/mycelium.md": "---\ntitle: Mycelium CLI\ntype: tool\ncreated: 2026-08-19\n" +
			"updated: 2026-08-20\n---\n\n### An older finding\nprose\n",
		"index.md": "# Index\n\n## Bugs\n\n- [a-bug](bugs/a-bug.md): something.\n\n## Tools\n\n" +
			"- [lefthook](tools/lefthook.md): hooks.\n\n## Projects\n\n- [nacelle](projects/nacelle.md): sdk.\n",
		"log.md": "## [2026-08-20] ingest | something earlier\n",
	})
}

func aFinding() Finding {
	return Finding{
		Page:   "tools/mycelium",
		Title:  "Upgrading leaves a shadow copy behind",
		Source: "direct observation",
		Body:   "The old binary stays on PATH and answers first.",
		Log:    "mycelium: the upgrade path leaves a second binary, and which one answers is PATH order",
	}
}

func readWiki(t *testing.T, dataDir, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dataDir, "memory", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestAddDoesEveryPieceOfBookkeepingInOneCall(t *testing.T) {
	dir := wikiWithPage(t)
	day := time.Date(2026, 8, 25, 11, 0, 0, 0, time.UTC)

	res, err := Add(dir, aFinding(), day, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Page != "tools/mycelium.md" || !res.Indexed {
		t.Fatalf("result = %+v, want tools/mycelium.md with an index pointer", res)
	}

	page := readWiki(t, dir, "tools/mycelium.md")
	for _, want := range []string{
		"### Upgrading leaves a shadow copy behind",
		"**Date**: 2026-08-25",
		"**Source**: direct observation",
		"The old binary stays on PATH and answers first.",
		"updated: 2026-08-25",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page is missing %q:\n%s", want, page)
		}
	}
	if strings.Contains(page, "updated: 2026-08-20") {
		t.Error("the old updated: stamp is still there")
	}
	if !strings.Contains(page, "### An older finding") {
		t.Error("the finding that was already on the page is gone")
	}

	index := readWiki(t, dir, "index.md")
	pointer := "- [Mycelium CLI](tools/mycelium.md): Upgrading leaves a shadow copy behind"
	if !strings.Contains(index, pointer) {
		t.Errorf("index is missing %q:\n%s", pointer, index)
	}
	if !strings.Contains(index, "- [lefthook](tools/lefthook.md): hooks.\n"+pointer) {
		t.Errorf("the pointer did not land at the end of the Tools section:\n%s", index)
	}

	history := readWiki(t, dir, "log.md")
	line := "## [2026-08-25] ingest | " + aFinding().Log + "\n"
	if !strings.HasSuffix(history, line) {
		t.Errorf("log.md does not end with %q:\n%s", line, history)
	}
	if !strings.HasPrefix(history, "## [2026-08-20] ingest | something earlier\n") {
		t.Errorf("log.md is not append-only:\n%s", history)
	}
}

func TestAFailingSyncStillLeavesTheFindingOnDisk(t *testing.T) {
	dir := wikiWithPage(t)
	offline := errors.New("dial tcp: connection refused")

	res, err := Add(dir, aFinding(), time.Now(), func() error { return offline })
	if err != nil {
		t.Fatalf("a failing sync must not fail the write: %v", err)
	}
	if !errors.Is(res.SyncErr, offline) {
		t.Fatalf("SyncErr = %v, want %v", res.SyncErr, offline)
	}
	landed := map[string]string{
		"tools/mycelium.md": aFinding().Title,
		"index.md":          "tools/mycelium.md",
		"log.md":            aFinding().Log,
	}
	for rel, want := range landed {
		if !strings.Contains(readWiki(t, dir, rel), want) {
			t.Errorf("%s did not keep %q when the sync failed", rel, want)
		}
	}
}

func TestAddRefusesAPageThatIsNotThere(t *testing.T) {
	dir := wikiWithPage(t)
	f := aFinding()
	f.Page = "tools/not-a-page"

	if _, err := Add(dir, f, time.Now(), nil); err == nil {
		t.Fatal("expected an error for a page the wiki does not have")
	}
	if strings.Contains(readWiki(t, dir, "log.md"), "shadow copy") {
		t.Error("a refused write still touched log.md")
	}
}

func TestAddRefusesFrenchProseBeforeTouchingAnything(t *testing.T) {
	dir := wikiWithPage(t)
	before := readWiki(t, dir, "tools/mycelium.md")
	f := aFinding()
	f.Body = "Le binaire qui est sur le PATH n'est pas celui que la commande vient d'installer."

	if _, err := Add(dir, f, time.Now(), nil); err == nil {
		t.Fatal("expected the English-only check to refuse the write")
	}
	if readWiki(t, dir, "tools/mycelium.md") != before {
		t.Error("the page changed even though the write was refused")
	}
}

func TestAddLeavesAnIndexPointerThatAlreadyExistsAlone(t *testing.T) {
	dir := writeDir(t, map[string]string{
		"tools/mycelium.md": "---\ntitle: Mycelium CLI\ntype: tool\nupdated: 2026-08-20\n---\n\nprose\n",
		"index.md":          "# Index\n\n## Tools\n\n- [mycelium](tools/mycelium.md): the CLI.\n",
		"log.md":            "",
	})

	res, err := Add(dir, aFinding(), time.Now(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Indexed {
		t.Error("a page the index already names must not get a second line")
	}
	if got := strings.Count(readWiki(t, dir, "index.md"), "tools/mycelium.md"); got != 1 {
		t.Errorf("index names the page %d times, want 1", got)
	}
}

func TestAddRefusesAPageWithNoFrontmatterToStamp(t *testing.T) {
	dir := writeDir(t, map[string]string{
		"tools/mycelium.md": "just prose, no header\n",
		"index.md":          "# Index\n",
		"log.md":            "",
	})

	_, err := Add(dir, aFinding(), time.Now(), nil)
	if err == nil || !strings.Contains(err.Error(), "frontmatter") {
		t.Fatalf("err = %v, want a refusal naming the missing frontmatter", err)
	}
}

func TestTheIndexPointerCarriesThePageTitleAndFallsBackToItsSlug(t *testing.T) {
	titled, _ := indexPointer("", "tools/mycelium.md", "Mycelium CLI", "a finding")
	if !strings.Contains(titled, "- [Mycelium CLI](tools/mycelium.md): a finding") {
		t.Errorf("a titled page is not named by its title:\n%s", titled)
	}
	untitled, _ := indexPointer("", "tools/mycelium.md", "", "a finding")
	if !strings.Contains(untitled, "- [mycelium](tools/mycelium.md): a finding") {
		t.Errorf("a page with no title does not fall back to its slug:\n%s", untitled)
	}
}

func TestAStagedFileIsNamedSoSyncWillNotCarryIt(t *testing.T) {
	dir := t.TempDir()
	staged, err := stage(edit{path: filepath.Join(dir, "page.md"), data: "prose"})
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(staged.tmp)
	if !strings.HasSuffix(staged.tmp, ".tmp") {
		t.Fatalf("staged file is %q; internal/sync only skips a %q suffix, so anything "+
			"else is a whole copy of the page published to every machine", staged.tmp, ".tmp")
	}
}

func TestTwoWritersBothLandTheirLogLine(t *testing.T) {
	dir := wikiWithPage(t)
	second := aFinding()
	second.Title = "A second finding"
	second.Log = "the second writer's line"

	done := make(chan error, 2)
	for _, f := range []Finding{aFinding(), second} {
		go func(f Finding) { _, err := Add(dir, f, time.Now(), nil); done <- err }(f)
	}
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}

	history := readWiki(t, dir, "log.md")
	for _, want := range []string{aFinding().Log, second.Log, "something earlier"} {
		if !strings.Contains(history, want) {
			t.Errorf("log.md lost %q, so one writer overwrote the other:\n%s", want, history)
		}
	}
	page := readWiki(t, dir, "tools/mycelium.md")
	for _, want := range []string{"### Upgrading leaves a shadow copy behind", "### A second finding"} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lost %q:\n%s", want, page)
		}
	}
}
