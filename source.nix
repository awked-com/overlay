{
  flakeRef,
  package,
  system ? builtins.currentSystem,
}:
let
  # Git references exclude ignored worktrees and retain tracked local edits.
  flake = builtins.getFlake flakeRef;
  outputs = flake.packages.${system} or { };
  pkgs = import flake.inputs.nixpkgs {
    inherit system;
    overlays = if flake ? overlays.default then [ flake.overlays.default ] else [ ];
  };
  selected =
    if builtins.hasAttr package outputs then
      outputs.${package}
    else if flake ? inputs.nixpkgs then
      pkgs.${package}
    else
      throw "overlay: expose packages.${system}.${package} or a nixpkgs input";
  source = selected.src or (throw "overlay: package ${package} has no source");
in
# String context lets `nix build` realize either a fetcher derivation or a
# literal source path. Returning a bare path is not a buildable installable.
if builtins.isPath source then
  # Local directory names need not be valid Nix store names.
  "${builtins.path {
    path = source;
    name = "source";
  }}"
else
  "${source}"
