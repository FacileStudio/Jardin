package sessions

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func writeNacelleTranscript(t *testing.T, dir, name string, lines []string) string {
	t.Helper()
	sessions := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessions, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sessions, name+".jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, l := range lines {
		if _, err := f.WriteString(l + "\n"); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func nacelleTurn(ts string, in, out int64, cost float64) string {
	return `{"v":1,"ts":"` + ts + `","kind":"turn","usage":{"input_tokens":` +
		strconv.FormatInt(in, 10) + `,"output_tokens":` + strconv.FormatInt(out, 10) +
		`,"cost":` + strconv.FormatFloat(cost, 'f', -1, 64) + `}}`
}

func TestCollectNacelleCountsTurnsOnly(t *testing.T) {
	dir := t.TempDir()
	writeNacelleTranscript(t, dir, "sess1", []string{
		nacelleTurn("2026-08-24T10:00:00Z", 100, 50, 0.01),
		`{"v":1,"ts":"2026-08-24T10:01:00Z","kind":"text","text":"hello"}`,
		`{"v":1,"ts":"2026-08-24T10:02:00Z","kind":"done","usage":{"input_tokens":100,"output_tokens":50,"cost":0.01}}`,
	})

	state := newScanState()
	events, err := collectNacelle(dir, state)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event (turn only), got %d", len(events))
	}
	e := events[0]
	if e.Agent != "nacelle" || e.TokensIn != 100 || e.TokensOut != 50 || e.CostTotal != 0.01 {
		t.Fatalf("unexpected event: %+v", e)
	}
}

func TestCollectNacelleResumesFromOffset(t *testing.T) {
	dir := t.TempDir()
	path := writeNacelleTranscript(t, dir, "sess1", []string{
		nacelleTurn("2026-08-24T10:00:00Z", 10, 5, 0.001),
	})

	state := newScanState()
	if _, err := collectNacelle(dir, state); err != nil {
		t.Fatal(err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(nacelleTurn("2026-08-24T10:05:00Z", 20, 8, 0.002) + "\n")
	f.Close()

	events, err := collectNacelle(dir, state)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].TokensIn != 20 {
		t.Fatalf("expected only the new turn, got %+v", events)
	}
}

func TestCollectNacelleIgnoresGarbage(t *testing.T) {
	dir := t.TempDir()
	writeNacelleTranscript(t, dir, "sess1", []string{
		`not json at all`,
		`{"v":1,"kind":"turn"}`,
		nacelleTurn("2026-08-24T10:00:00Z", 1, 2, 0.0001),
	})
	state := newScanState()
	events, err := collectNacelle(dir, state)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}

// TestTailNacelleReadsAFinalLineWithoutNewline covers the end of a session that
// stopped mid-flush. collectNacelle skips a file once its size stops changing,
// so a complete record with no trailing newline would never be counted again.
func TestTailNacelleReadsAFinalLineWithoutNewline(t *testing.T) {
	dir := t.TempDir()
	path := writeNacelleTranscript(t, dir, "sess1", []string{nacelleTurn("2026-08-24T10:00:00Z", 100, 50, 0.01)})
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(nacelleTurn("2026-08-24T10:05:00Z", 7, 3, 0.02)); err != nil {
		t.Fatal(err)
	}
	f.Close()

	events, offset, err := tailNacelle(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].TokensIn != 7 {
		t.Fatalf("the unterminated final record was dropped: %+v", events)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if offset != info.Size() {
		t.Fatalf("offset = %d, want the whole file at %d", offset, info.Size())
	}
	again, _, err := tailNacelle(path, offset)
	if err != nil || len(again) != 0 {
		t.Fatalf("a consumed record must not be counted twice: %+v %v", again, err)
	}
}

// TestTailNacelleWaitsForATornWrite is the other half: a half-written record is
// not JSON, so the offset must stay before it until the rest lands.
func TestTailNacelleWaitsForATornWrite(t *testing.T) {
	dir := t.TempDir()
	path := writeNacelleTranscript(t, dir, "sess1", []string{nacelleTurn("2026-08-24T10:00:00Z", 100, 50, 0.01)})
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	whole := info.Size()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	torn := nacelleTurn("2026-08-24T10:05:00Z", 7, 3, 0.02)
	if _, err := f.WriteString(torn[:len(torn)/2]); err != nil {
		t.Fatal(err)
	}
	f.Close()

	events, offset, err := tailNacelle(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("a torn record must not be counted: %+v", events)
	}
	if offset != whole {
		t.Fatalf("offset = %d, want it held at %d before the torn write", offset, whole)
	}
}
