{
  sources ? import ./sources.nix,
  system ? builtins.currentSystem,
  ...
}:

import sources.nixpkgs {
  overlays = [
    (import ./build_overlay.nix)
    (_: pkgs: {
      flake-compat = import sources.flake-compat;
      buildGo125Module = pkgs.buildGoModule.override { go = pkgs.go_1_25; };
      go-ethereum = pkgs.callPackage ./go-ethereum.nix {
        inherit (pkgs.darwin) libobjc;
        inherit (pkgs.darwin.apple_sdk.frameworks) IOKit;
        buildGoModule = pkgs.buildGo125Module or (pkgs.buildGoModule.override { go = pkgs.go_1_25; });
      };
      golangci-lint = pkgs.callPackage ./golangci-lint.nix {
        buildGo125Module = pkgs.buildGo125Module or (pkgs.buildGoModule.override { go = pkgs.go_1_25; });
      };
    }) # update to a version that supports eip-1559
    (import "${sources.poetry2nix}/overlay.nix")
    # Custom gomod2nix overlay that avoids darwin.apple_sdk_11_0 reference
    (
      final: prev:
      let
        gomodSrc = sources.gomod2nix;
        callPackage = final.callPackage;
        gomodBuilder = callPackage "${gomodSrc}/builder" { };
      in
      {
        inherit (gomodBuilder) buildGoApplication mkGoEnv mkVendorEnv;
        gomod2nix = (callPackage "${gomodSrc}/default.nix" { }).overrideAttrs (_: {
          modRoot = ".";
        });
      }
    )
    (
      pkgs: _:
      import ./scripts.nix {
        inherit pkgs;
        config = {
          ethermint-config = ../scripts/ethermint-devnet.yaml;
          geth-genesis = ../scripts/geth-genesis.json;
          dotenv = builtins.path {
            name = "dotenv";
            path = ../scripts/env;
          };
        };
      }
    )
    (_: pkgs: { test-env = pkgs.callPackage ./testenv.nix { }; })
    (_: pkgs: {
      cosmovisor = (pkgs.buildGo125Module or (pkgs.buildGoModule.override { go = pkgs.go_1_25; })) rec {
        name = "cosmovisor";
        src = sources.cosmos-sdk + "/cosmovisor";
        subPackages = [ "./cmd/cosmovisor" ];
        vendorHash = "sha256-OAXWrwpartjgSP7oeNvDJ7cTR9lyYVNhEM8HUnv3acE=";
        doCheck = false;
      };
    })
  ];
  config = { };
  inherit system;
}
