{ mkProvider, pkgs }:

mkProvider {
  name = "ast-grep";
  runtimeInputs = [ pkgs.ast-grep ];
}
