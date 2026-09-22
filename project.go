package overlay

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// projectRoot finds the nearest project, never crossing a Git boundary to use
// an unrelated parent flake. An explicit directory can also hold source-only stacks.
func projectRoot(directory string) (string, error) {
	if directory != "" {
		root, err := filepath.Abs(directory)
		if err != nil {
			return "", err
		}
		info, err := os.Stat(root)
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			return "", errors.New("project directory is not a directory")
		}
		return filepath.EvalSymlinks(root)
	}
	directory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		for _, name := range []string{"flake.nix", ".git"} {
			if _, err := os.Stat(filepath.Join(directory, name)); err == nil {
				return filepath.EvalSymlinks(directory)
			} else if !errors.Is(err, os.ErrNotExist) {
				return "", err
			}
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", errors.New("no project found; run inside a flake or Git checkout, or select one with -C DIRECTORY")
		}
		directory = parent
	}
}

func projectPath(root, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

func sourceFlakeReference(root string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	// Nix path inputs reject symlink components, including macOS's /tmp alias.
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	for dir := root; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			reference := url.URL{Scheme: "git+file", Path: dir}
			relative, err := filepath.Rel(dir, root)
			if err != nil {
				return "", err
			}
			if relative != "." {
				query := url.Values{"dir": {filepath.ToSlash(relative)}}
				// Flake URL queries use percent escapes, not form-style '+' spaces.
				reference.RawQuery = strings.ReplaceAll(query.Encode(), "+", "%20")
			}
			return reference.String(), nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		if filepath.Dir(dir) == dir {
			return (&url.URL{Scheme: "path", Path: root}).String(), nil
		}
	}
}

// resolveDirectory resolves existing ancestors even when the final directory
// has not been created yet. This prevents aliases from hiding overlapping roots.
func resolveDirectory(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	var missing []string
	for {
		if _, err := os.Lstat(path); err == nil {
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil {
				return "", err
			}
			return filepath.Join(append([]string{resolved}, missing...)...), nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		missing = append([]string{filepath.Base(path)}, missing...)
		parent := filepath.Dir(path)
		if parent == path {
			return "", fmt.Errorf("no existing ancestor for directory: %s", path)
		}
		path = parent
	}
}

func containsDirectory(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func (o *Overlay) validateDirectories() error {
	packages, err := resolveDirectory(o.PackagesDir)
	if err != nil {
		return err
	}
	worktrees, err := resolveDirectory(o.Worktrees)
	if err != nil {
		return err
	}
	if containsDirectory(packages, worktrees) || containsDirectory(worktrees, packages) {
		return fmt.Errorf("package and worktree directories must not overlap: %s and %s", o.PackagesDir, o.Worktrees)
	}
	return nil
}
