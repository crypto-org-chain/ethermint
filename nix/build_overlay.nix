# some basic overlays necessary for the build
final: super: {
  # Separate from go_1_26 so other Go tools keep the cached compiler; overriding go_1_26 rebuilds them all.
  # Remove once the pinned nixpkgs ships Go >= 1.26.11 (fixes GO-2026-6859).
  go_1_26_11 = super.go_1_26.overrideAttrs (_old: rec {
    version = "1.26.11";
    src = final.fetchurl {
      url = "https://go.dev/dl/go${version}.src.tar.gz";
      hash = "sha256-1EEBsP8n7SpMda1GwXucdDWSCeLrLKfJ32y0/baOGWk=";
    };
  });
}
