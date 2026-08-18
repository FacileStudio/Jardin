package ui

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestSpinnerIsSilentOnNonTerminal(t *testing.T) {
	out := &syncBuffer{}
	s := NewSpinner(out, "searching")
	s.Start()
	time.Sleep(spinnerDelay + 3*spinnerInterval)
	s.Stop()
	s.Stop()

	if out.String() != "" {
		t.Fatalf("spinner wrote %q to a non-terminal writer, want nothing", out.String())
	}
}

func TestSpinnerIsSilentInsideTheStartupDelay(t *testing.T) {
	out := &syncBuffer{}
	s := newSpinner(out, "searching", true)
	s.Start()
	s.Stop()

	if out.String() != "" {
		t.Fatalf("spinner wrote %q before its startup delay, want nothing", out.String())
	}
}

func TestSpinnerDrawsThenClearsItsLine(t *testing.T) {
	out := &syncBuffer{}
	s := newSpinner(out, "searching", true)
	s.Start()
	time.Sleep(spinnerDelay + 2*spinnerInterval)
	s.Stop()

	written := out.String()
	if !strings.Contains(written, "searching") {
		t.Fatalf("spinner wrote %q, want it to contain its label", written)
	}
	if !strings.HasSuffix(written, clearLine) {
		t.Errorf("spinner output ends with %q, want it to end by clearing its line", written)
	}
}

func TestSpinnerStopWithoutStart(t *testing.T) {
	newSpinner(&syncBuffer{}, "searching", true).Stop()
	NewSpinner(&syncBuffer{}, "searching").Stop()
}
