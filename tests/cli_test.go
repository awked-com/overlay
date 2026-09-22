package overlay_test

import (
	"bytes"
	"fmt"
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
		{"help", []string{"help", "quilt"}, false, "0", 0, "overlay quilt PACKAGE COMMAND [ARGS...]", ""},
		{"list", []string{"list"}, false, "0", 0, "fixture\n", ""},
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
				if !strings.Contains(stdout.String(), test.stdout) || strings.Contains(stdout.String(), "arg:") {
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

	for _, test := range []struct {
		name     string
		patches  int
		applied  int
		prepared bool
	}{
		{"empty", 0, 0, true},
		{"unprepared", 3, 0, false},
		{"pending", 3, 0, true},
		{"mixed", 3, 2, true},
		{"applied", 3, 3, true},
	} {
		t.Run("stack "+test.name, func(t *testing.T) {
			pkg := "stack-" + test.name
			write("pkgs/"+pkg+"/default.nix", "{}\n", 0600)
			var patches []string
			for i := range test.patches {
				name := fmt.Sprintf("%04d-example.patch", i+1)
				patches = append(patches, name)
				write("pkgs/"+pkg+"/patches/"+name, "", 0600)
			}
			if test.prepared {
				write("worktrees/"+pkg+"/.quilt-series", strings.Join(patches, "\n"), 0600)
				applied := ""
				if test.applied > 0 {
					applied = strings.Join(patches[:test.applied], "\n") + "\n"
				}
				write("worktrees/"+pkg+"/.pc/applied-patches", applied, 0600)
			}
			cmd := exec.Command(binary, "status", pkg)
			cmd.Dir = root
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("status: %v\n%s", err, &stderr)
			}
			output := stdout.String()
			if stderr.Len() != 0 || strings.Contains(output, "\x1b[") {
				t.Fatalf("unexpected stderr or color escapes: stdout=%q stderr=%q", output, &stderr)
			}
			for _, count := range []string{
				fmt.Sprintf("%d patches", test.patches),
				fmt.Sprintf("%d applied", test.applied),
				fmt.Sprintf("%d pending", test.patches-test.applied),
			} {
				if !strings.Contains(output, count) {
					t.Errorf("missing count %q: %s", count, output)
				}
			}
			if test.prepared {
				if !strings.Contains(output, filepath.Join(root, "worktrees", pkg)) {
					t.Errorf("missing worktree path: %s", output)
				}
			} else if !strings.Contains(output, "overlay setup "+pkg) {
				t.Errorf("missing setup hint: %s", output)
			}
			var rows [][]string
			for _, line := range strings.Split(output, "\n") {
				fields := strings.Fields(line)
				if len(fields) == 3 && strings.HasSuffix(fields[2], ".patch") {
					rows = append(rows, fields)
				}
			}
			if len(rows) != len(patches) {
				t.Fatalf("got %d patch rows, want %d: %s", len(rows), len(patches), output)
			}
			for i, row := range rows {
				state := "pending"
				if i < test.applied {
					state = "applied"
				}
				if i == test.applied-1 {
					state = "current"
				}
				if row[1] != state || row[2] != patches[i] {
					t.Errorf("patch row %q, want %s %s", row, state, patches[i])
				}
			}
		})
	}
}
