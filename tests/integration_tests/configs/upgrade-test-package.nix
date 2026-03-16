let
  pkgs = import ../../../nix { };
  fetchFlake =
    repo: rev:
    (pkgs.flake-compat {
      src = {
        outPath = builtins.fetchTarball "https://github.com/${repo}/archive/${rev}.tar.gz";
        inherit rev;
        shortRev = builtins.substring 0 7 rev;
      };
    }).defaultNix;
  released =
    (fetchFlake "crypto-org-chain/ethermint" "b216a320ac6a60b019c1cbe5a6b730856482f071").default;
  sdk50 =
    (fetchFlake "crypto-org-chain/ethermint" "9e97913655b02f9fef288e8f85c372115f75d0a3").default;
  current = pkgs.callPackage ../../../. { };
in
pkgs.linkFarm "upgrade-test-package" [
  {
    name = "genesis";
    path = released;
  }
  {
    name = "sdk50";
    path = sdk50;
  }
  {
    name = "sdk53";
    path = current;
  }
]
