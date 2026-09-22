package overlay_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func buildOverlay(t *testing.T) string {
	t.Helper()
	repository, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "overlay")
	cmd := exec.Command("go", "build", "-o", binary, "./cmd/overlay")
	cmd.Dir = repository
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build overlay: %v\n%s", err, output)
	}
	return binary
}

func writeFile(t *testing.T, path, contents string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), mode); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

func runOverlay(t *testing.T, binary, directory string, args ...string) string {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = directory
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("overlay %v: %v\nstdout: %s\nstderr: %s", args, err, &stdout, &stderr)
	}
	return stdout.String()
}

func TestProjectSelection(t *testing.T) {
	binary := buildOverlay(t)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "flake.nix"), "{}\n", 0600)
	writeFile(t, filepath.Join(root, "pkgs", "outer", "default.nix"), "{}\n", 0600)
	writeFile(t, filepath.Join(root, "definitions", "alternate", "default.nix"), "{}\n", 0600)
	inner := filepath.Join(root, "nested")
	writeFile(t, filepath.Join(inner, "flake.nix"), "{}\n", 0600)
	writeFile(t, filepath.Join(inner, "pkgs", "inner", "default.nix"), "{}\n", 0600)
	cwd := filepath.Join(inner, "pkgs", "inner")
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{"nearest flake", []string{"list"}, "inner\n"},
		{"explicit short directory", []string{"-C", root, "list"}, "outer\n"},
		{"explicit long directory", []string{"--directory", root, "list"}, "outer\n"},
		{"custom package directory", []string{"-C", root, "--packages-dir", "definitions", "list"}, "alternate\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if output := runOverlay(t, binary, cwd, test.args...); output != test.want {
				t.Fatalf("packages: %q, want %q", output, test.want)
			}
		})
	}

	t.Run("git repository without a flake", func(t *testing.T) {
		project := t.TempDir()
		if err := os.Mkdir(filepath.Join(project, ".git"), 0755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(project, "pkgs", "example", "default.nix"), "{}\n", 0600)
		if output := runOverlay(t, binary, filepath.Join(project, "pkgs"), "list"); output != "example\n" {
			t.Fatalf("packages: %q", output)
		}
	})
}

func realQuilt(t *testing.T) {
	t.Helper()
	quilt, err := exec.LookPath("quilt")
	if err != nil {
		t.Skip("real Quilt workflow requires quilt on PATH")
	}
	t.Setenv("QUILT", quilt)
	t.Setenv("SOURCE_ROOT", "")
	t.Setenv("QUILT_DIFF_ARGS", "")
	t.Setenv("QUILT_REFRESH_ARGS", "")
	t.Setenv("QUILT_PUSH_ARGS", "")
	t.Setenv("QUILT_PATCH_OPTS", "")
}

const firstPatch = `--- a/message.txt
+++ b/message.txt
@@ -1 +1 @@
-original
+patched
`

