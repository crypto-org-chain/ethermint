{
  sources ? import ./sources.nix,
  system ? builtins.currentSystem,
  ...
}:

let
  # Bootstrap nixpkgs (without overlays) just to get applyPatches.
  # Used to patch poetry2nix's vendored pep599.nix to add the missing
  # riscv64 entry (some Python packages now ship riscv64 wheels, which
  # this older poetry2nix doesn't know how to evaluate).
  bootstrapPkgs = import sources.nixpkgs { inherit system; };
  patchedPoetry2nix = bootstrapPkgs.applyPatches {
    name = "poetry2nix-with-riscv64";
    src = sources.poetry2nix;
    postPatch = ''
      substituteInPlace vendor/pyproject.nix/lib/pep599.nix \
        --replace 'manyLinuxTargetMachines = {' \
                  'manyLinuxTargetMachines = { riscv64 = "riscv64";'
    '';
  };
  # Patch gomod2nix's symlink builder to handle split-module monorepos where
  # a root module and its sub-modules share a vendor path prefix.
  # E.g. github.com/aws/aws-sdk-go-v2 (root) + aws-sdk-go-v2/config (sub-module)
  # both need entries under vendor/github.com/aws/aws-sdk-go-v2/.
  #
  # The naive "symlink whole module dir" approach fails because sub-module
  # MkdirAll creates the parent as a real directory before the root module
  # can claim it as a symlink.  The original skip-if-exists patch fixed the
  # "file exists" panic but silently dropped root module content.
  #
  # This patch appends symlinkOrMerge directly into symlink.go (the only file
  # compiled by gomod2nix-symlink.drv) and replaces os.Symlink with it:
  #   - dst missing       → create symlink as usual
  #   - dst is a symlink  → skip (already claimed by another module)
  #   - dst is a dir      → recurse and symlink each top-level src entry
  #
  # Upstream issue is unfixed as of nix-community/gomod2nix@514283ec.
  gomod2nixMergeFunc = bootstrapPkgs.writeText "symlink-merge-func" ''

    func symlinkOrMerge(src, dst string) error {
        dstInfo, dstErr := os.Lstat(dst)
        if dstErr != nil {
            return os.Symlink(src, dst)
        }
        if dstInfo.Mode()&os.ModeSymlink != 0 {
            return nil
        }
        srcInfo, srcErr := os.Lstat(src)
        if srcErr != nil || !srcInfo.IsDir() {
            return nil
        }
        entries, err := os.ReadDir(src)
        if err != nil {
            return err
        }
        for _, entry := range entries {
            if err := symlinkOrMerge(
                src+"/"+entry.Name(),
                dst+"/"+entry.Name(),
            ); err != nil {
                return err
            }
        }
        return nil
    }
  '';
  patchedGomod2nix = bootstrapPkgs.applyPatches {
    name = "gomod2nix-symlink-merge";
    src = sources.gomod2nix;
    postPatch = ''
      cat ${gomod2nixMergeFunc} >> builder/symlink/symlink.go
      substituteInPlace builder/symlink/symlink.go \
        --replace-fail \
        $'\t\tif err := os.Symlink(innerSrc, dst); err != nil {\n' \
        $'\t\tif err := symlinkOrMerge(innerSrc, dst); err != nil {\n'
    '';
  };
in
import sources.nixpkgs {
  overlays = [
    (import ./build_overlay.nix)
    (final: super: {
      flake-compat = import sources.flake-compat;
      # nixpkgs 25.11 already aliases go = go_1_25 (1.25.9) and buildGoModule = buildGo125Module
      go-ethereum = final.callPackage ./go-ethereum.nix {
        # Skip darwin-specific dependencies to avoid apple_sdk_11_0 errors in nixpkgs 25.11
        libobjc = null;
        IOKit = null;
      };
      golangci-lint = final.callPackage ./golangci-lint.nix { };
    }) # update to a version that supports eip-1559
    (import "${patchedPoetry2nix}/overlay.nix")
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
        gomodSrc = patchedGomod2nix;
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
