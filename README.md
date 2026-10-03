# overlay

Maintain numbered package patches with Quilt in worktrees.

```sh
nix profile add github:awked-com/overlay
overlay -C /path/to/project setup hello
```

## Sources and worktrees

Packages live at `pkgs/<name>/default.nix`. Recipes must attach
`patches/NNNN-description.patch` files in filename order; `overlay` does not
edit recipes or use a repository `series` file.

`setup PACKAGE PATH` accepts unpacked, unpatched source without Nix. Otherwise,
`SOURCE_ROOT/PACKAGE` takes priority over `packages.<system>.<name>` and the
flake's `nixpkgs` input. Git sources include tracked edits and leave the lockfile
unchanged; add new Nix files to Git before setup.

Keep worktrees outside package directories and version control. Setup and discard
only accept worktrees with overlay metadata. Refresh edits before leaving a Quilt
shell; pop affected patches before renaming them. After changing a source pin,
refresh edits, discard, and set up again. **Discard deletes unrefreshed edits**;
repository patches remain.

## Develop

```sh
nix develop
go test -race ./...
go vet ./...
go build ./...
nix flake check
nix fmt
```

After a Go dependency update, set `vendorHash` in `default.nix` to
`lib.fakeHash`, run `nix build .#overlay`, replace it with the reported `got:`
hash, then run `nix flake check`.
