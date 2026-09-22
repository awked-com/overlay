package ui

import (
	"fmt"
	"os"

	"github.com/awked-com/overlay/internal/terminal"
)

func Heading(s string)  { fmt.Println(terminal.Style(os.Stdout, terminal.Bold+";"+terminal.Cyan, s)) }
func Detail(s string)   { fmt.Println("  " + s) }
func Progress(s string) { fmt.Fprintln(os.Stderr, terminal.Style(os.Stderr, terminal.Cyan, s)) }
