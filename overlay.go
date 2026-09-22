package overlay

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/awked-com/overlay/internal/process"
	"github.com/awked-com/overlay/internal/ui"
)

type Overlay struct {
	Root, PackagesDir, Worktrees, Quilt, System string
}

var packagePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._+\-]*$`)
var patchPattern = regexp.MustCompile(`^[0-9]{4}-[a-zA-Z0-9][a-zA-Z0-9._+\-]*\.patch$`)

func NewOverlay(root string) *Overlay {
	worktrees := os.Getenv("PATCH_WORKTREES")
	if worktrees == "" {
		worktrees = filepath.Join(root, ".patch-worktrees/pkgs")
	}

	worktrees, _ = filepath.Abs(worktrees)
	quilt := os.Getenv("QUILT")
	if quilt == "" {
		quilt = "quilt"
	}

	return &Overlay{Root: root, PackagesDir: filepath.Join(root, "pkgs"), Worktrees: worktrees, Quilt: quilt}
}

func (o *Overlay) Packages() ([]string, error) {
	paths, e := filepath.Glob(filepath.Join(o.PackagesDir, "*/default.nix"))
	if e != nil {
		return nil, e
	}

	names := []string{}
	for _, p := range paths {
		if s, e := os.Stat(p); e == nil && s.Mode().IsRegular() {
			names = append(names, filepath.Base(filepath.Dir(p)))
		}
	}

	return names, nil
}

func (o *Overlay) ValidatePackage(p string) error {
	if e := o.validateDirectories(); e != nil {
		return e
	}
	names, e := o.Packages()
	if e != nil {
		return e
	}
	if !packagePattern.MatchString(p) || !slices.Contains(names, p) {
		return fmt.Errorf("unknown package: %s", p)
	}

	return nil
}

func ValidatePatch(p string) error {
	if !patchPattern.MatchString(p) {
		return fmt.Errorf("PATCH must be a numbered filename such as 0001-description.patch: %s", p)
	}

	return nil
}

func (o *Overlay) patches(p string) string { return filepath.Join(o.PackagesDir, p, "patches") }

func PatchNames(dir string) ([]string, error) {
	entries, e := os.ReadDir(dir)
	if errors.Is(e, os.ErrNotExist) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}

	out := []string{}
	for _, p := range entries {
		if p.Type().IsRegular() && strings.HasSuffix(p.Name(), ".patch") {
			if e = ValidatePatch(p.Name()); e != nil {
				return nil, e
			}

			out = append(out, p.Name())
		}
	}

	return out, nil
}

func ValidateMetadata(worktree string) error {
	for _, p := range []string{worktree, filepath.Join(worktree, ".pc")} {
		s, e := os.Lstat(p)
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			return e
		}
		if !s.IsDir() {
			return fmt.Errorf("worktree and Quilt state must be real directories: %s", p)
		}
	}

	for _, n := range []string{
		".upstream-source",
		".quilt-series",
		".pc/applied-patches",
		".pc/.quilt_series",
		".pc/.quilt_patches",
	} {
		p := filepath.Join(worktree, n)
		s, e := os.Lstat(p)
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			return e
		}
		if !s.Mode().IsRegular() {
			return fmt.Errorf("patch metadata must be a regular file: %s", p)
		}
	}

	return nil
}

func requirePreparedWorktree(worktree string) error {
	for _, name := range []string{".upstream-source", ".quilt-series"} {
		info, err := os.Lstat(filepath.Join(worktree, name))
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("refusing existing directory without overlay worktree metadata %s: %s", name, worktree)
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("patch metadata must be a regular file: %s", filepath.Join(worktree, name))
		}
	}
	return nil
}

func lines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}

	return strings.Split(s, "\n")
}

func Stack(worktree, patches string) ([]string, []string, error) {
	if e := ValidateMetadata(worktree); e != nil {
		return nil, nil, e
	}

	names, e := PatchNames(patches)
	if e != nil {
		return nil, nil, e
	}

	b, e := os.ReadFile(filepath.Join(worktree, ".pc/applied-patches"))
	if e != nil && !errors.Is(e, os.ErrNotExist) {
		return nil, nil, e
	}

	applied := lines(string(b))
	if len(applied) > len(names) || !slices.Equal(applied, names[:len(applied)]) {
		return nil, nil, fmt.Errorf("applied stack differs from repository order or has missing patches; pop with Quilt before continuing: %s", worktree)
	}

	return names, applied, nil
}

func prepareSeries(worktree, patches string) ([]string, error) {
	names, _, e := Stack(worktree, patches)
	if e != nil {
		return nil, e
	}

	body := ""
	if len(names) > 0 {
		body = strings.Join(names, "\n") + "\n"
	}

	if e = process.AtomicWrite(filepath.Join(worktree, ".quilt-series"), []byte(body), 0600); e != nil {
		return nil, e
	}

	if s, e := os.Stat(filepath.Join(worktree, ".pc")); e == nil && s.IsDir() {
		e = process.AtomicWrite(filepath.Join(worktree, ".pc/.quilt_series"), []byte(filepath.Join(worktree, ".quilt-series")+"\n"), 0600)
		if e != nil {
			return nil, e
		}
	}

	return names, nil
}

func environment(worktree, patches string) []string {
	return process.Env(map[string]string{
		"LC_ALL":           "C",
		"QUILT_PC":         ".pc",
		"QUILT_PATCH_OPTS": "",
		"QUILTRC":          "/dev/null",
		"QUILT_SERIES":     filepath.Join(worktree, ".quilt-series"),
		"QUILT_PATCHES":    patches,
	}, "args")
}

func commandAt(dir string, env []string, args ...string) *exec.Cmd {
	c := exec.Command(args[0], args[1:]...)
	c.Dir = dir
	c.Env = env
	c.Stdin = os.Stdin
	c.Stderr = os.Stderr
	c.Stdout = os.Stdout
	process.LogCommand(os.Stderr, c)
	return c
}

func (o *Overlay) quilt(worktree, patches string, args ...string) *exec.Cmd {
	return commandAt(worktree, environment(worktree, patches), append([]string{o.Quilt}, args...)...)
}

func (o *Overlay) Setup(pkg, supplied string) (string, error) {
	if e := o.ValidatePackage(pkg); e != nil {
		return "", e
	}

	if _, e := exec.LookPath(o.Quilt); e != nil {
		return "", e
	}

	worktree := filepath.Join(o.Worktrees, pkg)
	if e := ValidateMetadata(worktree); e != nil {
		return "", e
	}

	if s, e := os.Stat(worktree); e == nil && s.IsDir() {
		if e := requirePreparedWorktree(worktree); e != nil {
			return "", e
		}
		if supplied != "" {
			source, e := o.Source(pkg, supplied)
			if e != nil {
				return "", e
			}

			b, e := os.ReadFile(filepath.Join(worktree, ".upstream-source"))
			if e != nil || strings.TrimSuffix(string(b), "\n") != source {
				return "", fmt.Errorf("source changed; discard %s before setting up a new source", pkg)
			}
		}

		b, e := os.ReadFile(filepath.Join(worktree, ".pc/.quilt_patches"))
		if e == nil && strings.TrimSuffix(string(b), "\n") != "" && strings.TrimSuffix(string(b), "\n") != o.patches(pkg) {
			return "", fmt.Errorf("worktree uses another patch location; discard %s before editing", pkg)
		}

		_, e = prepareSeries(worktree, o.patches(pkg))
		return worktree, e
	}

	source, e := o.Source(pkg, supplied)
	if e != nil {
		return "", e
	}

	if e = os.MkdirAll(o.Worktrees, 0755); e != nil {
		return "", e
	}

	temporary, e := os.MkdirTemp(o.Worktrees, ".setup-"+pkg+".")
	if e != nil {
		return "", e
	}
	defer os.RemoveAll(temporary)

	staged := filepath.Join(temporary, "source")
	if e = copySource(source, staged); e != nil {
		return "", e
	}

	if e = process.AtomicWrite(filepath.Join(staged, ".upstream-source"), []byte(source+"\n"), 0600); e != nil {
		return "", e
	}

	names, e := prepareSeries(staged, o.patches(pkg))
	if e != nil {
		return "", e
	}

	if len(names) > 0 {
		if e = o.quilt(staged, o.patches(pkg), "push", "-a").Run(); e != nil {
			return "", e
		}
	}

	if e = os.Rename(staged, worktree); e != nil {
		return "", e
	}

	_, e = prepareSeries(worktree, o.patches(pkg))
	return worktree, e
}

func (o *Overlay) RefreshStacks(packages []string) error {
	if len(packages) == 0 {
		all, e := o.Packages()
		if e != nil {
			return e
		}

		for _, p := range all {
			names, e := PatchNames(o.patches(p))
			if e != nil {
				return e
			}

			if len(names) > 0 {
				packages = append(packages, p)
			}
		}
	}

	unique := []string{}
	for _, p := range packages {
		if e := o.ValidatePackage(p); e != nil {
			return e
		}

		if !slices.Contains(unique, p) {
			unique = append(unique, p)
		}
	}

	if _, e := exec.LookPath(o.Quilt); e != nil {
		return e
	}

	tmp, e := os.MkdirTemp("", "package-patches.")
	if e != nil {
		return e
	}
	defer os.RemoveAll(tmp)

	type replacement struct{ source, target string }
	staged := []replacement{}
	for _, pkg := range unique {
		names, e := PatchNames(o.patches(pkg))
		if e != nil {
			return e
		}
		if len(names) == 0 {
			continue
		}

		source, e := o.Source(pkg, "")
		if e != nil {
			return e
		}

		worktree := filepath.Join(tmp, "worktrees", pkg)
		patches := filepath.Join(tmp, "patches", pkg)
		if e = copySource(source, worktree); e != nil {
			return e
		}

		if e = copySource(o.patches(pkg), patches); e != nil {
			return e
		}

		if _, e = prepareSeries(worktree, patches); e != nil {
			return e
		}

		for _, p := range names {
			ui.Progress(fmt.Sprintf("Refreshing %s/%s", pkg, p))
			if e = o.quilt(worktree, patches, "push", p).Run(); e != nil {
				return e
			}

			if e = o.quilt(worktree, patches, "refresh", "--no-index", "--no-timestamps").Run(); e != nil {
				return e
			}

			staged = append(staged, replacement{filepath.Join(patches, p), filepath.Join(o.patches(pkg), p)})
		}
	}

	for _, r := range staged {
		b, e := os.ReadFile(r.source)
		if e != nil {
			return e
		}

		s, e := os.Stat(r.target)
		if e != nil {
			return e
		}

		if e = process.AtomicWrite(r.target, b, s.Mode().Perm()); e != nil {
			return e
		}
	}

	for _, p := range unique {
		fmt.Println("Refreshed patches: " + p)
	}

	return nil
}
