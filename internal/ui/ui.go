package ui

import (
	"fmt"
	"os"

	"github.com/fatih/color"
)

var (
	cyan   = color.New(color.FgCyan)
	green  = color.New(color.FgGreen)
	yellow = color.New(color.FgYellow)
	red    = color.New(color.FgRed)
	faint  = color.New(color.Faint)
)

// DisableColor implements --no-color. NO_COLOR and TTY detection are already
// handled by fatih/color at init time.
func DisableColor() {
	color.NoColor = true
}

// Step prints a cyan progress step to stdout.
func Step(format string, a ...any) {
	fmt.Fprintf(os.Stdout, "%s %s\n", cyan.Sprint("▸"), fmt.Sprintf(format, a...))
}

// Success prints a green confirmation to stdout.
func Success(format string, a ...any) {
	fmt.Fprintf(os.Stdout, "%s %s\n", green.Sprint("✓"), fmt.Sprintf(format, a...))
}

// Warn prints a yellow warning to stderr.
func Warn(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "%s %s\n", yellow.Sprint("!"), fmt.Sprintf(format, a...))
}

// Error prints a red error to stderr.
func Error(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "%s %s\n", red.Sprint("✗"), fmt.Sprintf(format, a...))
}

// Hint prints an indented, faint auxiliary line to stdout.
func Hint(format string, a ...any) {
	fmt.Fprintf(os.Stdout, "  %s\n", faint.Sprint(fmt.Sprintf(format, a...)))
}

// ErrorHint prints the same indented, faint line to stderr, for the detail that
// belongs under an Error. Reaching for Hint there splits one message across two
// streams, so a `2>log` keeps the headline and loses the explanation.
func ErrorHint(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "  %s\n", faint.Sprint(fmt.Sprintf(format, a...)))
}

// Outcome is how one line of a report turned out.
type Outcome int

// The outcomes a reported item can have.
const (
	OK Outcome = iota
	Warned
	Failed
)

// Result prints one line of a multi-item report to stdout, with the same glyph
// and colour Success, Warn and Error use.
//
// It exists because those three split by stream — warnings and errors go to
// stderr — which is right for a message and wrong for a report. A list of
// checked items read as a whole must not arrive down two pipes in an order
// nobody chose, so callers reached for color.RedString and rebuilt the glyphs
// by hand instead. This is that vocabulary, on one stream.
func Result(outcome Outcome, format string, a ...any) {
	glyph := green.Sprint("✓")
	switch outcome {
	case Warned:
		glyph = yellow.Sprint("!")
	case Failed:
		glyph = red.Sprint("✗")
	}
	fmt.Fprintf(os.Stdout, "  %s %s\n", glyph, fmt.Sprintf(format, a...))
}

// Detail prints an indented line belonging to the Result above it.
func Detail(outcome Outcome, format string, a ...any) {
	label := faint.Sprint("note")
	switch outcome {
	case Warned:
		label = yellow.Sprint("warn")
	case Failed:
		label = red.Sprint("error")
	}
	fmt.Fprintf(os.Stdout, "    %s %s\n", label, fmt.Sprintf(format, a...))
}

// Dim returns its input formatted as faint text.
func Dim(s string) string {
	return faint.Sprint(s)
}
