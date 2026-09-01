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
          changes = pkgs.buildGoModule {
            pname = "changes";
            version = "0.2.0";
            src = ./.;
            vendorHash = "sha256-dS2stk2YJHM5MFS/7DskZMLeH7mOKWd8e5MWRIwI084=";
            nativeBuildInputs = [
              pkgs.installShellFiles
              pkgs.makeWrapper
            ];
            nativeCheckInputs = [ pkgs.git ];
            postInstall = ''
              wrapProgram "$out/bin/changes" \
                --prefix PATH : ${
                  pkgs.lib.makeBinPath [
                    pkgs.ast-grep
                    pkgs.delta
                    pkgs.diff-so-fancy
                    pkgs.difftastic
                    pkgs.git
                    pkgs.ripgrep
                    pkgs.tree-sitter
                  ]
                }
              installShellCompletion \
                --cmd changes \
                --bash <("$out/bin/changes" completion bash) \
                --fish <("$out/bin/changes" completion fish) \
                --zsh <("$out/bin/changes" completion zsh)
              mkdir -p "$out/share/nushell/vendor/autoload"
              "$out/bin/changes" completion nu > "$out/share/nushell/vendor/autoload/changes.nu"
            '';
            meta = {
              description = "Read Git changes as a symbol tree with call edges";
              homepage = "https://github.com/roshbhatia/changes";
              license = pkgs.lib.licenses.mit;
              mainProgram = "changes";
              platforms = pkgs.lib.platforms.unix;
            };
          };
        in
        {
          inherit changes;
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
            ];
            shellHook = ''
              export GOTOOLCHAIN=local
            '';
          };
        }
      );
    };
}
