{ mkProvider, pkgs }:

mkProvider {
  name = "codex-review";
  runtimeInputs = [ pkgs.codex ];
}
