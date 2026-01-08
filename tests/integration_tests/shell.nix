{
  system ? builtins.currentSystem,
  pkgs ? import ../../nix { inherit system; },
}:
pkgs.mkShell {
  buildInputs = [
    pkgs.jq
    (pkgs.callPackage ../../. { }) # ethermintd
    pkgs.start-scripts
    pkgs.go-ethereum
    pkgs.cosmovisor
    pkgs.poetry
    pkgs.nodejs
    pkgs.test-env
  ];
  shellHook = ''
    . ${../../scripts/env}
    # Fix poetry2nix Python environment in nixpkgs 25.11
    # The wrapper binary doesn't set PYTHONHOME, causing sys.prefix to point
    # to the base python instead of the poetry environment
    export PYTHONHOME="${pkgs.test-env}"
  '';
}
