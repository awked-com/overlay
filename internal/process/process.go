// Package process handles subprocesses and atomic file updates.
package process

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"

	"github.com/awked-com/overlay/internal/terminal"
)

var Nix = []string{"nix", "--extra-experimental-features", "nix-command flakes"}

var (
	commandWord    = regexp.MustCompile(`^[a-zA-Z0-9_@%+=:,./-]+$`)
	urlCredentials = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)[^/\s]*@`)
)

func commandArgument(arg string) string {
	arg = urlCredentials.ReplaceAllString(arg, "${1}<redacted>@")
	if !commandWord.MatchString(arg) {
		return Quote(arg)
	}
	return arg
}

// CommandString quotes arguments and redacts URL credentials for display only.
// Callers must keep other secrets out of command arguments.
func CommandString(args []string) string {
	display := make([]string, len(args))
	for i, arg := range args {
		display[i] = commandArgument(arg)
	}
	return strings.Join(display, " ")
}

// LogCommand omits environment and stdin, and redacts URL credentials.
// Callers must keep other secrets out of command arguments.
func LogCommand(w io.Writer, cmd *exec.Cmd) {
	line := CommandString(cmd.Args)
	if cmd.Dir != "" {
		line = "(cd " + commandArgument(cmd.Dir) + " && " + line + ")"
	}
	fmt.Fprintln(w, terminal.Style(w, terminal.Dim, "Running: "+line))
}

func Run(args ...string) error {
	c := exec.Command(args[0], args[1:]...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	LogCommand(os.Stderr, c)
	if err := c.Run(); err != nil {
		return fmt.Errorf("%s failed: %w", filepath.Base(args[0]), err)
	}
	return nil
}

func Quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }

func Env(values map[string]string, remove ...string) []string {
	out := []string{}
	for _, v := range os.Environ() {
		k, _, _ := strings.Cut(v, "=")
		if _, ok := values[k]; ok || slices.Contains(remove, k) {
			continue
		}
		out = append(out, v)
	}

	for k, v := range values {
		out = append(out, k+"="+v)
	}

	return out
}

func AtomicWrite(path string, data []byte, mode os.FileMode) error {
	f, e := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path))
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	defer f.Close()

	if _, e = f.Write(data); e != nil {
		return e
	}

	if e = f.Chmod(mode); e != nil {
		return e
	}

	if e = f.Sync(); e != nil {
		return e
	}

	if e = f.Close(); e != nil {
		return e
	}

	return os.Rename(f.Name(), path)
}

func CopyFile(src, dst string, mode os.FileMode) error {
	in, e := os.Open(src)
	if e != nil {
		return e
	}
	defer in.Close()

	out, e := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if e != nil {
		return e
	}

	_, e = io.Copy(out, in)
	ce := out.Close()
	if e != nil {
		return e
	}

	return ce
}

// StatusError preserves a command-specific status, including failures without a diagnostic.
type StatusError struct {
	Code int
	Err  error
}

func (e *StatusError) Error() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}
func (e *StatusError) Unwrap() error { return e.Err }

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var status *StatusError
	if errors.As(err, &status) {
		return status.Code
	}
	var child *exec.ExitError
	if errors.As(err, &child) {
		if child.ExitCode() > 0 {
			return child.ExitCode()
		}
		if status, ok := child.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return 128 + int(status.Signal())
		}
	}
	return 1
}

func Report(err error) int {
	if err != nil && err.Error() != "" {
		fmt.Fprintln(os.Stderr, terminal.Style(os.Stderr, terminal.Bold+";"+terminal.Red, "error:"), err)
	}
	return ExitCode(err)
}

func Exit(err error) { os.Exit(Report(err)) }
