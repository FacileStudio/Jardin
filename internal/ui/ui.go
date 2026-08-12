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

// Dim returns its input formatted as faint text.
func Dim(s string) string {
	return faint.Sprint(s)
}
