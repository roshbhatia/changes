{
  description = "Symbol and call-aware diff viewer";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    systems.url = "github:nix-systems/default";
  };

  outputs =
    {
      self,
      nixpkgs,
      systems,
      ...
    }:
    let
      eachSystem = nixpkgs.lib.genAttrs (import systems);
    in
    {
      formatter = eachSystem (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        pkgs.writeShellApplication {
          name = "changes-format";
          runtimeInputs = [
            pkgs.fd
            pkgs.nixfmt
          ];
          text = ''
            if [ "$#" -gt 0 ] && [ "''${1#-}" = "$1" ]; then
              exec nixfmt "$@"
            fi
            exec fd --extension nix --type file --exec-batch nixfmt "$@"
          '';
        }
      );

      packages = eachSystem (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          version = "0.6.0";
          mkPackage =
            {
              name,
              subPackage,
              runtimeInputs ? [ ],
              completions ? false,
              providerManifest ? null,
              providerName ? name,
              builtName ? name,
            }:
            pkgs.buildGoModule {
              pname = name;
              inherit version;
              src = ./.;
              vendorHash = "sha256-BoQxSWsOzt3a9Z02unXcJUpl6k6C/Lgc61qSvy+vCCE=";
              subPackages = [ subPackage ];
              nativeBuildInputs = [ pkgs.makeWrapper ] ++ pkgs.lib.optional completions pkgs.installShellFiles;
              nativeCheckInputs = [
                pkgs.cue
                pkgs.git
              ];
              doCheck = completions;
              checkPhase = pkgs.lib.optionalString completions ''
                runHook preCheck
                go test -race ./...
                go run . generate --check
                for manifest in extras/*/provider.yaml; do
                  cue vet schema/provider.cue "$manifest" -d '#Provider'
                done
                runHook postCheck
              '';
              ldflags = pkgs.lib.optionals completions [
                "-s"
                "-w"
                "-X main.version=${version}"
              ];
              postInstall = ''
                ${pkgs.lib.optionalString (builtName != name) ''
                  mv "$out/bin/${builtName}" "$out/bin/${name}"
                ''}
                wrapProgram "$out/bin/${name}" \
                  --prefix PATH : ${pkgs.lib.makeBinPath runtimeInputs}
              ''
              + pkgs.lib.optionalString completions ''
                installShellCompletion \
                  --cmd changes \
                  --bash <("$out/bin/changes" completion bash) \
                  --fish <("$out/bin/changes" completion fish) \
                  --zsh <("$out/bin/changes" completion zsh)
                mkdir -p "$out/share/nushell/vendor/autoload"
                "$out/bin/changes" completion nu > "$out/share/nushell/vendor/autoload/changes.nu"
              ''
              + pkgs.lib.optionalString (providerManifest != null) ''
                mkdir -p "$out/share/changes/providers/${providerName}"
                install -m 0444 ${providerManifest} "$out/share/changes/providers/${providerName}/provider.yaml"
              '';
              meta = {
                description = "Composable Git change viewer component";
                homepage = "https://github.com/roshbhatia/changes";
                license = pkgs.lib.licenses.mit;
                mainProgram = name;
                platforms = pkgs.lib.platforms.unix;
              };
            };
          changes = mkPackage {
            name = "changes";
            subPackage = ".";
            runtimeInputs = [ pkgs.git ];
            completions = true;
          };
          astGrep = mkPackage {
            name = "changes-provider-ast-grep";
            subPackage = "./extras/ast-grep";
            runtimeInputs = [ pkgs.ast-grep ];
            providerManifest = ./extras/ast-grep/provider.yaml;
            providerName = "ast-grep";
            builtName = "ast-grep";
          };
          calldiff = mkPackage {
            name = "changes-provider-calldiff";
            subPackage = "./extras/calldiff";
            providerManifest = ./extras/calldiff/provider.yaml;
            providerName = "calldiff";
            builtName = "calldiff";
          };
          full = pkgs.symlinkJoin {
            name = "changes-full-${version}";
            paths = [
              changes
              astGrep
              calldiff
            ];
          };
        in
        {
          inherit changes full;
          provider-ast-grep = astGrep;
          provider-calldiff = calldiff;
          default = changes;
        }
      );

      apps = eachSystem (system: {
        default = {
          type = "app";
          program = "${nixpkgs.lib.getExe self.packages.${system}.default}";
        };
      });

      checks = eachSystem (system: {
        default = self.packages.${system}.default;
      });

      devShells = eachSystem (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          default = pkgs.mkShellNoCC {
            packages = [
              pkgs.go
              pkgs.gopls
              pkgs.gotools
              pkgs.go-tools
              pkgs.goreleaser
              pkgs.cue
              pkgs.ripgrep
              pkgs.shfmt
              pkgs.ast-grep
              pkgs.delta
              pkgs.diff-so-fancy
              pkgs.difftastic
              pkgs.git
              pkgs.fish
              pkgs.ffmpeg
              pkgs.tree-sitter
              pkgs.charm-freeze
              pkgs.vhs
            ];
            shellHook = ''
              export GOTOOLCHAIN=local
            '';
          };
        }
      );
    };
}
