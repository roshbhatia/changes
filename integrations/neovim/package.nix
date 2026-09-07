{ pkgs, version }:

pkgs.vimUtils.buildVimPlugin {
  pname = "changes.nvim";
  inherit version;
  src = ./.;
}
