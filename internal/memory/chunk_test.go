package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const samplePage = `---
title: mycelium flow — recorded procedures
type: tool
confidence: high
---

Intro prose before any finding.

### The trust store is lazy
**Date**: 2026-08-18
A pin lands only on the first trust.

### Runs never sync
An explicit prefix exclusion keeps them local.
`

// TestChunksSplitOnFindingBlocks proves the wiki's own `###` convention is the
// retrieval unit, preamble included.
func TestChunksSplitOnFindingBlocks(t *testing.T) {
	chunks := Chunks("tools/mycelium-flow.md", samplePage)
	if len(chunks) != 3 {
		t.Fatalf("want preamble + two findings, got %d: %+v", len(chunks), chunks)
	}
	if chunks[1].Heading != "The trust store is lazy" {
		t.Fatalf("heading not captured: %q", chunks[1].Heading)
	}
	if !strings.Contains(chunks[2].Body, "prefix exclusion") {
		t.Fatalf("body not captured: %q", chunks[2].Body)
	}
}

// TestChunkTextCarriesThePageIdentity is the enrichment that matters: a block
// saying "a pin lands only on the first trust" is meaningless on its own, and
// embeds as noise unless it says which page it came from.
func TestChunkTextCarriesThePageIdentity(t *testing.T) {
	chunks := Chunks("tools/mycelium-flow.md", samplePage)
	text := chunks[1].Text()
	for _, want := range []string{"tools/mycelium-flow", "mycelium flow — recorded procedures", "tool", "The trust store is lazy"} {
		if !strings.Contains(text, want) {
			t.Fatalf("enriched text missing %q:\n%s", want, text)
		}
	}
}

// TestChunksArePureOfContent locks the property an incremental index depends
// on: same bytes in, same chunks out, so a content hash can decide what needs
// re-embedding.
func TestChunksArePureOfContent(t *testing.T) {
	first := Chunks("a.md", samplePage)
	for i := 0; i < 10; i++ {
		again := Chunks("a.md", samplePage)
		if len(first) != len(again) {
			t.Fatal("chunk count is not stable")
		}
		for j := range first {
			if first[j] != again[j] {
				t.Fatalf("chunk %d differs between runs", j)
			}
		}
	}
}

// TestChunksHandleAPageWithNoFrontmatter keeps index.md and log.md from
// vanishing out of the index.
func TestChunksHandleAPageWithNoFrontmatter(t *testing.T) {
	chunks := Chunks("index.md", "# Index\n- a line\n")
	if len(chunks) != 1 || !strings.Contains(chunks[0].Body, "a line") {
		t.Fatalf("want one chunk carrying the body, got %+v", chunks)
	}
}

// TestChunksOverTheRealWiki is the sanity check that the corpus really is
// shaped the way the retrieval design assumes.
func TestChunksOverTheRealWiki(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	dir := filepath.Join(home, ".mycelium", "memory")
	if _, statErr := os.Stat(dir); statErr != nil {
		t.Skip("no wiki on this machine")
	}
	docs, err := readDocs(dir)
	if err != nil {
		t.Fatal(err)
	}
	total, empty := 0, 0
	for _, d := range docs {
		for _, c := range Chunks(d.path, d.body) {
			total++
			if strings.TrimSpace(c.Text()) == "" {
				empty++
			}
		}
	}
	t.Logf("%d pages → %d chunks", len(docs), total)
	if total < len(docs) {
		t.Fatalf("every page must yield at least one chunk: %d pages, %d chunks", len(docs), total)
	}
	if empty > 0 {
		t.Fatalf("%d chunks would embed as empty text", empty)
	}
}

// TestChunksSplitOversizedBlocks covers the silent failure this bounds:
// embedding models truncate rather than error, so log.md — one 190k-character
// block with no headings — indexed as its opening paragraph while every status
// reported it healthy.
func TestChunksSplitOversizedBlocks(t *testing.T) {
	var body strings.Builder
	body.WriteString("# Log\n")
	for i := 0; i < 400; i++ {
		body.WriteString("## [2026-08-18] ingest | a reasonably long log line about something\n")
	}
	chunks := Chunks("log.md", body.String())
	if len(chunks) < 2 {
		t.Fatalf("an oversized page must split, got %d chunk(s)", len(chunks))
	}
	for _, c := range chunks {
		if len(c.Text()) > MaxChunkChars*2 {
			t.Fatalf("a part is still %d chars, over the bound", len(c.Text()))
		}
		if c.Header == "" {
			t.Fatal("every part must keep the header that says where it came from")
		}
	}
	lines := map[int]bool{}
	for _, c := range chunks {
		if lines[c.Line] {
			t.Fatalf("parts must carry distinct line numbers, %d repeats", c.Line)
		}
		lines[c.Line] = true
	}
}

// TestChunksLeaveNormalBlocksAlone keeps the split from fragmenting the ordinary
// finding, which is what retrieval is tuned on.
func TestChunksLeaveNormalBlocksAlone(t *testing.T) {
	chunks := Chunks("tools/x.md", samplePage)
	if len(chunks) != 3 {
		t.Fatalf("a normal page must not fragment, got %d", len(chunks))
	}
}
