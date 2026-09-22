// Package terminal formats human-readable output for its destination.
package terminal

import (
	"io"
	"os"

	"golang.org/x/term"
)

// Styles contain ANSI SGR parameters; join them with semicolons to combine them.
const (
	Bold   = "1"
	Dim    = "2"
	Red    = "31"
	Green  = "32"
	Yellow = "33"
	Cyan   = "36"
)

// Style applies SGR parameters only when the destination supports color.
func Style(w io.Writer, style, text string) string {
	f, ok := w.(interface{ Fd() uintptr })
	if text == "" || style == "" || !ok || !term.IsTerminal(int(f.Fd())) ||
		os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" || os.Getenv("CLICOLOR") == "0" {
		return text
	}
	return "\x1b[" + style + "m" + text + "\x1b[0m"
}
