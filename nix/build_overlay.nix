# some basic overlays necessary for the build
final: super: {
  # For ethermintd only: 1.26.11 has security fixes the pinned nixpkgs (1.26.10) lacks.
  # Other Go tools keep the cached go_1_26 compiler, so only their own code is built.
  go_1_26_11 = super.go_1_26.overrideAttrs (_old: rec {
    version = "1.26.11";
    src = final.fetchurl {
      url = "https://go.dev/dl/go${version}.src.tar.gz";
      hash = "sha256-1EEBsP8n7SpMda1GwXucdDWSCeLrLKfJ32y0/baOGWk=";
    };
  });
}
