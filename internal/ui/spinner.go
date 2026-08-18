package ui

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/fatih/color"
	"golang.org/x/term"
)

const (
	spinnerDelay    = 150 * time.Millisecond
	spinnerInterval = 100 * time.Millisecond
	spinnerFrames   = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
	clearLine       = "\r\x1b[K"
)

// Spinner is a one-line progress indicator for a wait of unknown length. It
// animates only when it is drawing on a terminal with colour enabled;
// redirected or piped output gets nothing at all. That silence is the point:
// this CLI's busiest caller is an agent capturing stdout, and animation frames
// in a captured transcript are noise nobody can read back.
//
// It hides nothing and installs no signal handler, so an interrupted run leaves
// no terminal state to repair. A spinner that hides the cursor has to restore
// it, and restoring it on SIGINT means a library quietly competing with the
// program's own handler.
type Spinner struct {
	out   io.Writer
	label string
	stop  chan struct{}
	done  chan struct{}
	once  sync.Once
}

// NewSpinner returns a spinner drawing on w, inert unless w is a terminal that
// can render it. A --no-color or NO_COLOR run, and a TERM=dumb terminal, are
// all requests for plain output, which an animated line is not.
func NewSpinner(w io.Writer, label string) *Spinner {
	return newSpinner(w, label, animates(w))
}

// Start begins the animation. Nothing is drawn for the first delay, so work
// that finishes quickly — the local fallback answers in about a tenth of a
// second — never flashes a frame the reader cannot follow.
func (s *Spinner) Start() {
	if s.out == nil {
		return
	}
	s.stop, s.done = make(chan struct{}), make(chan struct{})
	go s.run()
}

// Stop ends the animation and clears the line it was drawn on, so the caller's
// next line starts on clean ground. It waits for the drawing goroutine to
// finish, so the final erase cannot race a frame, and repeated calls from any
// goroutine are harmless.
func (s *Spinner) Stop() {
	if s.out == nil || s.stop == nil {
		return
	}
	s.once.Do(func() {
		close(s.stop)
		<-s.done
	})
}

func newSpinner(w io.Writer, label string, animate bool) *Spinner {
	if !animate {
		return &Spinner{}
	}
	return &Spinner{out: w, label: label}
}

// run draws until it is stopped. The erase is a carriage return followed by
// CSI K rather than CSI 2K: erase-in-line never moves the cursor, so clearing
// the whole line without returning first would leave the next frame printing
// from wherever the last one ended.
func (s *Spinner) run() {
	defer close(s.done)
	select {
	case <-s.stop:
		return
	case <-time.After(spinnerDelay):
	}

	ticker := time.NewTicker(spinnerInterval)
	defer ticker.Stop()
	defer fmt.Fprint(s.out, clearLine)

	frames := []rune(spinnerFrames)
	for i := 0; ; i++ {
		fmt.Fprintf(s.out, "%s%s %s", clearLine, cyan.Sprint(string(frames[i%len(frames)])), s.label)
		select {
		case <-s.stop:
			return
		case <-ticker.C:
		}
	}
}

func animates(w io.Writer) bool {
	if color.NoColor || os.Getenv("TERM") == "dumb" {
		return false
	}
	file, ok := w.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}
