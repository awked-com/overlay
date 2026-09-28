package ui

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/awked-com/overlay/internal/process"
	"github.com/spf13/cobra"
)

func TestHelpUsesCommandGroups(t *testing.T) {
	root := &cobra.Command{Use: "infra", Short: "Operate fixtures"}
	root.AddGroup(&cobra.Group{ID: "operations", Title: "Operate fixtures"}, &cobra.Group{ID: "empty", Title: "Empty group"})
	root.SetHelpCommandGroupID("operations")
	root.AddCommand(
		&cobra.Command{Use: "status", Short: "Inspect fixture status", GroupID: "operations", Run: func(*cobra.Command, []string) {}},
		&cobra.Command{Use: "other", Short: "Inspect another fixture", Run: func(*cobra.Command, []string) {}},
		&cobra.Command{Use: "private", Short: "Hidden command", GroupID: "operations", Hidden: true, Run: func(*cobra.Command, []string) {}},
	)
	addHelp(root)
	var out bytes.Buffer
	root.SetOut(&out)
	writeHelp(root, true)
	help := out.String()
	for _, want := range []string{"infra [command]", "Operate fixtures:", "status", "Inspect fixture status", "other", "Inspect another fixture", "Show command help", "--help"} {
		if !strings.Contains(help, want) {
			t.Errorf("help missing %q:\n%s", want, help)
		}
	}
	for _, unwanted := range []string{"private", "Hidden command", "Empty group", "\x1b["} {
		if strings.Contains(help, unwanted) {
			t.Errorf("help contains %q:\n%s", unwanted, help)
		}
	}
	groupedHelp, _, err := root.Find([]string{"help"})
	if err != nil || groupedHelp.GroupID != "operations" {
		t.Fatalf("help lost its command group: %q (%v)", groupedHelp.GroupID, err)
	}
}

func TestHelpPreservesFlagMetadataAndExamples(t *testing.T) {
	root := &cobra.Command{Use: "infra"}
	root.PersistentFlags().String("directory", "/fixtures", "Read fixtures from this directory")
	status := &cobra.Command{Use: "status TARGET", Aliases: []string{"inspect"}, Short: "Inspect a fixture", Example: "  infra status fixture --jobs 2", Run: func(*cobra.Command, []string) {}}
	status.Flags().IntP("jobs", "j", 2, "Run this many jobs")
	status.Flags().Bool("internal", false, "Internal switch")
	if err := status.Flags().MarkHidden("internal"); err != nil {
		t.Fatal(err)
	}
	root.AddCommand(status)
	addHelp(root)
	var out bytes.Buffer
	root.SetOut(&out)
	writeHelp(status, true)
	help := out.String()
	for _, want := range []string{"infra status TARGET [flags]", "inspect", "--jobs int", "-j,", "default 2", "--directory string", "/fixtures", status.Example} {
		if !strings.Contains(help, want) {
			t.Errorf("help missing %q:\n%s", want, help)
		}
	}
	for _, unwanted := range []string{"--internal", "Internal switch", "\x1b["} {
		if strings.Contains(help, unwanted) {
			t.Errorf("help contains %q:\n%s", unwanted, help)
		}
	}
}

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

func TestUsageErrorPreservesCause(t *testing.T) {
	cause := errors.New("fixture failure")
	err := UsageError(&cobra.Command{Use: "infra"}, cause)
	if !errors.Is(err, cause) || process.ExitCode(err) != 2 {
		t.Fatalf("usage error lost cause or status: %v", err)
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
