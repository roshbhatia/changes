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
      supportedSystems = builtins.filter (system: system != "x86_64-darwin") (import systems);
      eachSystem = nixpkgs.lib.genAttrs supportedSystems;
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
                platforms = pkgs.lib.platforms.darwin ++ pkgs.lib.platforms.linux;
              };
              passthru.runtimeInputs = runtimeInputs;
            };
          changes = mkPackage {
            name = "changes";
            subPackage = ".";
            runtimeInputs = [ pkgs.git ];
            completions = true;
          };
          mkProvider =
            {
              name,
              runtimeInputs ? [ ],
            }:
            let
              adapter = mkPackage {
                name = "changes-provider-${name}";
                subPackage = "./extras/${name}";
                inherit runtimeInputs;
                providerManifest = ./extras/${name}/provider.yaml;
                providerName = name;
                builtName = name;
              };
            in
            pkgs.symlinkJoin {
              name = "changes-provider-${name}-${version}";
              paths = [ adapter ] ++ runtimeInputs;
              passthru.providerRuntimeInputs = runtimeInputs;
              meta = adapter.meta;
            };
          providerNames = builtins.filter (name: builtins.pathExists (./extras + "/${name}/package.nix")) (
            builtins.attrNames (builtins.readDir ./extras)
          );
          providers = builtins.listToAttrs (
            map (name: {
              inherit name;
              value = import (./extras + "/${name}/package.nix") { inherit mkProvider pkgs; };
            }) providerNames
          );
          full = pkgs.symlinkJoin {
            name = "changes-full-${version}";
            paths = [ changes ] ++ builtins.attrValues providers;
            nativeBuildInputs = [ pkgs.makeWrapper ];
            postBuild = ''
              wrapProgram "$out/bin/changes" \
                --prefix XDG_DATA_DIRS : "$out/share"
            '';
            meta = changes.meta // {
              mainProgram = "changes";
            };
          };
        in
        {
          inherit changes full;
          default = changes;
        }
        // builtins.listToAttrs (
          map (name: {
            name = "provider-${name}";
            value = providers.${name};
          }) providerNames
        )
      );

      apps = eachSystem (system: {
        default = {
          type = "app";
          program = "${nixpkgs.lib.getExe self.packages.${system}.default}";
        };
      });

      checks = eachSystem (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          packages = self.packages.${system};
          providerNames = builtins.filter (name: builtins.pathExists (./extras + "/${name}/package.nix")) (
            builtins.attrNames (builtins.readDir ./extras)
          );
          providerChecks = builtins.listToAttrs (
            map (name: {
              name = "provider-${name}";
              value = pkgs.runCommand "changes-provider-${name}-check" { } ''
                export HOME="$TMPDIR/home"
                export XDG_CACHE_HOME="$TMPDIR/cache"
                export XDG_CONFIG_HOME="$TMPDIR/config"
                export XDG_DATA_HOME="$TMPDIR/data-home"
                export XDG_DATA_DIRS="${packages."provider-${name}"}/share"
                unset CHANGES_CONFIG CHANGES_PROVIDERS_DIRECTORY
                export PATH="${
                  pkgs.lib.makeBinPath [
                    packages.default
                    packages."provider-${name}"
                    pkgs.coreutils
                  ]
                }"
                mkdir -p "$HOME" "$XDG_CACHE_HOME" "$XDG_CONFIG_HOME" "$XDG_DATA_HOME"
                ${pkgs.lib.getExe packages.default} provider validate ${name}
                touch "$out"
              '';
            }) providerNames
          );
          providerRuntimeInputs = pkgs.lib.unique (
            pkgs.lib.concatMap (name: packages."provider-${name}".providerRuntimeInputs) providerNames
          );
          coreRuntimePaths = map toString packages.default.runtimeInputs;
          providerExclusiveRuntimeInputs = builtins.filter (
            runtime: !(builtins.elem (toString runtime) coreRuntimePaths)
          ) providerRuntimeInputs;
        in
        {
          default = packages.default;
          provider-boundary =
            pkgs.runCommand "changes-provider-boundary"
              {
                nativeBuildInputs = [ pkgs.gnugrep ];
              }
              ''
                ${pkgs.bash}/bin/bash ${./hack/audit-provider-boundary.sh} ${./.}
                touch "$out"
              '';
          provider-manifests =
            pkgs.runCommand "changes-provider-manifests"
              {
                nativeBuildInputs = [ pkgs.cue ];
              }
              ''
                for manifest in ${./.}/extras/*/provider.yaml; do
                  cue vet ${./.}/schema/provider.cue "$manifest" -d '#Provider'
                done
                touch "$out"
              '';
          zero-provider-closure =
            pkgs.runCommand "changes-zero-provider-closure"
              {
                closure = pkgs.closureInfo { rootPaths = [ packages.default ]; };
                nativeBuildInputs = [ pkgs.gnugrep ];
              }
              ''
                for runtime in ${builtins.concatStringsSep " " (map toString providerExclusiveRuntimeInputs)}; do
                  if grep -Fxq "$runtime" "$closure/store-paths"; then
                    echo "default package closure contains provider runtime $runtime" >&2
                    exit 1
                  fi
                done
                export HOME="$TMPDIR/home"
                export XDG_CACHE_HOME="$TMPDIR/cache"
                export XDG_CONFIG_HOME="$TMPDIR/config"
                export XDG_DATA_HOME="$TMPDIR/data-home"
                export XDG_DATA_DIRS="$TMPDIR/data"
                unset CHANGES_CONFIG CHANGES_PROVIDERS_DIRECTORY
                mkdir -p "$HOME" "$XDG_CACHE_HOME" "$XDG_CONFIG_HOME" "$XDG_DATA_HOME" "$XDG_DATA_DIRS"
                test "$(${pkgs.lib.getExe packages.default} provider list)" = "No analysis providers are configured."
                touch "$out"
              '';
          full-provider-layout =
            pkgs.runCommand "changes-full-provider-layout"
              {
                nativeBuildInputs = [ pkgs.jq ];
              }
              ''
                export HOME="$TMPDIR/home"
                export XDG_CACHE_HOME="$TMPDIR/cache"
                export XDG_CONFIG_HOME="$TMPDIR/config"
                export XDG_DATA_HOME="$TMPDIR/data-home"
                export XDG_DATA_DIRS="$TMPDIR/data"
                unset CHANGES_CONFIG CHANGES_PROVIDERS_DIRECTORY
                mkdir -p "$HOME" "$XDG_CACHE_HOME" "$XDG_CONFIG_HOME" "$XDG_DATA_HOME" "$XDG_DATA_DIRS"
                test "$(${pkgs.lib.getExe packages.full} provider list --json | jq 'length')" -eq ${toString (builtins.length providerNames)}
                touch "$out"
              '';
        }
        // providerChecks
      );

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
              pkgs.git
              pkgs.fish
              pkgs.ffmpeg
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
