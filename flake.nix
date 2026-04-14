{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/54170c54449ea4d6725efd30d719c5e505f1c10e";
    flake-utils.url = "github:numtide/flake-utils";
    poetry2nix = {
      url = "github:nix-community/poetry2nix";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.flake-utils.follows = "flake-utils";
    };
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
      poetry2nix,
    }:
    let
      rev = self.shortRev or "dirty";
      mkApp = drv: {
        type = "app";
        program = "${drv}/bin/${drv.meta.mainProgram}";
      };
    in
    (flake-utils.lib.eachDefaultSystem (
      system:
      let
        # Import niv sources to maintain single source of truth for dependencies
        sources = import ./nix/sources.nix;

        # Custom gomod2nix overlay that avoids darwin.apple_sdk_11_0 reference
        # Uses the same gomod2nix version as niv to prevent drift between flake and niv builds
        gomodOverlay =
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
          };

        pkgs = import nixpkgs {
          inherit system;
          overlays = [
            gomodOverlay
            poetry2nix.overlays.default
          ]
          ++ self.overlays.default;
        };
      in
      rec {
        packages.default = pkgs.callPackage ./. { inherit rev; };
        apps.default = mkApp packages.default;
        devShells = {
          default = pkgs.mkShell {
            buildInputs = [
              packages.default.go
              pkgs.gomod2nix
            ];
          };
          full = pkgs.mkShell {
            buildInputs = [
              packages.default.go
              pkgs.gomod2nix
              pkgs.test-env
            ];
          };
        };
        legacyPackages = pkgs;
      }
    ))
    // {
      overlays.default = [
        (import ./nix/build_overlay.nix)
        (final: super: {
          test-env = final.callPackage ./nix/testenv.nix { };
        })
      ];
    };
}
