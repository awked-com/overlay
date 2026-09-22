// Package terminal formats human-readable output for its destination.
package terminal

import (
	"io"
	"os"

	"golang.org/x/term"
)

// Styles contain ANSI SGR parameters; join them with semicolons to combine them.
const (
	Bold    = "1"
	Dim     = "2"
	Red     = "31"
	Green   = "32"
	Yellow  = "33"
	Blue    = "34"
	Magenta = "35"
	Cyan    = "36"
)

// IsTerminal checks the destination, so redirected output remains plain even
// when another stream is connected to a terminal.
func IsTerminal(w io.Writer) bool {
	f, ok := w.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(int(f.Fd()))
}

// ColorEnabled respects terminal capabilities and the operator's color policy.
// Forced color never adds escape sequences to files, pipes, or captured output.
func ColorEnabled(w io.Writer) bool {
	return colorEnabled(IsTerminal(w))
}

func colorEnabled(tty bool) bool {
	return tty && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb" && os.Getenv("CLICOLOR") != "0"
}

// Style applies SGR parameters only when the destination supports color.
func Style(w io.Writer, style, text string) string {
	if text == "" || style == "" || !ColorEnabled(w) {
		return text
	}
	return "\x1b[" + style + "m" + text + "\x1b[0m"
}
