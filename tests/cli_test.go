package overlay_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOverlayThroughCLI(t *testing.T) {
	binary := buildOverlay(t)
	root := t.TempDir()
	write := func(name, contents string, mode os.FileMode) {
		t.Helper()
		writeFile(t, filepath.Join(root, name), contents, mode)
	}
	write("flake.nix", "{}\n", 0600)
	write("pkgs/fixture/default.nix", "{}\n", 0600)
	write("quilt", `#!/bin/sh
test -f .quilt-series || exit 71
test "$QUILT_SERIES" = "$PATCH_WORKTREES/fixture/.quilt-series" || exit 72
if [ "$1" = files ]; then
  printf ' tracked file \n'
else
  printf 'arg:%s\n' "$@"
  cat
fi
exit "${OVERLAY_TEST_QUILT_EXIT:-0}"
`, 0700)
	write("editor", "#!/bin/sh\nprintf 'editor:%s\\n' \"$@\"\n", 0700)
	t.Setenv("EDITOR", filepath.Join(root, "editor")+" --flag 'a value'")

	t.Setenv("QUILT", filepath.Join(root, "quilt"))
	t.Setenv("PATCH_WORKTREES", filepath.Join(root, "worktrees"))
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("CLICOLOR_FORCE", "1")
	for _, test := range []struct {
		name     string
		args     []string
		prepared bool
		child    string
		code     int
		stdout   string
		hint     string
	}{
		{"help", []string{"help", "quilt"}, false, "0", 0, "", ""},
		{"missing arguments", []string{"quilt", "fixture"}, false, "0", 2, "", "overlay quilt --help"},
		{"unprepared", []string{"quilt", "fixture", "push"}, false, "0", 1, "", "overlay setup fixture"},
		{"forwarded arguments", []string{"quilt", "fixture", "refresh", "a file", "--help", "$literal"}, true, "0", 0, "arg:refresh\narg:a file\narg:--help\narg:$literal\ninput\n", "refresh 'a file' --help '$literal'"},
		{"setup prepared worktree", []string{"setup", "fixture"}, true, "0", 0, "Prepared worktree: " + filepath.Join(root, "worktrees", "fixture") + "\n", ""},
		{"edit files", []string{"edit", "fixture", " tracked file ", "new file"}, true, "0", 0, "arg:add\narg:new file\ninput\neditor:--flag\neditor:a value\neditor: tracked file \neditor:new file\n", "editor --flag 'a value' ' tracked file ' 'new file'"},
		{"child failure", []string{"quilt", "fixture", "push"}, true, "17", 17, "arg:push\ninput\n", "error:"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.prepared {
				write("worktrees/fixture/.quilt-series", "", 0600)
				write("worktrees/fixture/.upstream-source", "/example/source\n", 0600)
			}
			t.Setenv("OVERLAY_TEST_QUILT_EXIT", test.child)
			cmd := exec.Command(binary, test.args...)
			cmd.Dir = root
			cmd.Stdin = strings.NewReader("input\n")
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			code := 0
			if err := cmd.Run(); err != nil {
				failure, ok := err.(*exec.ExitError)
				if !ok {
					t.Fatal(err)
				}
				code = failure.ExitCode()
			}
			if code != test.code {
				t.Fatalf("status %d, want %d: %s", code, test.code, &stderr)
			}
			if strings.Contains(stdout.String()+stderr.String(), "\x1b[") {
				t.Fatal("redirected output contains terminal color escapes")
			}
			if test.name == "help" {
				if stdout.Len() == 0 || strings.Contains(stdout.String(), "arg:") {
					t.Fatalf("help output: %s", &stdout)
				}
			} else if stdout.String() != test.stdout {
				t.Fatalf("stdout: %q, want %q", &stdout, test.stdout)
			}
			if test.hint == "" && stderr.Len() != 0 || !strings.Contains(stderr.String(), test.hint) {
				t.Fatalf("stderr: %s", &stderr)
			}
			if code != 0 && strings.Count(stderr.String(), "error:") != 1 {
				t.Fatalf("expected one error: %s", &stderr)
			}
		})
	}
}
