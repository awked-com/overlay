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
	root.SilenceErrors, root.SilenceUsage = true, true
	var helpErr error
	root.SetHelpFunc(func(cmd *cobra.Command, _ []string) { helpErr = writeHelp(cmd, true) })
	root.SetUsageFunc(func(cmd *cobra.Command) error { return writeHelp(cmd, false) })
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
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
	parent.InitDefaultHelpCmd()
	for _, child := range parent.Commands() {
		if child.Name() != "help" {
			addHelp(child)
			continue
		}
		child.Use = "help [COMMAND [SUBCOMMAND...]]"
		child.Short, child.Long = "Show command help", ""
		child.Run = nil
		child.RunE = func(_ *cobra.Command, args []string) error {
			target, rest, err := parent.Find(args)
			if err != nil {
				return UsageError(target, err)
			}
			if len(rest) != 0 {
				return UsageError(target, fmt.Errorf("unknown command %q", strings.Join(rest, " ")))
			}
			return target.Help()
		}
		child.ValidArgsFunction = func(_ *cobra.Command, args []string, prefix string) ([]string, cobra.ShellCompDirective) {
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
		}
		child.InitDefaultHelpFlag()
		child.Flags().Lookup("help").Usage = "Show command help"
	}
}

func writeHelp(cmd *cobra.Command, description bool) error {
	out := cmd.OutOrStdout()
	var text strings.Builder
	action := func(s string) string { return terminal.Style(out, terminal.Cyan, s) }
	section := func(title string) {
		fmt.Fprintln(&text, "\n"+terminal.Style(out, terminal.Bold+";"+terminal.Cyan, title+":"))
	}
	if description {
		fmt.Fprintln(&text, terminal.Style(out, terminal.Bold+";"+terminal.Cyan, cmd.CommandPath()))
		description := cmd.Long
		if description == "" {
			description = cmd.Short
		}
		fmt.Fprintln(&text, strings.TrimSpace(description))
	}
	section("Usage")
	if cmd.Runnable() || !cmd.HasAvailableSubCommands() {
		fmt.Fprintln(&text, "  "+action(cmd.UseLine()))
	}
	if cmd.HasAvailableSubCommands() {
		fmt.Fprintln(&text, "  "+action(cmd.CommandPath()+" [command]"))
		section("Commands")
		for _, child := range cmd.Commands() {
			if child.IsAvailableCommand() || child.Name() == "help" {
				fmt.Fprintf(&text, "  %s  %s\n", action(fmt.Sprintf("%-18s", child.Name())), child.Short)
			}
		}
	}
	for _, flags := range []struct{ title, usage string }{
		{"Flags", cmd.LocalFlags().FlagUsages()},
		{"Global flags", cmd.InheritedFlags().FlagUsages()},
	} {
		if flags.usage != "" {
			section(flags.title)
			fmt.Fprint(&text, flags.usage)
		}
	}
	if cmd.Example != "" {
		section("Examples")
		fmt.Fprintln(&text, action(strings.TrimRight(cmd.Example, "\n")))
	}
	if cmd.HasAvailableSubCommands() {
		fmt.Fprintf(&text, "\nRun `%s help COMMAND` for command details.\n", cmd.CommandPath())
	}
	// Build help before writing so a failed output cannot be hidden by Cobra's
	// help callback, which has no error return.
	n, err := io.WriteString(out, text.String())
	if err == nil && n != text.Len() {
		err = io.ErrShortWrite
	}
	return err
}
