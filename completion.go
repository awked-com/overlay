package overlay

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

func completeNames(names, selected []string, prefix string) ([]string, cobra.ShellCompDirective) {
	var matches []string
	for _, name := range names {
		if strings.HasPrefix(name, prefix) && !slices.Contains(selected, name) && !strings.ContainsAny(name, "\x00\t\r\n") {
			matches = append(matches, name)
		}
	}
	return matches, cobra.ShellCompDirectiveNoFileComp
}

func (o *Overlay) completeCommand(cmd *cobra.Command, args []string, prefix string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 || cmd.Name() == "refresh-stacks" {
		names, err := o.Packages()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError | cobra.ShellCompDirectiveNoFileComp
		}
		names = slices.DeleteFunc(names, func(name string) bool { return !packagePattern.MatchString(name) })
		return completeNames(names, args, prefix)
	}
	if err := o.ValidatePackage(args[0]); err != nil {
		return nil, cobra.ShellCompDirectiveError | cobra.ShellCompDirectiveNoFileComp
	}
	switch cmd.Name() {
	case "setup":
		if len(args) == 1 {
			return nil, cobra.ShellCompDirectiveFilterDirs
		}
	case "select":
		if len(args) == 1 {
			return o.completePatches(args[0], prefix)
		}
	case "edit":
		return completePaths(filepath.Join(o.Worktrees, args[0]), args[1:], prefix, false)
	case "new":
		if len(args) >= 2 {
			return completePaths(filepath.Join(o.Worktrees, args[0]), args[2:], prefix, false)
		}
	case "quilt":
		if len(args) == 2 && (args[1] == "push" || args[1] == "pop") {
			return o.completePatches(args[0], prefix)
		}
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func (o *Overlay) completePatches(pkg, prefix string) ([]string, cobra.ShellCompDirective) {
	names, err := PatchNames(o.patches(pkg))
	if err != nil {
		return nil, cobra.ShellCompDirectiveError | cobra.ShellCompDirectiveNoFileComp
	}
	return completeNames(names, nil, prefix)
}

// Paths passed to Quilt are relative to the worktree, not the caller's directory.
func completePaths(root string, selected []string, prefix string, directoriesOnly bool) ([]string, cobra.ShellCompDirective) {
	dir, partial := filepath.Split(prefix)
	entries, err := os.ReadDir(projectPath(root, dir))
	if os.IsNotExist(err) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	if err != nil {
		return nil, cobra.ShellCompDirectiveError | cobra.ShellCompDirectiveNoFileComp
	}
	var names []string
	allDirs := true
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, partial) || strings.ContainsAny(name, "\x00\t\r\n") {
			continue
		}
		if !directoriesOnly && (name == ".pc" || name == ".quilt-series" || name == ".upstream-source") {
			continue
		}
		info, err := os.Stat(projectPath(root, dir+name))
		if err != nil || (directoriesOnly && !info.IsDir()) {
			continue
		}
		if info.IsDir() {
			name += string(filepath.Separator)
		}
		if !slices.Contains(selected, dir+name) {
			names = append(names, dir+name)
			allDirs = allDirs && info.IsDir()
		}
	}
	directive := cobra.ShellCompDirectiveNoFileComp
	if len(names) > 0 && allDirs {
		directive |= cobra.ShellCompDirectiveNoSpace
	}
	return names, directive
}
