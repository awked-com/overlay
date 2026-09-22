package ui

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/awked-com/overlay/internal/process"
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
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	root.SetFlagErrorFunc(UsageError)
	root.SetHelpCommand(&cobra.Command{
		Use: "help [COMMAND]", Short: "Show command help",
		RunE: func(_ *cobra.Command, args []string) error {
			target, rest, err := root.Find(args)
			if err != nil {
				return UsageError(root, err)
			}
			if len(rest) != 0 {
				return UsageError(root, fmt.Errorf("unknown command %q", strings.Join(rest, " ")))
			}
			return target.Help()
		},
	})
	root.SetArgs(args)
	cmd, err := root.ExecuteC()
	var status *process.StatusError
	if err != nil && !errors.As(err, &status) && (cmd == nil || !cmd.Runnable()) {
		return UsageError(root, err)
	}
	return err
}
