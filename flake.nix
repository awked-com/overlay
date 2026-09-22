{
  description = "Manage Nix package patch stacks with Quilt";
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  outputs =
    { self, nixpkgs }:
    let
      inherit (nixpkgs) lib;
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "aarch64-darwin"
      ];
      forAllSystems = lib.genAttrs systems;
      pkgsFor = system: import nixpkgs { inherit system; };
    in
    {
      overlays.default = final: _: { overlay = final.callPackage ./default.nix { }; };
      packages = forAllSystems (
        system:
        let
          package = (pkgsFor system).callPackage ./default.nix { };
        in
        {
          default = package;
          overlay = package;
        }
      );
      apps = forAllSystems (
        system:
        let
          app = {
            type = "app";
            program = "${self.packages.${system}.overlay}/bin/overlay";
            meta.description = "Manage Nix package patch stacks with Quilt";
          };
        in
        {
          default = app;
          overlay = app;
        }
      );
      checks = forAllSystems (system: {
        inherit (self.packages.${system}) overlay;
      });
      formatter = forAllSystems (system: (pkgsFor system).nixfmt);
      devShells = forAllSystems (
        system:
        let
          pkgs = pkgsFor system;
        in
        {
          default = pkgs.mkShell {
            inputsFrom = [ self.packages.${system}.overlay ];
            packages = [
              pkgs.go
              pkgs.quilt
              pkgs.nix
              pkgs.gnutar
              pkgs.gzip
              pkgs.xz
              pkgs.bzip2
              pkgs.nixfmt
              pkgs.actionlint
            ];
          };
        }
      );
    };
}
