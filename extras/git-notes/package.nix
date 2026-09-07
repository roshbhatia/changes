{ mkProvider, pkgs }:

mkProvider {
  name = "git-notes";
  runtimeInputs = [ pkgs.git ];
  includeInFull = false;
}
