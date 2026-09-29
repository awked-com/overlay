package ui

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/awked-com/overlay/internal/process"
	"github.com/spf13/cobra"
)

func TestHelpRoutesDoNotRunCommands(t *testing.T) {
	for _, args := range [][]string{{"help", "group", "status"}, {"group", "help", "status"}, {"group", "status", "--help"}, {"help", "help"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			root := &cobra.Command{Use: "infra"}
			group := &cobra.Command{Use: "group"}
			group.AddCommand(&cobra.Command{Use: "status TARGET", Args: Args(cobra.ExactArgs(1)), Run: func(*cobra.Command, []string) { t.Fatal("help ran an operation") }})
			root.AddCommand(group)
			out, err := executeOutput(t, root, args)
			if err != nil || out == "" {
				t.Fatalf("help failed: %q (%v)", out, err)
			}
			if args[len(args)-1] != "help" && !strings.Contains(out, "infra group status TARGET") {
				t.Fatalf("wrong help target: %s", out)
			}
		})
	}
}

func TestCommandSuggestionsAndUsageStatus(t *testing.T) {
	for _, args := range [][]string{{"stats"}, {"help", "stats"}} {
		root := &cobra.Command{Use: "infra"}
		root.AddCommand(&cobra.Command{Use: "status", Run: func(*cobra.Command, []string) { t.Fatal("invalid command ran an operation") }})
		out, err := executeOutput(t, root, args)
		if out != "" || process.ExitCode(err) != 2 {
			t.Fatalf("invalid command: output %q, error %v", out, err)
		}
		if !strings.Contains(err.Error(), "status") || !strings.Contains(err.Error(), "infra --help") {
			t.Fatalf("missing command suggestion or usage hint: %v", err)
		}
	}
}

func executeOutput(t *testing.T, root *cobra.Command, args []string) (string, error) {
	t.Helper()
	output, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	previous := os.Stdout
	os.Stdout = output
	defer func() { os.Stdout = previous }()
	runErr := Execute(root, args)
	data, err := os.ReadFile(output.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(data), runErr
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
