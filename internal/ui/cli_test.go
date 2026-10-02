package ui

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/awked-com/overlay/internal/process"
	"github.com/spf13/cobra"
)

func TestInvalidCommandUsageStatus(t *testing.T) {
	for _, args := range [][]string{{"stats"}, {"help", "stats"}} {
		root := &cobra.Command{Use: "overlay"}
		root.AddCommand(&cobra.Command{Use: "status", Run: func(*cobra.Command, []string) { t.Fatal("invalid command ran an operation") }})
		var output bytes.Buffer
		err := ExecuteTo(root, args, &output, io.Discard)
		if output.Len() != 0 || process.ExitCode(err) != 2 {
			t.Fatalf("invalid command: output %q, error %v", &output, err)
		}
	}
}

type failedOutput struct{}

func (failedOutput) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestHelpPropagatesOutputFailures(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"help", "status"}} {
		root := &cobra.Command{Use: "fixture"}
		root.AddCommand(&cobra.Command{Use: "status", Run: func(*cobra.Command, []string) { t.Fatal("help ran the command") }})
		if err := ExecuteTo(root, args, failedOutput{}, io.Discard); !errors.Is(err, io.ErrClosedPipe) || process.ExitCode(err) != 1 {
			t.Fatalf("help lost output error: %v", err)
		}
	}
}
