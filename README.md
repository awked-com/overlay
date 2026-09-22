# overlay

Maintain numbered package patches with Quilt. `overlay` prepares worktrees from
locked Nix sources, applies patch stacks, and refreshes patches into the project.

## Install and run

```sh
nix profile add github:awked-com/overlay
overlay -C /path/to/project list
```

To run without installing:

```sh
nix run github:awked-com/overlay -- -C /path/to/project list
```

The package includes Quilt, Nix, archive tools, and Bash, Zsh, and Fish
completions. It supports x86_64 Linux, aarch64 Linux, and aarch64 macOS.
`nix build` builds the command. The flake exports `packages.<system>.overlay`,
`apps.<system>.overlay`, and `overlays.default`; the shared
[packages](https://github.com/awked-com/packages) overlay also provides
`pkgs.overlay` at a pinned revision.

## Project layout and sources

Packages live at `pkgs/<name>/default.nix` and patches at
`pkgs/<name>/patches/NNNN-description.patch`. `--packages-dir` selects another
package directory. Patch filenames define application order; do not maintain a
repository `series` file. The package recipe must attach the patches to its
derivation; `overlay` does not modify Nix recipes.

`-C` / `--directory` selects the project. Without it, `overlay` finds the nearest
parent containing a flake or Git repository. Source lookup uses
`packages.<system>.<name>` from the project's flake, falling back to the flake's
`nixpkgs` input with its default overlay. `--system` selects the Nix system.
Git projects use tracked files, including local edits; add new Nix source files
to Git first. Ignored worktrees are excluded, and lookup leaves the lockfile
unchanged.

`setup PACKAGE PATH` uses an unpacked, unpatched source directory without Nix.
If PATH is omitted, `SOURCE_ROOT/PACKAGE` takes priority over flake lookup.

## Patch workflow

```sh
overlay -C /path/to/project setup hello
overlay -C /path/to/project status hello
overlay -C /path/to/project new hello 0001-fix-greeting.patch src/hello.c
overlay -C /path/to/project edit hello src/hello.c
overlay -C /path/to/project refresh hello
```

Use `select PACKAGE PATCH` to edit an existing patch, `quilt PACKAGE COMMAND
[ARGS...]` to run Quilt directly, or `shell PACKAGE` to open a shell with the Quilt
environment. `refresh-stacks [PACKAGE...]` refreshes whole stacks.
`discard PACKAGE` removes its worktree, including unrefreshed edits, and preserves
repository patches. See each command's `--help` for arguments.

Worktrees default to `.patch-worktrees/pkgs` under the project. `--worktrees` or
`PATCH_WORKTREES` selects another location. Keep worktrees out of version control
and separate from package directories. Existing worktrees must contain this
tool's metadata before setup reuses them or discard removes them.
`QUILT`, `EDITOR`, and `SHELL` select their respective programs.

Refresh edits before leaving a Quilt shell. Pop affected patches before
reordering filenames. After updating a source pin, preserve edits, discard the
old worktree, then run setup again.

## Develop

```sh
nix develop
go test ./...
go vet ./...
nix flake check
nix fmt
```

The flake check builds the command and runs its Go tests. Native source-resolution
tests run outside the build sandbox with a local Nix store. Their synthetic Git
repositories and local flake inputs require no downloads:

```sh
nix develop -c env OVERLAY_NATIVE_NIX_TESTS=1 go test -run TestNativeNixSources -count=1 .
```

CI runs package and native source checks, `actionlint`, and Nix formatting checks
on all three supported systems.
