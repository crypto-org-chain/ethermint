# some basic overlays necessary for the build
final: super: {
  # cosmos-sdk v0.55 requires go >= 1.26.10; nixpkgs 26.05 ships 1.26.4.
  go_1_26 = super.go_1_26.overrideAttrs (_old: rec {
    version = "1.26.11";
    src = final.fetchurl {
      url = "https://go.dev/dl/go${version}.src.tar.gz";
      hash = "sha256-1EEBsP8n7SpMda1GwXucdDWSCeLrLKfJ32y0/baOGWk=";
    };
  });
}
