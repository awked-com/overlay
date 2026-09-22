package overlay

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These checks exercise Nix's real flake filtering and evaluation semantics.
// The fixtures have only local inputs and need no package builds or downloads.
func TestNativeNixSources(t *testing.T) {
	if os.Getenv("OVERLAY_NATIVE_NIX_TESTS") != "1" {
		t.Skip("set OVERLAY_NATIVE_NIX_TESTS=1 to exercise native Nix source lookup")
	}
	for _, tool := range []string{"nix", "git"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatalf("native source tests require %s: %v", tool, err)
		}
	}
	t.Setenv("SOURCE_ROOT", "")
	t.Setenv("PATCH_WORKTREES", "")
	t.Setenv("NIX_CONFIG", os.Getenv("NIX_CONFIG")+"\nsubstituters =\nbuilders =\nflake-registry =\n")

	t.Run("tracked local changes and ignored worktrees", func(t *testing.T) {
		root := nativeSourceProject(t, map[string]string{
			"flake.nix": `{ outputs = { self }: {
  packages.${builtins.currentSystem}.demo.src = ./.;
}; }`,
			".gitignore":  ".patch-worktrees/\nignored.txt\n",
			"tracked.txt": "committed source\n",
		})
		nativeSourceWrite(t, root, "tracked.txt", "local source edit\n")
		nativeSourceWrite(t, root, "ignored.txt", "ignored fixture\n")
		nativeSourceWrite(t, root, "untracked.txt", "untracked fixture\n")
		nativeSourceWrite(t, root, ".patch-worktrees/pkgs/demo/work.txt", "worktree fixture\n")

		source := nativeSourceResolve(t, root, "")
		nativeSourceContent(t, source, "tracked.txt", "local source edit\n")
		for _, excluded := range []string{"ignored.txt", "untracked.txt", ".patch-worktrees", ".git"} {
			if _, err := os.Lstat(filepath.Join(source, excluded)); !os.IsNotExist(err) {
				t.Errorf("Git-filtered source includes %s, or stat failed: %v", excluded, err)
			}
		}
		if _, err := os.Stat(filepath.Join(root, "flake.lock")); !os.IsNotExist(err) {
			t.Errorf("source lookup wrote an unexpected lock file: %v", err)
		}
	})

	t.Run("nested Git flake with literal path characters", func(t *testing.T) {
		const nested = `nested "quotes" ${literal}`
		root := nativeSourceProject(t, map[string]string{
			filepath.Join(nested, "flake.nix"): `{ outputs = { self }: {
  packages.${builtins.currentSystem}.demo.src = ./.;
}; }`,
			filepath.Join(nested, "tracked.txt"): "committed nested source\n",
			".gitignore":                         ".patch-worktrees/\nignored.txt\n",
			"outside.txt":                        "outside the selected flake\n",
		})
		project := filepath.Join(root, nested)
		nativeSourceWrite(t, project, "tracked.txt", "nested local source edit\n")
		nativeSourceWrite(t, project, "ignored.txt", "ignored fixture\n")
		nativeSourceWrite(t, project, ".patch-worktrees/pkgs/demo/work.txt", "worktree fixture\n")

		source := nativeSourceResolve(t, project, "")
		nativeSourceContent(t, source, "tracked.txt", "nested local source edit\n")
		for _, excluded := range []string{"ignored.txt", ".patch-worktrees", "outside.txt"} {
			if _, err := os.Lstat(filepath.Join(source, excluded)); !os.IsNotExist(err) {
				t.Errorf("nested source includes %s, or stat failed: %v", excluded, err)
			}
		}
	})

	t.Run("non-Git flake with literal path characters", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), `project "quotes" ${literal}`)
		nativeSourceWrite(t, root, "flake.nix", `{ outputs = { self }: {
  packages.${builtins.currentSystem}.demo.src = ./source;
}; }`)
		nativeSourceWrite(t, root, "source/content.txt", "non-Git source\n")
		source := nativeSourceResolve(t, root, "")
		nativeSourceContent(t, source, "content.txt", "non-Git source\n")
	})

	t.Run("package output wins over overlay fallback", func(t *testing.T) {
		root := nativeSourceProject(t, map[string]string{
			"flake.nix": `{
  inputs.nixpkgs.url = "path:./mock-nixpkgs";
  outputs = { self, nixpkgs }: {
    packages.${builtins.currentSystem}.demo.src = ./selected;
    overlays.default = final: prev: throw "overlay fallback must remain unevaluated";
  };
}`,
			"selected/content.txt":     "package output\n",
			"mock-nixpkgs/flake.nix":   `{ outputs = { self }: {}; }`,
			"mock-nixpkgs/default.nix": `{ system, overlays ? [] }: throw "nixpkgs fallback must remain unevaluated"`,
		})
		nativeSourceLock(t, root)
		source := nativeSourceResolve(t, root, "")
		nativeSourceContent(t, source, "content.txt", "package output\n")
	})

	t.Run("composed default overlay fallback", func(t *testing.T) {
		root := nativeSourceProject(t, map[string]string{
			"flake.nix": `{
  inputs.nixpkgs.url = "path:./mock-nixpkgs";
  outputs = { self, nixpkgs }: {
    overlays.default = final: prev:
      let
        shared = final: prev: { demo = prev.demo // { src = ./shared; }; };
        project = final: prev: { demo = prev.demo // { src = prev.demo.src + "/nested"; }; };
        sharedResult = shared final prev;
      in sharedResult // project final (prev // sharedResult);
  };
}`,
			"shared/nested/content.txt": "composed overlay\n",
			"mock-nixpkgs/flake.nix":    `{ outputs = { self }: {}; }`,
			"mock-nixpkgs/default.nix": `{ system, overlays ? [] }:
let
  base = { demo.src = ./upstream; };
  final = builtins.foldl' (prev: overlay: prev // overlay final prev) base overlays;
in final`,
			"mock-nixpkgs/upstream/content.txt": "unoverlaid source\n",
		})
		nativeSourceLock(t, root)
		source := nativeSourceResolve(t, root, "")
		nativeSourceContent(t, source, "content.txt", "composed overlay\n")
	})

	t.Run("explicit Linux system", func(t *testing.T) {
		for _, viaOutput := range []bool{true, false} {
			name := "nixpkgs fallback"
			if viaOutput {
				name = "package output"
			}
			t.Run(name, func(t *testing.T) {
				files := map[string]string{
					"flake.nix": `{ outputs = { self }: {
  packages.x86_64-linux.demo.src = ./linux;
}; }`,
					"linux/content.txt": "Linux source\n",
				}
				if !viaOutput {
					files = map[string]string{
						"flake.nix": `{
  inputs.nixpkgs.url = "path:./mock-nixpkgs";
  outputs = { self, nixpkgs }: {};
}`,
						"mock-nixpkgs/flake.nix": `{ outputs = { self }: {}; }`,
						"mock-nixpkgs/default.nix": `{ system, overlays ? [] }:
assert system == "x86_64-linux";
{ demo.src = ./linux; }`,
						"mock-nixpkgs/linux/content.txt": "Linux source\n",
					}
				}
				root := nativeSourceProject(t, files)
				if !viaOutput {
					nativeSourceLock(t, root)
				}
				source := nativeSourceResolve(t, root, "x86_64-linux")
				nativeSourceContent(t, source, "content.txt", "Linux source\n")
			})
		}
	})
}

