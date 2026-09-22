# overlay

A standalone command for maintaining numbered package patches with Quilt.
It prepares worktrees from a project's locked Nix sources, applies patch stacks,
and refreshes patches back into the project. It has no dependency on a particular
NixOS configuration or organization.

## Install and run

From this checkout:

```sh
nix profile add /path/to/overlay
overlay -C /path/to/project list
```

Or run without installing:

```sh
nix run /path/to/overlay -- -C /path/to/project list
```

The Nix package includes Quilt, Nix, and archive tools in the command's runtime
environment. It supports x86_64 Linux, aarch64 Linux, and aarch64 macOS, and
installs Bash, Zsh, and Fish completions. `nix build` produces the package;
`packages.<system>.overlay`, `apps.<system>.overlay`, and `overlays.default`
are also exported.

## Project layout

By default, packages live at `pkgs/<name>/default.nix` and patches at
`pkgs/<name>/patches/NNNN-description.patch`. `--packages-dir` selects a different
package directory. Patch filenames define application order; do not maintain a
repository `series` file. The package recipe or overlay must attach these patches
to its derivation; this command does not modify Nix recipes.

`-C` / `--directory` selects the project. Without it, the command finds the nearest
parent project containing a flake or Git repository. Locked source lookup first
uses `packages.<system>.<name>` from the project's flake. Otherwise it imports
the flake's `nixpkgs` input with its default overlay. `--system` selects the Nix
system. An explicit source directory also works without Nix and must already be
unpacked and unpatched.

Git projects use tracked files, including local edits, for source lookup. Add
new Nix source files to Git first; ignored worktrees are excluded. Source lookup
does not update the project's lockfile.

## Patch workflow

```sh
overlay -C /path/to/project list
overlay -C /path/to/project setup hello
overlay -C /path/to/project status hello
overlay -C /path/to/project new hello 0001-fix-greeting.patch src/hello.c
overlay -C /path/to/project edit hello src/hello.c
overlay -C /path/to/project refresh hello
```

Use `select PACKAGE PATCH` to edit an existing patch, `quilt PACKAGE COMMAND
[ARGS...]` to run Quilt directly, or `shell PACKAGE` for an interactive shell with
the Quilt environment configured. `refresh-stacks [PACKAGE...]` refreshes whole
stacks. `discard PACKAGE` removes its prepared worktree, including unrefreshed
edits, and preserves repository patch files. See `overlay help` and each
subcommand's `--help` for details.

Worktrees default to `.patch-worktrees/pkgs` under the project. Use `--worktrees`
or `PATCH_WORKTREES` to move them. `SOURCE_ROOT/PACKAGE` can supply sources instead
of fetching them; an explicit path passed to `setup PACKAGE PATH` takes priority.
`QUILT`, `EDITOR`, and `SHELL` select their respective programs.
Package and worktree directories must not overlap. Existing worktrees must
contain this tool's metadata before setup reuses them or discard removes them.

Refresh edits before leaving a Quilt shell. Pop affected patches before
reordering filenames. After updating an upstream source pin, preserve edits,
discard the old worktree, then run setup again. Keep `.patch-worktrees/` out of
version control.

## Relationship to NixOS

Nixpkgs already applies patches declared through `patches` and `overrideAttrs`.
For a simple change, an ordinary patch file and a package override are sufficient.
This command manages the editing and refreshing of patch stacks; Nix still owns
source selection, package definitions, and builds.

Use it in any repository that maintains package source patches, including a
NixOS modules repository with such packages. NixOS module configuration itself
uses the module system (`imports`, options, and overrides) and does not require
Quilt or this command.

## Develop

```sh
nix develop
go test ./...
go vet ./...
nix flake check
nix fmt
```

The flake check builds the command and runs its Go tests on the current system.
Native source-resolution tests run outside the build sandbox with a local Nix
store. They use synthetic Git repositories and local flake inputs without
downloading dependencies:

```sh
nix develop -c env OVERLAY_NATIVE_NIX_TESTS=1 go test -run TestNativeNixSources -count=1 .
```

The included CI workflow runs both checks on all three supported systems.
