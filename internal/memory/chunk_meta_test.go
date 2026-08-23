package memory

import (
	"strings"
	"testing"
)

const metaPage = `---
title: the agent surface
type: convention
---

### The line is domain versus storage
<!-- id: agent-surface-domain-vs-storage
     date: 2026-08-22
     source: decided with the user
     confirmed: 2026-08-22
     supersedes: agent-surface-nothing-below-crud -->
The wiki is a domain, not a filesystem.
`

// TestChunkMetaPopulatesEveryField is what the whole block exists for: a
// superseded claim can only stop outranking its correction if the ranker can
// read the supersession as a field instead of guessing at prose.
func TestChunkMetaPopulatesEveryField(t *testing.T) {
	chunks := Chunks("conventions/agent-surface.md", metaPage)
	if len(chunks) != 1 {
		t.Fatalf("want one finding, got %d: %+v", len(chunks), chunks)
	}
	want := Meta{
		ID:         "agent-surface-domain-vs-storage",
		Date:       "2026-08-22",
		Source:     "decided with the user",
		Confirmed:  "2026-08-22",
		Supersedes: "agent-surface-nothing-below-crud",
	}
	if chunks[0].Meta != want {
		t.Fatalf("metadata not parsed:\ngot  %+v\nwant %+v", chunks[0].Meta, want)
	}
}

// TestChunkTextDropsTheMetadataBlock keeps the block out of what gets embedded.
// Text() feeds both the vector and the BM25 index, so every `<!--` left in it
// is paid for on every query, forever, in exchange for nothing.
func TestChunkTextDropsTheMetadataBlock(t *testing.T) {
	text := Chunks("conventions/agent-surface.md", metaPage)[0].Text()
	for _, unwanted := range []string{"<!--", "-->", "agent-surface-domain-vs-storage", "decided with the user"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("embedded text still carries %q:\n%s", unwanted, text)
		}
	}
	if !strings.Contains(text, "The wiki is a domain, not a filesystem.") {
		t.Fatalf("the finding's prose went out with the block:\n%s", text)
	}
}

// TestChunksWithoutMetaAreUnchanged is the promise made to the existing corpus.
// Not one page carries a block today, so any difference here is a regression
// the whole wiki would eat at once.
func TestChunksWithoutMetaAreUnchanged(t *testing.T) {
	chunks := Chunks("tools/mycelium-flow.md", samplePage)
	if len(chunks) != 3 {
		t.Fatalf("a page with no block must chunk exactly as before, got %d", len(chunks))
	}
	for _, c := range chunks {
		if c.Meta != (Meta{}) {
			t.Fatalf("metadata invented for %q: %+v", c.Heading, c.Meta)
		}
	}
	if !strings.HasPrefix(chunks[1].Body, "**Date**: 2026-08-18") {
		t.Fatalf("the body was rewritten: %q", chunks[1].Body)
	}
}

// TestChunkMetaReadsTheSingleLineForm covers the shape a human actually types
// when there is one field to record. A grammar that only accepts the five-line
// block is a grammar nobody uses.
func TestChunkMetaReadsTheSingleLineForm(t *testing.T) {
	chunks := Chunks("bugs/x.md", "### A finding\n<!-- id: single-line -->\nThe body.\n")
	if chunks[0].Meta.ID != "single-line" {
		t.Fatalf("single-line block not parsed: %+v", chunks[0].Meta)
	}
	if chunks[0].Body != "The body." {
		t.Fatalf("body should be the prose alone, got %q", chunks[0].Body)
	}
}

// TestUnrecognisedCommentsStayInTheBody protects a marker that already carries
// meaning: a line holding `<!-- lang:fr -->` is exempt from the English-only
// check, and stripping it as metadata would silently revoke the exemption.
func TestUnrecognisedCommentsStayInTheBody(t *testing.T) {
	chunks := Chunks("conventions/x.md", "### Une note\n<!-- lang:fr -->\nLe corps de la note.\n")
	if chunks[0].Meta != (Meta{}) {
		t.Fatalf("a comment with no recognised field is not metadata: %+v", chunks[0].Meta)
	}
	if !strings.Contains(chunks[0].Body, "<!-- lang:fr -->") {
		t.Fatalf("the marker was eaten: %q", chunks[0].Body)
	}
}

// TestUnterminatedCommentLeavesTheBodyAlone stops a typo from deleting a
// finding. A missing `-->` is a mistake in one line, and it must not cost the
// prose underneath it.
func TestUnterminatedCommentLeavesTheBodyAlone(t *testing.T) {
	chunks := Chunks("bugs/x.md", "### A finding\n<!-- id: never-closed\nThe body continues.\n")
	if chunks[0].Meta != (Meta{}) {
		t.Fatalf("an unclosed comment is not metadata: %+v", chunks[0].Meta)
	}
	for _, want := range []string{"<!-- id: never-closed", "The body continues."} {
		if !strings.Contains(chunks[0].Body, want) {
			t.Fatalf("body lost %q: %q", want, chunks[0].Body)
		}
	}
}

// TestSplitPartsInheritMeta matters because a long finding is exactly the kind
// that gets superseded. If only the first part carried the fields, the tail of
// a stale block would keep ranking as if it were current.
func TestSplitPartsInheritMeta(t *testing.T) {
	var page strings.Builder
	page.WriteString("### A long finding\n<!-- id: long-finding\n     date: 2026-08-22 -->\n")
	for i := 0; i < 60; i++ {
		page.WriteString("A reasonably long line of prose that pushes this block past the chunk bound.\n")
	}
	chunks := Chunks("syntheses/long.md", page.String())
	if len(chunks) < 2 {
		t.Fatalf("want an oversized block to split, got %d", len(chunks))
	}
	for i, c := range chunks {
		if c.Meta.ID != "long-finding" || c.Meta.Date != "2026-08-22" {
			t.Fatalf("part %d lost its metadata: %+v", i, c.Meta)
		}
		if strings.Contains(c.Text(), "<!--") {
			t.Fatalf("part %d still embeds the comment:\n%s", i, c.Text())
		}
	}
}

// TestProseAfterTheClosingMarkerSurvives is the one failure mode of this parser
// that nobody would notice: the page still renders correctly in an editor, so
// the only symptom is a sentence that can never be found by search again.
func TestProseAfterTheClosingMarkerSurvives(t *testing.T) {
	cases := []struct{ name, page, want string }{
		{"single line", "### A finding\n<!-- id: x --> The body starts here.\nMore body.\n", "The body starts here."},
		{"block", "### A finding\n<!-- id: x\n     date: 2026-08-22 --> Trailing prose.\nMore.\n", "Trailing prose."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := Chunks("bugs/x.md", tc.page)[0]
			if c.Meta.ID != "x" {
				t.Fatalf("metadata should still parse: %+v", c.Meta)
			}
			if !strings.HasPrefix(c.Body, tc.want) {
				t.Fatalf("prose after --> was dropped, body is %q", c.Body)
			}
			if strings.Contains(c.Body, "-->") {
				t.Fatalf("the marker itself leaked into the body: %q", c.Body)
			}
		})
	}
}
