package overlay

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/awked-com/overlay/internal/process"
)

//go:embed source.nix
var sourceExpression string

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

func (o *Overlay) Source(pkg, supplied string) (string, error) {
	if supplied == "" && os.Getenv("SOURCE_ROOT") != "" {
		supplied = filepath.Join(os.Getenv("SOURCE_ROOT"), pkg)
	}

	if supplied == "" {
		fmt.Fprintf(os.Stderr, "Fetching %s source from the locked flake\n", pkg)
		flakeRef, e := sourceFlakeReference(o.Root)
		if e != nil {
			return "", e
		}
		parameters := map[string]string{"flakeRef": flakeRef, "package": pkg}
		if o.System != "" {
			parameters["system"] = o.System
		}
		encoded, e := json.Marshal(parameters)
		if e != nil {
			return "", e
		}
		// Apply explicitly: nix build's --argstr auto-call rejects source
		// strings. JSON keeps project paths out of Nix syntax, while a temporary
		// expression file keeps subprocess diagnostics readable.
		expression, e := os.CreateTemp("", "overlay-source-*.nix")
		if e != nil {
			return "", e
		}
		defer os.Remove(expression.Name())
		_, writeErr := expression.WriteString("(" + sourceExpression + ") (builtins.fromJSON (builtins.getEnv \"OVERLAY_SOURCE_ARGS\"))")
		closeErr := expression.Close()
		if e = errors.Join(writeErr, closeErr); e != nil {
			return "", e
		}
		args := []string{
			"nix", "--extra-experimental-features", "nix-command flakes",
			"build",
			"--impure",
			"--file", expression.Name(),
			"--no-write-lock-file",
			"--no-link",
			"--print-out-paths",
			"--option", "builders", "",
		}
		command := commandAt(o.Root, process.Env(map[string]string{"OVERLAY_SOURCE_ARGS": string(encoded)}), args...)
		command.Stdout = nil
		output, e := command.Output()
		if e != nil {
			return "", e
		}
		supplied = strings.TrimSuffix(string(output), "\n")

		st, e := os.Stat(supplied)
		if e != nil {
			return "", e
		}

		if !st.IsDir() {
			if !st.Mode().IsRegular() {
				return "", fmt.Errorf("Nix did not return a source directory or archive: %s", supplied)
			}

			cache := filepath.Join(o.Worktrees, ".sources", filepath.Base(supplied))
			if _, e = os.Stat(cache); errors.Is(e, os.ErrNotExist) {
				if e = os.MkdirAll(filepath.Dir(cache), 0755); e != nil {
					return "", e
				}

				tmp, e := os.MkdirTemp(filepath.Dir(cache), ".unpack.")
				if e != nil {
					return "", e
				}
				defer os.RemoveAll(tmp)

				unpacked := filepath.Join(tmp, "source")
				if e = os.Mkdir(unpacked, 0755); e != nil {
					return "", e
				}

				if e = process.Run("tar", "-xf", supplied, "-C", unpacked); e != nil {
					return "", e
				}

				if e = os.Rename(unpacked, cache); e != nil {
					return "", e
				}
			}

			supplied = cache
			entries, e := os.ReadDir(cache)
			if e != nil {
				return "", e
			}

			if len(entries) == 1 && entries[0].IsDir() {
				supplied = filepath.Join(cache, entries[0].Name())
			}
		}
	}

	source, e := filepath.EvalSymlinks(supplied)
	if e != nil {
		return "", e
	}

	source, e = filepath.Abs(source)
	if e != nil {
		return "", e
	}

	s, e := os.Stat(source)
	if e != nil {
		return "", e
	}
	if !s.IsDir() {
		return "", fmt.Errorf("source directory does not exist: %s", source)
	}

	for _, n := range []string{".pc", ".quilt-series", ".upstream-source"} {
		if _, e = os.Lstat(filepath.Join(source, n)); !errors.Is(e, os.ErrNotExist) {
			return "", fmt.Errorf("source contains reserved patch metadata: %s", n)
		}
	}

	return source, nil
}

func copySource(source, destination string) error {
	return filepath.WalkDir(source, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}

		rel, e := filepath.Rel(source, path)
		if e != nil {
			return e
		}

		target := filepath.Join(destination, rel)
		s, e := d.Info()
		if e != nil {
			return e
		}

		switch {
		case d.IsDir():
			return os.MkdirAll(target, s.Mode().Perm()|0200)
		case d.Type()&os.ModeSymlink != 0:
			link, e := os.Readlink(path)
			if e != nil {
				return e
			}

			return os.Symlink(link, target)
		case s.Mode().IsRegular():
			return process.CopyFile(path, target, s.Mode().Perm()|0200)
		default:
			return fmt.Errorf("unsupported source entry: %s", path)
		}
	})
}