func nativeSourceProject(t *testing.T, files map[string]string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "project with spaces")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	// Resolve macOS's /var alias so Nix's fetched root and our fixture agree.
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	for path, content := range files {
		nativeSourceWrite(t, root, path, content)
	}
	nativeSourceCommand(t, root, "git", "init", "--quiet", "--initial-branch=main")
	nativeSourceCommand(t, root, "git", "config", "--local", "user.name", "Fixture")
	nativeSourceCommand(t, root, "git", "config", "--local", "user.email", "fixture@example.invalid")
	nativeSourceCommand(t, root, "git", "add", ".")
	nativeSourceCommand(t, root, "git", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "--quiet", "-m", "Create synthetic source fixture")
	return root
}

func nativeSourceWrite(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func nativeSourceCommand(t *testing.T, root, name string, args ...string) {
	t.Helper()
	command := exec.Command(name, args...)
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, output)
	}
}

func nativeSourceLock(t *testing.T, root string) {
	t.Helper()
	nativeSourceCommand(t, root, "nix", "--extra-experimental-features", "nix-command flakes", "flake", "lock", "--offline")
	nativeSourceCommand(t, root, "git", "add", "flake.lock")
}

func nativeSourceResolve(t *testing.T, root, system string) string {
	t.Helper()
	lockPath := filepath.Join(root, "flake.lock")
	before, beforeErr := os.ReadFile(lockPath)
	overlay := NewOverlay(root)
	overlay.System = system
	source, err := overlay.Source("demo", "")
	if err != nil {
		t.Fatalf("resolve source: %v", err)
	}
	after, afterErr := os.ReadFile(lockPath)
	if string(before) != string(after) || os.IsNotExist(beforeErr) != os.IsNotExist(afterErr) {
		t.Fatal("source lookup modified the project's lock file")
	}
	if !strings.HasPrefix(source, "/nix/store/") {
		t.Errorf("source was not realized in the Nix store: %s", source)
	}
	return source
}

func nativeSourceContent(t *testing.T, source, name, want string) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(source, name))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != want {
		t.Errorf("source %s = %q, want %q", name, content, want)
	}
}
