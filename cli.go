package overlay

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/awked-com/overlay/internal/terminal"
	"github.com/awked-com/overlay/internal/ui"
	"github.com/google/shlex"
	"github.com/spf13/cobra"
)

func Command() *cobra.Command {
	var directory, packagesDir, worktrees, system string
	var o *Overlay
	command := &cobra.Command{
		Use:   "overlay",
		Short: "Edit package patch stacks",
		Long:  "Edit numbered package patches with Quilt in any repository.\n\nThe nearest flake or Git checkout is selected by default; use -C to select another.\nPATCH_WORKTREES selects the worktree root; QUILT selects the Quilt executable.\nPass global flags before PACKAGE when forwarding commands to Quilt.",
	}
	command.PersistentFlags().StringVarP(&directory, "directory", "C", "", "Select project DIRECTORY (default: nearest flake or Git checkout)")
	command.PersistentFlags().StringVar(&packagesDir, "packages-dir", "pkgs", "Package definitions DIRECTORY, relative to the project")
	command.PersistentFlags().StringVar(&worktrees, "worktrees", "", "Worktree DIRECTORY, relative to the project (default: .patch-worktrees/pkgs)")
	command.PersistentFlags().StringVar(&system, "system", "", "Select the Nix source SYSTEM (default: current system)")
	command.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		// Help and completion must work even outside a project.
		for c := cmd; c != nil; c = c.Parent() {
			if c.Name() == "help" || c.Name() == "completion" || c.Name() == "__complete" || c.Name() == "__completeNoDesc" {
				return nil
			}
		}
		root, err := projectRoot(directory)
		if err != nil {
			return err
		}
		o = NewOverlay(root)
		o.PackagesDir = projectPath(root, packagesDir)
		o.System = system
		if worktrees != "" {
			o.Worktrees = projectPath(root, worktrees)
		}
		return o.validateDirectories()
	}
	quilt := &cobra.Command{
		Use:   "quilt PACKAGE COMMAND [ARGS...]",
		Short: "Run a Quilt command",
		Long:  "Run COMMAND with ARGS in PACKAGE's prepared worktree. Arguments after PACKAGE\nare passed to Quilt, including flags such as --help.",
		Example: "  overlay quilt hello push -a\n" +
			"  overlay quilt hello refresh --help",
		Args: ui.Args(cobra.MinimumNArgs(2)),
		RunE: func(_ *cobra.Command, args []string) error {
			pkg := args[0]
			if e := o.ValidatePackage(pkg); e != nil {
				return e
			}
			worktree, patches := filepath.Join(o.Worktrees, pkg), o.patches(pkg)
			if e := ValidateMetadata(worktree); e != nil {
				return e
			}

			if _, e := os.Stat(filepath.Join(worktree, ".quilt-series")); e != nil {
				return fmt.Errorf("run overlay setup %s first", pkg)
			}

			return o.quilt(worktree, patches, args[1:]...).Run()
		},
	}
	quilt.Flags().SetInterspersed(false)

	command.AddCommand(
		&cobra.Command{
			Use:     "list",
			Short:   "List package stacks",
			Long:    "List directories with a default.nix definition under --packages-dir.",
			Example: "  overlay list",
			Args:    ui.Args(cobra.NoArgs),
			RunE: func(_ *cobra.Command, args []string) error {
				names, e := o.Packages()
				if e != nil {
					return e
				}

				for _, n := range names {
					fmt.Println(n)
				}

				return nil
			},
		},
		&cobra.Command{
			Use:     "setup PACKAGE [PATH]",
			Short:   "Prepare a package worktree",
			Long:    "Prepare PACKAGE's worktree and apply its patches. PATH must contain unpacked,\nunpatched source. When omitted, use SOURCE_ROOT/PACKAGE if set; otherwise fetch\nsource from the locked flake. Refresh edits before replacing a worktree.",
			Example: "  overlay setup hello\n  overlay setup hello /tmp/hello",
			Args:    ui.Args(cobra.RangeArgs(1, 2)),
			RunE: func(_ *cobra.Command, args []string) error {
				supplied := ""
				if len(args) == 2 {
					supplied = args[1]
				}
				worktree, e := o.Setup(args[0], supplied)
				if e != nil {
					return e
				}
				fmt.Printf("%s %s\n", terminal.Style(os.Stdout, terminal.Green, "Prepared worktree:"), worktree)
				return nil
			},
		},
		&cobra.Command{
			Use:     "status PACKAGE",
			Short:   "Show a package patch stack",
			Long:    "Show PACKAGE's patches and which are applied.",
			Example: "  overlay status hello",
			Args:    ui.Args(cobra.ExactArgs(1)),
			RunE: func(_ *cobra.Command, args []string) error {
				pkg := args[0]
				if e := o.ValidatePackage(pkg); e != nil {
					return e
				}
				worktree, patches := filepath.Join(o.Worktrees, pkg), o.patches(pkg)
				names, applied, e := Stack(worktree, patches)
				if e != nil {
					return e
				}

				ui.Heading(pkg)
				ui.Detail(fmt.Sprintf("%d patches / %s / %s", len(names),
					terminal.Style(os.Stdout, terminal.Green, fmt.Sprintf("%d applied", len(applied))),
					terminal.Style(os.Stdout, terminal.Yellow, fmt.Sprintf("%d pending", len(names)-len(applied)))))
				if _, e = os.Stat(worktree); e == nil {
					ui.Detail("Worktree: " + terminal.Style(os.Stdout, terminal.Dim, worktree))
				} else {
					fmt.Printf("  %s Create one with %s\n",
						terminal.Style(os.Stdout, terminal.Yellow, "No worktree."),
						terminal.Style(os.Stdout, terminal.Bold, "overlay setup "+pkg))
				}

				if len(names) > 0 {
					fmt.Println()
				}
				for i, p := range names {
					mark := "- pending"
					style := terminal.Yellow
					if i < len(applied) {
						mark = "+ applied"
						style = terminal.Green
					}

					if i == len(applied)-1 {
						mark = "> current"
						style = terminal.Bold + ";" + terminal.Cyan
						p = terminal.Style(os.Stdout, terminal.Bold, p)
					}

					fmt.Printf("  %s %s\n", terminal.Style(os.Stdout, style, fmt.Sprintf("%-10s", mark)), p)
				}

				return nil
			},
		},
		&cobra.Command{
			Use:     "select PACKAGE PATCH",
			Short:   "Select an existing patch",
			Long:    "Select PATCH in PACKAGE's stack. PATCH must be an existing NNNN-description.patch file.",
			Example: "  overlay select hello 0002-fix-build.patch",
			Args:    ui.Args(cobra.ExactArgs(2)),
			RunE: func(_ *cobra.Command, args []string) error {
				pkg, patch := args[0], args[1]
				if e := o.ValidatePackage(pkg); e != nil {
					return e
				}
				if e := ValidatePatch(patch); e != nil {
					return e
				}
				worktree, e := o.Setup(pkg, "")
				if e != nil {
					return e
				}
				patches := o.patches(pkg)
				names, applied, e := Stack(worktree, patches)
				if e != nil {
					return e
				}
				if !slices.Contains(names, patch) {
					return fmt.Errorf("unknown patch: %s", patch)
				}

				if len(applied) > 0 {
					if e = o.quilt(worktree, patches, "pop", "-a").Run(); e != nil {
						return e
					}
				}

				return o.quilt(worktree, patches, "push", patch).Run()
			},
		},
		&cobra.Command{
			Use:     "new PACKAGE PATCH [PATH...]",
			Short:   "Create a patch",
			Long:    "Create PATCH and optionally add PATH entries. PATCH must match\nNNNN-description.patch and sort after the existing patches.",
			Example: "  overlay new hello 0002-fix-build.patch\n  overlay new hello 0002-fix-build.patch src/hello.c",
			Args:    ui.Args(cobra.MinimumNArgs(2)),
			RunE: func(_ *cobra.Command, args []string) error {
				pkg, patch := args[0], args[1]
				if e := o.ValidatePackage(pkg); e != nil {
					return e
				}
				if e := ValidatePatch(patch); e != nil {
					return e
				}
				worktree, e := o.Setup(pkg, "")
				if e != nil {
					return e
				}
				patches := o.patches(pkg)
				names, applied, e := Stack(worktree, patches)
				if e != nil {
					return e
				}
				if len(names) > 0 && patch <= names[len(names)-1] {
					return fmt.Errorf("new patch must sort after %s", names[len(names)-1])
				}

				if e = os.MkdirAll(patches, 0755); e != nil {
					return e
				}

				if len(applied) < len(names) {
					if e = o.quilt(worktree, patches, "push", "-a").Run(); e != nil {
						return e
					}
				}

				if e = o.quilt(worktree, patches, "new", patch).Run(); e != nil {
					return e
				}

				f, e := os.OpenFile(filepath.Join(patches, patch), os.O_CREATE|os.O_WRONLY, 0644)
				if e != nil {
					return e
				}

				if e = f.Close(); e != nil {
					return e
				}

				if len(args) > 2 {
					if e = o.quilt(worktree, patches, append([]string{"add"}, args[2:]...)...).Run(); e != nil {
						return e
					}
				}

				fmt.Printf("%s %s/%s\n", terminal.Style(os.Stdout, terminal.Green, "Created patch:"), pkg, patch)
				return nil
			},
		},
		&cobra.Command{
			Use:     "edit PACKAGE PATH...",
			Short:   "Edit files in a package worktree",
			Long:    "Add PATH entries to the current patch and open them with EDITOR.\nEDITOR defaults to vi and may include shell-style arguments.",
			Example: "  overlay edit hello src/main.c README.md",
			Args:    ui.Args(cobra.MinimumNArgs(2)),
			RunE: func(_ *cobra.Command, args []string) error {
				pkg := args[0]
				worktree, e := o.Setup(pkg, "")
				if e != nil {
					return e
				}
				patches := o.patches(pkg)
				files := o.quilt(worktree, patches, "files")
				files.Stdout = nil
				output, e := files.Output()
				if e != nil {
					return e
				}

				tracked := lines(string(output))
				for _, p := range args[1:] {
					if !slices.Contains(tracked, p) {
						if e = o.quilt(worktree, patches, "add", p).Run(); e != nil {
							return e
						}
					}
				}

				editor := os.Getenv("EDITOR")
				if editor == "" {
					editor = "vi"
				}

				words, e := shlex.Split(editor)
				if e != nil {
					return e
				}
				if len(words) == 0 {
					return errors.New("empty editor command")
				}

				return commandAt(worktree, nil, append(words, args[1:]...)...).Run()
			},
		},
		&cobra.Command{
			Use:     "refresh PACKAGE",
			Short:   "Refresh the current patch",
			Long:    "Refresh PACKAGE's current patch without timestamps or index metadata.",
			Example: "  overlay refresh hello",
			Args:    ui.Args(cobra.ExactArgs(1)),
			RunE: func(_ *cobra.Command, args []string) error {
				pkg := args[0]
				worktree, e := o.Setup(pkg, "")
				if e != nil {
					return e
				}
				patches := o.patches(pkg)
				return o.quilt(worktree, patches, "refresh", "--no-index", "--no-timestamps").Run()
			},
		},
		&cobra.Command{
			Use:     "refresh-stacks [PACKAGE...]",
			Short:   "Refresh package patch stacks",
			Long:    "Refresh all package patches, or only the listed packages. Use SOURCE_ROOT/PACKAGE\nif set; otherwise fetch sources from the locked flake.",
			Example: "  overlay refresh-stacks\n  overlay refresh-stacks hello",
			Args:    ui.Args(cobra.MinimumNArgs(0)),
			RunE: func(_ *cobra.Command, args []string) error {
				return o.RefreshStacks(args)
			},
		},
		quilt,
		&cobra.Command{
			Use:     "shell PACKAGE",
			Short:   "Open a Quilt shell",
			Long:    "Open SHELL in PACKAGE's worktree with the Quilt environment configured.\nSHELL defaults to /bin/sh.",
			Example: "  overlay shell hello",
			Args:    ui.Args(cobra.ExactArgs(1)),
			RunE: func(_ *cobra.Command, args []string) error {
				pkg := args[0]
				worktree, e := o.Setup(pkg, "")
				if e != nil {
					return e
				}
				patches := o.patches(pkg)
				ui.Progress("Starting Quilt shell for " + pkg)
				ui.Progress("Worktree: " + worktree)
				ui.Progress("Patches: " + patches)
				shell := os.Getenv("SHELL")
				if shell == "" {
					shell = "/bin/sh"
				}

				return commandAt(worktree, environment(worktree, patches), shell).Run()
			},
		},
		&cobra.Command{
			Use:     "discard PACKAGE",
			Short:   "Remove a package worktree",
			Long:    "Remove PACKAGE's worktree and its unrefreshed edits. Refresh edits first.\nRepository patch files are preserved.",
			Example: "  overlay discard hello",
			Args:    ui.Args(cobra.ExactArgs(1)),
			RunE: func(_ *cobra.Command, args []string) error {
				pkg := args[0]
				if e := o.ValidatePackage(pkg); e != nil {
					return e
				}
				worktree := filepath.Join(o.Worktrees, pkg)
				if e := ValidateMetadata(worktree); e != nil {
					return e
				}

				if _, e := os.Stat(worktree); e == nil {
					if e := requirePreparedWorktree(worktree); e != nil {
						return e
					}
				} else if !errors.Is(e, os.ErrNotExist) {
					return e
				}

				if e := os.RemoveAll(worktree); e != nil {
					return e
				}

				fmt.Println("Removed " + worktree)
				return nil
			},
		},
	)

	return command
}
