# overlay

Maintain numbered package patches with Quilt, using worktrees from locked Nix
sources or an unpacked source directory.

```sh
nix profile add github:awked-com/overlay
overlay -C /path/to/project setup hello
overlay -C /path/to/project new hello 0001-fix-greeting.patch src/hello.c
overlay -C /path/to/project edit hello src/hello.c
overlay -C /path/to/project refresh hello
```

Run `overlay help` for commands and options.

## Project setup

Packages live at `pkgs/<name>/default.nix`, with patches in
`pkgs/<name>/patches/NNNN-description.patch`. Filenames define application order;
do not maintain a repository `series` file. Recipes must attach their patches to
the derivation; `overlay` does not modify recipes.

Without `-C`, the nearest parent flake or Git repository is selected. Source
lookup uses `packages.<system>.<name>`, falling back to the flake's `nixpkgs` input
with its default overlay. Git sources include tracked local edits, exclude
ignored worktrees, and leave the lockfile unchanged. Add new Nix files to Git
before setup.

`setup PACKAGE PATH` takes an unpacked, unpatched source directory without Nix.
If PATH is omitted, `SOURCE_ROOT/PACKAGE` takes priority over flake lookup.

Worktrees default to `.patch-worktrees/pkgs`; use `--worktrees` or
`PATCH_WORKTREES` to override. Keep them out of version control and separate from
package directories. Existing directories need this tool's metadata before
setup reuses them or discard removes them.

Refresh edits before leaving a Quilt shell. Pop affected patches before
reordering filenames. After changing a source pin, preserve edits, then discard
and set up the worktree again. **Discard deletes unrefreshed worktree edits** and
preserves repository patches.

## Develop

```sh
nix develop
go test -race ./...
go vet ./...
go build ./...
nix flake check
nix fmt
```

The flake check builds the command and runs Go tests. Native source tests require
a local Nix store outside the build sandbox; their synthetic inputs need no
downloads:

```sh
nix develop -c env OVERLAY_NATIVE_NIX_TESTS=1 go test -run TestNativeNixSources -count=1 .
```
