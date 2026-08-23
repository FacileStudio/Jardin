package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func tempCorpus(t *testing.T, pages map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range pages {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func rankOf(t *testing.T, dir, query, page string) int {
	t.Helper()
	results, err := SearchChunks(dir, query)
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range results {
		if r.Path == filepath.FromSlash(page) {
			return i + 1
		}
	}
	return 0
}

// TestDateReadsTheProseLineTheWikiActuallyWrites is the whole reason this is
// not dead code: every finding in the live wiki dates itself in prose and none
// of them fills in the metadata block, so a ranker reading only the block would
// have nothing to read.
func TestDateReadsTheProseLineTheWikiActuallyWrites(t *testing.T) {
	chunks := Chunks("tools/filet.md", "### a finding\n**Date**: 2026-08-21\n**Source**: direct\nbody\n")
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
	want := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	if got := chunks[0].Date(); !got.Equal(want) {
		t.Errorf("Date() = %v, want %v", got, want)
	}
}

func TestDatePrefersConfirmedThenBlockThenProse(t *testing.T) {
	body := "**Date**: 2026-01-01\n"
	cases := []struct {
		name string
		meta Meta
		want string
	}{
		{"prose only", Meta{}, "2026-01-01"},
		{"block wins over prose", Meta{Date: "2026-02-02"}, "2026-02-02"},
		{"confirmed wins over both", Meta{Date: "2026-02-02", Confirmed: "2026-03-03"}, "2026-03-03"},
	}
	for _, c := range cases {
		got := Chunk{Body: body, Meta: c.meta}.Date().Format(dayLayout)
		if got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

// TestDateTakesTheLaterOfTwo covers the five lines in the live wiki shaped
// "2026-08-22, updated 2026-08-23", where the second date is the one that says
// how current the claim is.
func TestDateTakesTheLaterOfTwo(t *testing.T) {
	c := Chunk{Body: "**Date**: 2026-08-22, updated 2026-08-23\n"}
	if got := c.Date().Format(dayLayout); got != "2026-08-23" {
		t.Errorf("got %s, want the later date", got)
	}
}

// TestUndatedChunksAreNotPenalised keeps a freshness signal from turning into a
// silent ranking change: page preambles carry no date, and treating absent as
// ancient would push every one of them down for no reason.
func TestUndatedChunksAreNotPenalised(t *testing.T) {
	if got := freshness(time.Time{}, time.Now()); got != 1 {
		t.Errorf("an undated chunk weighs %v, want 1", got)
	}
}

func TestFreshnessDecaysSlowlyAndNeverBelowTheFloor(t *testing.T) {
	now := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		age  time.Duration
		want float64
	}{
		{0, 1},
		{5 * 24 * time.Hour, 0.997},
		{freshnessHalfLife, freshnessFloor + (1-freshnessFloor)/2},
		{40 * freshnessHalfLife, freshnessFloor},
	}
	for _, c := range cases {
		got := freshness(now.Add(-c.age), now)
		if diff := got - c.want; diff > 0.002 || diff < -0.002 {
			t.Errorf("age %v: got %.4f, want about %.4f", c.age, got, c.want)
		}
		if got < freshnessFloor || got > 1 {
			t.Errorf("age %v: %.4f is outside the band", c.age, got)
		}
	}
}

// TestRecencyOnlyBreaksTies is the guarantee the narrow band exists for. Two
// findings with the same words rank by date; a finding that matches better than
// another still wins however old it is.
//
// The filenames matter: equal scores fall back to sorting by path, so the stale
// page is named to win that tiebreak. Without it the test passes whether or not
// recency does anything at all.
func TestRecencyOnlyBreaksTies(t *testing.T) {
	old := time.Now().AddDate(-3, 0, 0).Format(dayLayout)
	fresh := time.Now().Format(dayLayout)
	dir := tempCorpus(t, map[string]string{
		"tools/a-stale.md":   page("### the flock is exclusive\n**Date**: "+old+"\nflock exclusive lock\n", "stale"),
		"tools/z-current.md": page("### the flock is exclusive\n**Date**: "+fresh+"\nflock exclusive lock\n", "current"),
		"tools/a-better.md": page("### the flock is exclusive and non blocking\n**Date**: "+old+
			"\nflock exclusive lock non blocking retry bounded\n", "better"),
	})
	if got := rankOf(t, dir, "flock exclusive lock", "tools/z-current.md"); got != 1 {
		t.Errorf("the fresher of two equal findings ranks %d, want 1", got)
	}
	if got := rankOf(t, dir, "flock exclusive non blocking retry bounded", "tools/a-better.md"); got != 1 {
		t.Errorf("a three-year-old but far better match ranks %d, want 1", got)
	}
}

// TestStruckClaimsLeaveTheIndex is the failure SPEC.md cites by name: a claim
// the page itself contradicts two lines further down was matching queries.
// The page keeps every word; only what is indexed changes.
func TestStruckClaimsLeaveTheIndex(t *testing.T) {
	body := "### where the toolchain lives\n**Date**: 2026-08-21\n" +
		"~~Ruche has no toolchain: no mise, no golang package, nothing installed.~~ " +
		"[SUPERSEDED by: direct observation] Ruche has go1.26.4 under a local prefix.\n"
	chunks := Chunks("projects/nacelle.md", body)
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
	if strings.Contains(chunks[0].Text(), "golang package") {
		t.Error("the struck claim is still indexed")
	}
	if !strings.Contains(chunks[0].Body, "golang package") {
		t.Error("the struck claim was deleted from the page, not just from the index")
	}
	if !strings.Contains(chunks[0].Text(), "go1.26.4") {
		t.Error("the correction was stripped along with the claim it corrects")
	}

	dir := tempCorpus(t, map[string]string{"projects/nacelle.md": page(body, "nacelle")})
	if got := rankOf(t, dir, "mise golang package nothing installed", "projects/nacelle.md"); got != 0 {
		t.Errorf("a query made only of struck words still answers, at rank %d", got)
	}
	if got := rankOf(t, dir, "ruche go1.26.4 local prefix", "projects/nacelle.md"); got != 1 {
		t.Errorf("the correction ranks %d, want 1", got)
	}
}

func TestUnpairedTildesLeaveTheBodyAlone(t *testing.T) {
	body := "a claim with ~~ one stray marker and words after it"
	if got := dropStruck(body); got != body {
		t.Errorf("dropStruck ate the body: %q", got)
	}
}

// TestSupersededChunkRanksBelowItsReplacement is SPEC.md step 10's own exit
// criterion, for the day a finding fills in the metadata block. No page in the
// live wiki writes one yet.
func TestSupersededChunkRanksBelowItsReplacement(t *testing.T) {
	dir := tempCorpus(t, map[string]string{
		"conventions/surface.md": page("### the rule is nothing below crud\n"+
			"<!-- id: surface-nothing-below-crud\n     date: 2026-08-20 -->\n"+
			"The line is drawn at crud operations.\n\n"+
			"### the rule is domain versus storage\n"+
			"<!-- id: surface-domain-vs-storage\n     date: 2026-08-20\n"+
			"     supersedes: surface-nothing-below-crud -->\n"+
			"The line is drawn at crud operations, and it is domain versus storage.\n", "surface"),
	})
	results, err := SearchChunks(dir, "the line is drawn at crud operations")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) < 2 {
		t.Fatalf("got %d results, want both findings", len(results))
	}
	if !strings.Contains(results[0].Content, "domain versus storage") {
		t.Errorf("the replacement ranks below the claim it replaces: %q first", results[0].Content)
	}
}

func page(body, title string) string {
	return fmt.Sprintf("---\ntitle: %s\ntype: tool\n---\n\n%s", title, body)
}