func TestQuiltPatchLifecycle(t *testing.T) {
	realQuilt(t)
	binary := buildOverlay(t)
	root := t.TempDir()
	source := t.TempDir()
	writeFile(t, filepath.Join(root, "flake.nix"), "{}\n", 0600)
	writeFile(t, filepath.Join(root, "pkgs", "example", "default.nix"), "{}\n", 0600)
	patches := filepath.Join(root, "pkgs", "example", "patches")
	writeFile(t, filepath.Join(patches, "0001-example.patch"), firstPatch, 0600)
	writeFile(t, filepath.Join(source, "message.txt"), "original\n", 0600)
	worktrees := filepath.Join(root, "worktrees with spaces")
	t.Setenv("PATCH_WORKTREES", filepath.Join(root, "unused-environment-worktrees"))
	worktree := filepath.Join(worktrees, "example")
	run := func(args ...string) string {
		t.Helper()
		return runOverlay(t, binary, root, append([]string{"--worktrees", worktrees}, args...)...)
	}
	run("setup", "example", source)
	if got := readFile(t, filepath.Join(worktree, "message.txt")); got != "patched\n" {
		t.Fatalf("setup did not apply first patch: %q", got)
	}
	if got := readFile(t, filepath.Join(source, "message.txt")); got != "original\n" {
		t.Fatalf("setup modified supplied source: %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, "unused-environment-worktrees")); !os.IsNotExist(err) {
		t.Fatalf("explicit worktree location did not override environment: %v", err)
	}

	run("new", "example", "0002-revision.patch", "message.txt")
	editor := filepath.Join(t.TempDir(), "editor")
	writeFile(t, editor, "#!/bin/sh\nfor file do printf 'revised\\n' > \"$file\"; done\n", 0700)
	t.Setenv("EDITOR", editor)
	run("edit", "example", "message.txt")
	run("refresh", "example")
	secondPatch := filepath.Join(patches, "0002-revision.patch")
	refreshed := readFile(t, secondPatch)
	if !strings.Contains(refreshed, "-patched\n+revised\n") {
		t.Fatalf("refresh did not save edited content: %s", refreshed)
	}
	if strings.Contains(refreshed, "Index:") || strings.Contains(refreshed, "\t") {
		t.Fatalf("refresh retained index or timestamp metadata: %s", refreshed)
	}
	if got := run("status", "example"); !strings.Contains(got, "2 applied") || !strings.Contains(got, "current") {
		t.Fatalf("unexpected applied stack: %s", got)
	}

	run("discard", "example")
	if _, err := os.Stat(worktree); !os.IsNotExist(err) {
		t.Fatalf("discard left worktree: %v", err)
	}
	if got := readFile(t, secondPatch); got != refreshed {
		t.Fatal("discard altered repository patch")
	}
	run("setup", "example", source)
	if got := readFile(t, filepath.Join(worktree, "message.txt")); got != "revised\n" {
		t.Fatalf("saved stack did not reapply after discard: %q", got)
	}
	if got := readFile(t, filepath.Join(source, "message.txt")); got != "original\n" {
		t.Fatalf("workflow modified original source: %q", got)
	}
}

func TestFailedSetupRollsBackPartialStack(t *testing.T) {
	realQuilt(t)
	binary := buildOverlay(t)
	root := t.TempDir()
	source := t.TempDir()
	writeFile(t, filepath.Join(root, "flake.nix"), "{}\n", 0600)
	writeFile(t, filepath.Join(root, "pkgs", "example", "default.nix"), "{}\n", 0600)
	writeFile(t, filepath.Join(source, "message.txt"), "original\n", 0600)
	patches := filepath.Join(root, "pkgs", "example", "patches")
	writeFile(t, filepath.Join(patches, "0001-example.patch"), firstPatch, 0600)
	badPatch := `--- a/message.txt
+++ b/message.txt
@@ -1 +1 @@
-unexpected upstream content
+replacement
`
	badPath := filepath.Join(patches, "0002-incompatible.patch")
	writeFile(t, badPath, badPatch, 0600)
	t.Setenv("PATCH_WORKTREES", "")
	worktrees := filepath.Join(root, ".patch-worktrees", "pkgs")
	writeFile(t, filepath.Join(worktrees, "other", "keep.txt"), "untouched\n", 0600)
	cmd := exec.Command(binary, "setup", "example", source)
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("incompatible patch unexpectedly applied: %s", output)
	}
	entries, err := os.ReadDir(worktrees)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "other" {
		t.Fatalf("failed setup left worktree or temporary state: %v", entries)
	}
	if readFile(t, filepath.Join(worktrees, "other", "keep.txt")) != "untouched\n" {
		t.Fatal("failed setup modified another worktree")
	}
	if readFile(t, filepath.Join(source, "message.txt")) != "original\n" {
		t.Fatal("failed setup modified supplied source")
	}
	if readFile(t, filepath.Join(patches, "0001-example.patch")) != firstPatch || readFile(t, badPath) != badPatch {
		t.Fatal("failed setup changed repository patches")
	}
	if err := os.Remove(badPath); err != nil {
		t.Fatal(err)
	}
	runOverlay(t, binary, root, "setup", "example", source)
	if got := readFile(t, filepath.Join(worktrees, "example", "message.txt")); got != "patched\n" {
		t.Fatalf("retry after correcting stack failed: %q", got)
	}
}
