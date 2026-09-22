package main

import (
	"os"

	"github.com/awked-com/overlay"
	"github.com/awked-com/overlay/internal/process"
	"github.com/awked-com/overlay/internal/ui"
)

func main() { process.Exit(ui.Execute(overlay.Command(), os.Args[1:])) }
