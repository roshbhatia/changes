{ mkProvider, pkgs }:

let
  keepPrebuild =
    {
      aarch64-darwin = "darwin-arm64";
      x86_64-darwin = "darwin-x64";
      aarch64-linux = "linux-arm64";
      x86_64-linux = "linux-x64";
    }
    .${pkgs.stdenv.hostPlatform.system} or null;
  runtime = pkgs.buildNpmPackage {
    pname = "calldiff";
    version = "0.5.0";

    src = ./runtime;
    npmDeps = pkgs.fetchNpmDeps {
      name = "calldiff-npm-deps";
      src = ./runtime;
      hash = "sha256-7TJmIoccEd16xskB5fOveIzjZi0Srwad8TrwBhfc0Dc=";
    };
    npmFlags = [ "--legacy-peer-deps" ];
    dontNpmBuild = true;

    nativeBuildInputs = [
      pkgs.makeWrapper
    ]
    ++ pkgs.lib.optional pkgs.stdenv.hostPlatform.isLinux pkgs.autoPatchelfHook;
    buildInputs = pkgs.lib.optional pkgs.stdenv.hostPlatform.isLinux pkgs.stdenv.cc.cc.lib;

    installPhase = ''
      runHook preInstall

      mkdir -p "$out/lib/calldiff"
      cp -r node_modules package.json "$out/lib/calldiff/"
      substituteInPlace "$out/lib/calldiff/node_modules/calldiff/dist/languages/bash.js" \
        --replace-fail '[".sh", ".bash"]' '[".sh", ".bash", ".zsh"]'
    ''
    + pkgs.lib.optionalString (keepPrebuild != null) ''
      find "$out/lib/calldiff/node_modules" -type d -path '*/prebuilds/*' \
        -not -name ${keepPrebuild} -maxdepth 4 -exec rm -rf {} +
    ''
    + ''
      makeWrapper ${pkgs.nodejs}/bin/node "$out/bin/calldiff" \
        --add-flags "$out/lib/calldiff/node_modules/calldiff/dist/cli.js"

      runHook postInstall
    '';
  };
in
mkProvider {
  name = "calldiff";
  runtimeInputs = [
    runtime
    pkgs.git
  ];
}
