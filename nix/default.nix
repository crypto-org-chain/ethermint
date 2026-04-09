{
  sources ? import ./sources.nix,
  system ? builtins.currentSystem,
  ...
}:

import sources.nixpkgs {
  overlays = [
    (import ./build_overlay.nix)
    (final: super: {
      flake-compat = import sources.flake-compat;
      # In nixpkgs 25.11 with go = go_1_25 (set in build_overlay.nix),
      # buildGoModule already uses Go 1.25, so we just use it directly
      go-ethereum = final.callPackage ./go-ethereum.nix {
        # Skip darwin-specific dependencies to avoid apple_sdk_11_0 errors in nixpkgs 25.11
        libobjc = null;
        IOKit = null;
      };
      golangci-lint = final.callPackage ./golangci-lint.nix { };
    }) # update to a version that supports eip-1559
    (import "${sources.poetry2nix}/overlay.nix")
    # Fix poetry2nix compatibility with nixpkgs 25.11 - override fetchCargoTarball usage
    (final: prev: {
      poetry2nix = prev.poetry2nix.overrideScope (
        p2nFinal: p2nPrev: {
          defaultPoetryOverrides = p2nPrev.defaultPoetryOverrides.extend (
            pyFinal: pyPrev: {
              # Override rpds-py to use fetchCargoVendor instead of fetchCargoTarball
              rpds-py = pyPrev.rpds-py.overridePythonAttrs (
                old:
                if old.src.isWheel or false then
                  { }
                else
                  {
                    cargoDeps = final.rustPlatform.fetchCargoVendor {
                      inherit (old) src;
                      name = "${old.pname}-${old.version}-cargo-vendor.tar.gz";
                      hash = "sha256-npvJz6PMHWzPkI0LVNeiMsZVxmwR6uzjlhBPMCCrFfw=";
                    };
                  }
              );
            }
          );
        }
      );
    })
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
      cosmovisor = pkgs.buildGoModule rec {
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
