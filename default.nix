{
  lib,
  buildGoModule,
  makeWrapper,
  installShellFiles,
  quilt,
  nix,
  gnutar,
  gzip,
  xz,
  bzip2,
  coreutils,
}:

buildGoModule {
  pname = "overlay";
  version = "0.1.0";

  src = lib.fileset.toSource {
    root = ./.;
    fileset = lib.fileset.unions [
      ./go.mod
      ./go.sum
      ./source.nix
      ./cmd
      ./internal
      ./tests
      (lib.fileset.fileFilter (file: file.hasExt "go") ./.)
    ];
  };

  vendorHash = "sha256-0+YEKDc5jW/byxt3mutCh+EIvGm/sVaRoVRgnPBX5ag=";
  subPackages = [ "cmd/overlay" ];
  env.CGO_ENABLED = 0;
  ldflags = [
    "-s"
    "-w"
  ];
  nativeBuildInputs = [
    makeWrapper
    installShellFiles
  ];
  nativeCheckInputs = [ quilt ];
  checkPhase = ''
    runHook preCheck
    go test ./...
    runHook postCheck
  '';
  postInstall = ''
    installShellCompletion --cmd overlay \
      --bash <($out/bin/overlay completion bash) \
      --zsh <($out/bin/overlay completion zsh) \
      --fish <($out/bin/overlay completion fish)
    wrapProgram $out/bin/overlay --prefix PATH : ${
      lib.makeBinPath [
        quilt
        nix
        gnutar
        gzip
        xz
        bzip2
        coreutils
      ]
    }
  '';

  meta = {
    description = "Manage Nix package patch stacks with Quilt";
    platforms = [
      "x86_64-linux"
      "aarch64-linux"
      "aarch64-darwin"
    ];
    mainProgram = "overlay";
  };
}
