package overlay_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOverlappingWorktreesPreservePackageDefinitions(t *testing.T) {
	binary := buildOverlay(t)
	for _, test := range []struct {
		name      string
		worktrees string
	}{
		{"package root", "pkgs"},
		{"ancestor of packages", "."},
		{"uncreated descendant", "pkgs/not-created/worktrees"},
		{"symlinked uncreated descendant", "links/packages/not-created/worktrees"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "flake.nix"), "{}\n", 0600)
			definition := filepath.Join(root, "pkgs", "demo", "default.nix")
			patch := filepath.Join(root, "pkgs", "demo", "patches", "0001-example.patch")
			writeFile(t, definition, "{}\n", 0600)
			writeFile(t, patch, firstPatch, 0600)
			if err := os.Mkdir(filepath.Join(root, "links"), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(root, "pkgs"), filepath.Join(root, "links", "packages")); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(binary, "-C", root, "--worktrees", test.worktrees, "discard", "demo")
			if output, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(output), "must not overlap") {
				t.Fatalf("expected overlapping directories to be rejected: err=%v output=%s", err, output)
			}
			if readFile(t, definition) != "{}\n" || readFile(t, patch) != firstPatch {
				t.Fatal("rejected discard changed a package definition or patch")
			}
			if _, err := os.Stat(filepath.Join(root, "pkgs", "not-created")); !os.IsNotExist(err) {
				t.Fatalf("rejected operation created worktree ancestors: %v", err)
			}
		})
	}

	t.Run("sibling with a shared name prefix is allowed", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "pkgs", "demo", "default.nix"), "{}\n", 0600)
		runOverlay(t, binary, root, "-C", root, "--worktrees", "pkgs-worktrees", "discard", "demo")
		if _, err := os.Stat(filepath.Join(root, "pkgs-worktrees")); !os.IsNotExist(err) {
			t.Fatalf("discard of an absent worktree created state: %v", err)
		}
	})
}

func TestExistingUnpreparedDirectoriesArePreserved(t *testing.T) {
	binary := buildOverlay(t)
	// Setup checks for Quilt before inspecting a worktree. No child command
	// should run in these cases, so a present executable suffices for that check.
	t.Setenv("QUILT", "/bin/sh")
	t.Setenv("PATCH_WORKTREES", "")
	for _, test := range []struct {
		name   string
		marker string
	}{
		{"no metadata", ""},
		{"only source marker", ".upstream-source"},
		{"only series marker", ".quilt-series"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "flake.nix"), "{}\n", 0600)
			writeFile(t, filepath.Join(root, "pkgs", "demo", "default.nix"), "{}\n", 0600)
			worktree := filepath.Join(root, ".patch-worktrees", "pkgs", "demo")
			writeFile(t, filepath.Join(worktree, "keep.txt"), "unrelated content\n", 0600)
			if test.marker != "" {
				writeFile(t, filepath.Join(worktree, test.marker), "existing metadata\n", 0600)
			}
			for _, command := range []string{"setup", "discard"} {
				cmd := exec.Command(binary, command, "demo")
				cmd.Dir = root
				if output, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(output), "without overlay worktree metadata") {
					t.Fatalf("%s adopted or removed an unprepared directory: err=%v output=%s", command, err, output)
				}
				if readFile(t, filepath.Join(worktree, "keep.txt")) != "unrelated content\n" {
					t.Fatalf("%s altered unrelated content", command)
				}
				for _, marker := range []string{".upstream-source", ".quilt-series"} {
					path := filepath.Join(worktree, marker)
					if marker == test.marker {
						if readFile(t, path) != "existing metadata\n" {
							t.Fatalf("%s rewrote preexisting metadata", command)
						}
					} else if _, err := os.Stat(path); !os.IsNotExist(err) {
						t.Fatalf("%s created missing metadata: %v", command, err)
					}
				}
			}
		})
	}
}
