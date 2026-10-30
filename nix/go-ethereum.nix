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
  ];

in
buildGoModule rec {
  pname = "go-ethereum";
  # Use the old estimateGas implementation
  # https://github.com/crypto-org-chain/go-ethereum/commits/release/1.17-estimateGas/
  version = "8b02a630b22c9a4a9e1a243c59b5ce9220f443f8";

  src = fetchFromGitHub {
    owner = "crypto-org-chain";
    repo = pname;
    rev = version;
    sha256 = "sha256-IWYf6GM0CKqZJBsTgMJo3cIRsPjn6FT9iMftsDhyLBw=";
  };

  proxyVendor = true;
  vendorHash = "sha256-4Xi/33CwGfiikbv0yxcpM36VFGqcRLWXZjl6k0ILhRg=";

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
