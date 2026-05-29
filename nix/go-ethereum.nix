{
  lib,
  stdenv,
  buildGoModule,
  fetchFromGitHub,
  libobjc,
  IOKit,
  nixosTests,
}:

let
  # A list of binaries to put into separate outputs
  bins = [
    "geth"
    "clef"
  ];

in
buildGoModule rec {
  pname = "go-ethereum";
  # Use the old estimateGas implementation
  # https://github.com/crypto-org-chain/go-ethereum/commits/release/1.16-estimateGas/
  version = "84de8af25c3e0915a69838b0ecb8e683d3b6ea53";

  src = fetchFromGitHub {
    owner = "crypto-org-chain";
    repo = pname;
    rev = version;
    sha256 = "sha256-Zo3AnEDSu0qjBbftEkUEedrPwDIWf0P6gzLIWSil7gQ=";
  };

  proxyVendor = true;
  vendorHash = "sha256-KP9oD87kn8MCvEf3ply8HbP8xIBlGAEtthGob8Yh++A=";

  doCheck = false;

  outputs = [ "out" ] ++ bins;

  # Move binaries to separate outputs and symlink them back to $out
  postInstall = lib.concatStringsSep "\n" (
    builtins.map (
      bin:
      "mkdir -p \$${bin}/bin && mv $out/bin/${bin} \$${bin}/bin/ && ln -s \$${bin}/bin/${bin} $out/bin/"
    ) bins
  );

  subPackages = [
    "cmd/abidump"
    "cmd/abigen"
    "cmd/blsync"
    "cmd/clef"
    "cmd/devp2p"
    "cmd/era"
    "cmd/ethkey"
    "cmd/evm"
    "cmd/geth"
    "cmd/rlpdump"
    "cmd/utils"
  ];

  # Following upstream: https://github.com/ethereum/go-ethereum/blob/v1.10.25/build/ci.go#L218
  tags = [ "urfave_cli_no_docs" ];

  # Fix for usb-related segmentation faults on darwin
  propagatedBuildInputs = lib.optionals (stdenv.isDarwin && libobjc != null && IOKit != null) [
    libobjc
    IOKit
  ];

  # Add missing dependencies for HID support on Darwin
  buildInputs = lib.optionals (stdenv.isDarwin && libobjc != null && IOKit != null) [
    libobjc
    IOKit
  ];

  passthru.tests = { inherit (nixosTests) geth; };

  meta = with lib; {
    homepage = "https://geth.ethereum.org/";
    description = "Official golang implementation of the Ethereum protocol";
    license = with licenses; [
      lgpl3Plus
      gpl3Plus
    ];
    maintainers = with maintainers; [
      adisbladis
      RaghavSood
    ];
  };
}
