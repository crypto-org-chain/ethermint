{
  lib,
  buildGoApplication,
  buildPackages,
  rev ? "dirty",
}:
let
  version = "v0.20.0-rc2";
  pname = "ethermintd";
  tags = [ "netgo" ];
  ldflags = lib.concatStringsSep "\n" ([
    "-X github.com/cosmos/cosmos-sdk/version.Name=ethermint"
    "-X github.com/cosmos/cosmos-sdk/version.AppName=${pname}"
    "-X github.com/cosmos/cosmos-sdk/version.Version=${version}"
    "-X github.com/cosmos/cosmos-sdk/version.BuildTags=${lib.concatStringsSep "," tags}"
    "-X github.com/cosmos/cosmos-sdk/version.Commit=${rev}"
  ]);
in
buildGoApplication rec {
  inherit
    pname
    version
    tags
    ldflags
    ;
  go = buildPackages.go_1_25;
  src = lib.sourceByRegex ./. [
    "^(x|ante|evmd|cmd|client|server|crypto|rpc|types|encoding|ethereum|indexer|testutil|version|store|internal|go.mod|go.sum|gomod2nix.toml)($|/.*)"
    "^tests(/.*[.]go)?$"
  ];
  modRoot = ".";
  modules = ./gomod2nix.toml;
  doCheck = false;
  pwd = src; # needed to support replace
  subPackages = [ "cmd/ethermintd" ];
  CGO_ENABLED = "1";
  GOTOOLCHAIN = "local";

  meta = with lib; {
    description = "Ethermint is a scalable and interoperable Ethereum library, built on Proof-of-Stake with fast-finality using the Cosmos SDK which runs on top of Tendermint Core consensus engine.";
    homepage = "https://github.com/evmos/ethermint";
    license = licenses.asl20;
    mainProgram = "ethermintd";
  };
}
