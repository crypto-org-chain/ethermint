{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/release-25.05";
    flake-utils.url = "github:numtide/flake-utils";
    gomod2nix = {
      url = "github:obreitwi/gomod2nix/fix/go_mod_vendor";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.flake-utils.follows = "flake-utils";
    };
    poetry2nix = {
      url = "github:nix-community/poetry2nix";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.flake-utils.follows = "flake-utils";
    };
  };

  outputs = { self, nixpkgs, gomod2nix, flake-utils, poetry2nix }:
    let
      rev = self.shortRev or "dirty";
      mkApp = drv: {
        type = "app";
        program = "${drv}/bin/${drv.meta.mainProgram}";
      };
    in
    (flake-utils.lib.eachDefaultSystem
      (system:
        let
          pkgs = import nixpkgs {
            inherit system;
            overlays = [
              gomod2nix.overlays.default
              poetry2nix.overlays.default
              self.overlay
            ];
            config = { };
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
      )
    ) // {
      overlays.default = [
        (import ./nix/build_overlay.nix)
        (final: super: {
          test-env = final.callPackage ./nix/testenv.nix { };
        })
      ];
    };
}
