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
          version = "0.5.0";
          mkPackage =
            {
              name,
              subPackage,
              runtimeInputs ? [ ],
              completions ? false,
            }:
            pkgs.buildGoModule {
              pname = name;
              inherit version;
              src = ./.;
              vendorHash = "sha256-+eLh1JadOkKBjSnx06403XJtC7cZcxUf+kXT0vk56lA=";
              subPackages = [ subPackage ];
              nativeBuildInputs = [ pkgs.makeWrapper ] ++ pkgs.lib.optional completions pkgs.installShellFiles;
              nativeCheckInputs = [ pkgs.git ];
              doCheck = completions;
              checkPhase = pkgs.lib.optionalString completions ''
                runHook preCheck
                go test -race ./...
                go run . generate --check
                runHook postCheck
              '';
              ldflags = pkgs.lib.optionals completions [
                "-s"
                "-w"
                "-X main.version=${version}"
              ];
              postInstall = ''
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
            subPackage = "./extras/changes-provider-ast-grep";
            runtimeInputs = [ pkgs.ast-grep ];
          };
          calldiff = mkPackage {
            name = "changes-provider-calldiff";
            subPackage = "./extras/changes-provider-calldiff";
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
