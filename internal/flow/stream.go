package flow

import (
	"bytes"
	"io"
	"strings"
	"sync"
)

// This file is the step output path: what is mirrored to the terminal while
// a step runs, and what is kept for the artifact.

type sink struct {
	mu sync.Mutex
	w  io.Writer
}

func newSink(w io.Writer) *sink {
	if w == nil {
		return nil
	}
	return &sink{w: w}
}

func (s *sink) writeString(text string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = io.WriteString(s.w, text)
}

type capture struct {
	mu        sync.Mutex
	buf       []byte
	pending   []byte
	truncated bool
	stream    *sink
	prefix    string
	redact    func(string) string
}

func newCapture(stream *sink, prefix string, redact func(string) string) *capture {
	return &capture{stream: stream, prefix: prefix, redact: redact}
}

func (c *capture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if room := MaxStreamBytes - len(c.buf); room > 0 {
		if len(p) > room {
			c.buf = append(c.buf, p[:room]...)
			c.truncated = true
		} else {
			c.buf = append(c.buf, p...)
		}
	} else if len(p) > 0 {
		c.truncated = true
	}
	c.mirrorLines(p)
	return len(p), nil
}

func (c *capture) mirrorLines(p []byte) {
	if c.stream == nil || len(p) == 0 {
		return
	}
	c.pending = append(c.pending, p...)
	cut := bytes.LastIndexByte(c.pending, '\n')
	if cut < 0 {
		return
	}
	complete := string(c.pending[:cut+1])
	c.pending = append(c.pending[:0], c.pending[cut+1:]...)
	c.emit(complete)
}

func (c *capture) emit(text string) {
	var b strings.Builder
	for _, line := range strings.SplitAfter(c.redact(text), "\n") {
		if line == "" {
			continue
		}
		b.WriteString(c.prefix)
		b.WriteString(line)
		if !strings.HasSuffix(line, "\n") {
			b.WriteString("\n")
		}
	}
	c.stream.writeString(b.String())
}

// flush writes whatever the step left without a trailing newline, so a partial
// last line is not swallowed.
func (c *capture) flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stream == nil || len(c.pending) == 0 {
		return
	}
	c.emit(string(c.pending))
	c.pending = c.pending[:0]
}

func (c *capture) result() (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return string(c.buf), c.truncated
}
