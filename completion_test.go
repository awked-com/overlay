package overlay

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/awked-com/overlay/internal/ui"
)

func TestProjectCompletion(t *testing.T) {
	root := t.TempDir()
	for name, data := range map[string]string{
		"flake.nix":                               "{}",
		"recipes/hello/default.nix":               "{}",
		"recipes/hello/patches/0001-first.patch":  "patch",
		"recipes/hello/patches/0002-second.patch": "patch",
		"recipes/world/default.nix":               "{}",
		"trees/hello/src/main.c":                  "source",
		"trees/hello/src/file with spaces.c":      "source",
		"trees/hello/.pc/state":                   "metadata",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", t.TempDir())
	t.Setenv("PATCH_WORKTREES", filepath.Join(root, "trees"))
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"setup", "h"}, "hello\n:4\n"},
		{[]string{"select", "hello", "0002"}, "0002-second.patch\n:4\n"},
		{[]string{"select", "hello", ""}, "0001-first.patch\n0002-second.patch\n:4\n"},
		{[]string{"select", "../escape", ""}, ":5\n"},
		{[]string{"quilt", "hello", "push", "0002"}, "0002-second.patch\n:4\n"},
		{[]string{"quilt", "hello", "pop", "0001"}, "0001-first.patch\n:4\n"},
		{[]string{"refresh-stacks", "hello", ""}, "world\n:4\n"},
		{[]string{"edit", "hello", "s"}, "src/\n:6\n"},
		{[]string{"edit", "hello", "src/main.c", "src/"}, "src/file with spaces.c\n:4\n"},
		{[]string{"new", "hello", "0003-next.patch", "src/m"}, "src/main.c\n:4\n"},
		{[]string{"setup", "hello", ""}, ":16\n"},
		{[]string{"--packages-dir", "r"}, "recipes/\n:6\n"},
		{[]string{"--system", "aarch64-d"}, "aarch64-darwin\n:4\n"},
	} {
		t.Run(strings.Join(test.args, "/"), func(t *testing.T) {
			args := append([]string{"__complete", "-C", root, "--packages-dir", "recipes"}, test.args...)
			var out, stderr bytes.Buffer
			if err := ui.ExecuteTo(Command(), args, &out, &stderr); err != nil || out.String() != test.want {
				t.Fatalf("got %q (%v) %s; want %q", &out, err, &stderr, test.want)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(root, ".patch-worktrees")); !os.IsNotExist(err) {
		t.Fatalf("completion created a worktree: %v", err)
	}
	t.Chdir(filepath.Join(root, "recipes"))
	var out bytes.Buffer
	if err := ui.ExecuteTo(Command(), []string{"__complete", "--packages-dir", "recipes", "select", "hello", "0002"}, &out, &bytes.Buffer{}); err != nil || out.String() != "0002-second.patch\n:4\n" {
		t.Fatalf("parent project: %q (%v)", &out, err)
	}
}
