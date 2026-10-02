# overlay

Maintain numbered package patches with Quilt in worktrees from locked Nix
sources or an unpacked source directory.

```sh
nix profile add github:awked-com/overlay
overlay -C /path/to/project setup hello
overlay -C /path/to/project new hello 0001-fix-greeting.patch src/hello.c
overlay -C /path/to/project edit hello src/hello.c
overlay -C /path/to/project refresh hello
```

## Project setup

Packages live at `pkgs/<name>/default.nix`. Recipes must attach their
`patches/NNNN-description.patch` files in filename order; do not maintain a
repository `series` file. `overlay` does not modify recipes.

Source lookup uses `packages.<system>.<name>`, falling back to the flake's
`nixpkgs` input with its default overlay. Git sources include tracked local edits,
exclude ignored worktrees, and leave the lockfile unchanged. Add new Nix files
to Git before setup. `setup PACKAGE PATH` uses unpacked, unpatched source without
Nix; `SOURCE_ROOT/PACKAGE` takes priority over flake lookup when PATH is omitted.

Keep worktrees out of version control and separate from package directories.
Existing directories need this tool's metadata before setup reuses them or
discard removes them. Refresh edits before leaving a Quilt shell and pop affected
patches before reordering filenames. After changing a source pin, preserve edits,
then discard and set up again. **Discard deletes unrefreshed worktree edits**;
repository patches are preserved.

## Develop

```sh
nix develop
go test -race ./...
go vet ./...
go build ./...
nix flake check
nix fmt
```

The flake check builds the command and runs Go tests. Native source tests use
synthetic inputs without downloads and need a local Nix store outside the sandbox:

```sh
nix develop -c env OVERLAY_NATIVE_NIX_TESTS=1 go test -run TestNativeNixSources -count=1 .
```
