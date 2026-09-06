{ mkProvider, pkgs }:

mkProvider {
  name = "github-pr";
  runtimeInputs = [
    pkgs.gh
    pkgs.git
  ];
}
