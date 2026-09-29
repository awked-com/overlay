package ui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/awked-com/overlay/internal/process"
	"github.com/awked-com/overlay/internal/terminal"
	"github.com/spf13/cobra"
)

func UsageError(cmd *cobra.Command, err error) error {
	return &process.StatusError{Code: 2, Err: fmt.Errorf("%w\nRun `%s --help` for usage", err, cmd.CommandPath())}
}

func Args(validate cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := validate(cmd, args); err != nil {
			return UsageError(cmd, err)
		}
		return nil
	}
}

func Execute(root *cobra.Command, args []string) error {
	return ExecuteTo(root, args, os.Stdout, os.Stderr)
}

func ExecuteTo(root *cobra.Command, args []string, out, stderr io.Writer) error {
	root.SilenceErrors, root.SilenceUsage = true, true
	var helpErr error
	root.SetHelpFunc(func(cmd *cobra.Command, _ []string) { helpErr = writeHelp(cmd, true) })
	root.SetUsageFunc(func(cmd *cobra.Command) error { return writeHelp(cmd, false) })
	root.SetOut(out)
	root.SetErr(stderr)
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error { return UsageError(cmd, err) })
	addHelp(root)
	root.SetArgs(args)
	cmd, err := root.ExecuteC()
	if helpErr != nil {
		return helpErr
	}
	var status *process.StatusError
	if err != nil && !errors.As(err, &status) && (cmd == nil || !cmd.Runnable()) {
		return UsageError(cmd, err)
	}
	return err
}

func addHelp(parent *cobra.Command) {
	parent.InitDefaultHelpFlag()
	parent.Flags().Lookup("help").Usage = "Show command help"
	if !parent.HasSubCommands() {
		return
	}
	parent.InitDefaultHelpCmd()
	var previousHelp *cobra.Command
	for _, child := range parent.Commands() {
		if child.Name() == "help" {
			previousHelp = child
			continue
		}
		addHelp(child)
	}
	help := &cobra.Command{
		Use:   "help [COMMAND [SUBCOMMAND...]]",
		Short: "Show command help",
		ValidArgsFunction: func(_ *cobra.Command, args []string, prefix string) ([]string, cobra.ShellCompDirective) {
			target, rest, err := parent.Find(args)
			if err != nil || len(rest) != 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			var matches []string
			for _, child := range target.Commands() {
				if !child.Hidden && child.IsAvailableCommand() && strings.HasPrefix(child.Name(), prefix) {
					matches = append(matches, child.Name()+"\t"+child.Short)
				}
			}
			return matches, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			target, rest, err := parent.Find(args)
			if err != nil {
				return UsageError(target, err)
			}
			if len(rest) != 0 {
				return UsageError(target, fmt.Errorf("unknown command %q", strings.Join(rest, " ")))
			}
			return target.Help()
		},
	}
	if previousHelp != nil {
		help.GroupID = previousHelp.GroupID
		parent.RemoveCommand(previousHelp)
	}
	help.InitDefaultHelpFlag()
	help.Flags().Lookup("help").Usage = "Show command help"
	parent.SetHelpCommand(help)
	parent.AddCommand(help)
}

func writeHelp(cmd *cobra.Command, description bool) error {
	w := &helpWriter{Writer: cmd.OutOrStdout()}
	if description {
		fmt.Fprintln(w, terminal.Style(w, terminal.Bold+";"+terminal.Cyan, cmd.CommandPath()))
		text := cmd.Long
		if text == "" {
			text = cmd.Short
		}
		if text != "" {
			fmt.Fprintln(w, strings.TrimSpace(text))
		}
	}
	helpSection(w, "Usage")
	if cmd.Runnable() || !cmd.HasAvailableSubCommands() {
		fmt.Fprintln(w, "  "+terminal.Style(w, terminal.Cyan, cmd.UseLine()))
	}
	if cmd.HasAvailableSubCommands() {
		fmt.Fprintln(w, "  "+terminal.Style(w, terminal.Cyan, cmd.CommandPath()+" [command]"))
	}
	if len(cmd.Aliases) > 0 {
		helpSection(w, "Aliases")
		fmt.Fprintln(w, "  "+strings.Join(cmd.Aliases, ", "))
	}
	writeHelpCommands(w, cmd)
	if cmd.HasAvailableLocalFlags() {
		helpSection(w, "Flags")
		writeHelpFlags(w, cmd.LocalFlags().FlagUsages())
	}
	if cmd.HasAvailableInheritedFlags() {
		helpSection(w, "Global flags")
		writeHelpFlags(w, cmd.InheritedFlags().FlagUsages())
	}
	if cmd.Example != "" {
		helpSection(w, "Examples")
		fmt.Fprintln(w, terminal.Style(w, terminal.Cyan, strings.TrimRight(cmd.Example, "\n")))
	}
	if cmd.HasAvailableSubCommands() {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Run `%s help COMMAND` for command details.\n", cmd.CommandPath())
	}
	return w.err
}

// Cobra's help callback cannot return errors; retain the first failed write.
type helpWriter struct {
	io.Writer
	err error
}

func (w *helpWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.Writer.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	w.err = err
	return n, err
}

func (w *helpWriter) Fd() uintptr {
	if f, ok := w.Writer.(interface{ Fd() uintptr }); ok {
		return f.Fd()
	}
	return ^uintptr(0)
}

func helpSection(w io.Writer, title string) {
	fmt.Fprintln(w, "\n"+terminal.Style(w, terminal.Bold, strings.TrimSuffix(title, ":")+":"))
}

func writeHelpCommands(w io.Writer, cmd *cobra.Command) {
	var commands []*cobra.Command
	width := 0
	for _, child := range cmd.Commands() {
		if child.Hidden || (!child.IsAvailableCommand() && child.Name() != "help") {
			continue
		}
		commands = append(commands, child)
		width = max(width, len(child.Name()))
	}
	writeGroup := func(title, id string) {
		shown := false
		for _, child := range commands {
			if child.GroupID != id {
				continue
			}
			if !shown {
				helpSection(w, title)
				shown = true
			}
			name := terminal.Style(w, terminal.Cyan, child.Name())
			fmt.Fprintf(w, "  %s%s  %s\n", name, strings.Repeat(" ", width-len(child.Name())), child.Short)
		}
	}
	for _, group := range cmd.Groups() {
		writeGroup(group.Title, group.ID)
	}
	title := "Commands"
	if len(cmd.Groups()) > 0 {
		title = "Other commands"
	}
	writeGroup(title, "")
}

func writeHelpFlags(w io.Writer, usage string) {
	for _, line := range strings.Split(strings.TrimRight(usage, "\n"), "\n") {
		trimmed := strings.TrimLeft(line, " ")
		if strings.HasPrefix(trimmed, "-") {
			if end := strings.Index(trimmed, "  "); end >= 0 {
				padding := line[:len(line)-len(trimmed)]
				line = padding + terminal.Style(w, terminal.Cyan, trimmed[:end]) + trimmed[end:]
			}
		}
		fmt.Fprintln(w, line)
	}
}
