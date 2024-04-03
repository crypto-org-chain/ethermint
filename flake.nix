{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    gomod2nix = {
      url = "github:nix-community/gomod2nix";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.flake-utils.follows = "flake-utils";
    };
  };

  outputs = { self, nixpkgs, gomod2nix, flake-utils }:
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
              (import ./nix/build_overlay.nix)
              gomod2nix.overlays.default
              self.overlay
            ];
            config = { };
          };
        in
        rec {
          packages.default = pkgs.callPackage ./. { inherit rev; };
          apps.default = mkApp packages.default;
          devShells.default = pkgs.mkShell {
            buildInputs = [
              packages.default.go
              pkgs.gomod2nix
            ];
          };
          legacyPackages = pkgs;
        }
      )
    ) // {
      overlay = _: super: {
        go = super.go_1_22;
      };
    };
}
