# some basic overlays necessary for the build
final: super:
let
  hasInfix = final.lib.strings.hasInfix;
in
{
  go_1_25 = super.go_1_23.overrideAttrs (old: rec {
    version = "1.25.0";
    src = final.fetchurl {
      url = "https://go.dev/dl/go${version}.src.tar.gz";
      hash = "sha256-S9AekSlyB7+kUOpA1NWpOxtTGl5DhHOyoG4Y4HciciU=";
    };

    patches =
      let
        filtered =
          builtins.filter
            (
              patch:
              let str = builtins.toString patch;
              in !(hasInfix "iana-etc" str || hasInfix "go_no_vendor_checks" str)
            )
            old.patches;
        ianaPath = "${final."iana-etc"}";
        ianaProtocolsPatch = final.writeText "go-use-iana-protocols.patch"
          (builtins.replaceStrings
            [ "@IANA_ETC@" ]
            [ ianaPath ]
            ''
--- a/src/net/lookup_unix.go
+++ b/src/net/lookup_unix.go
@@ -15,7 +15,7 @@
 // readProtocolsOnce loads contents of /etc/protocols into protocols map
 // for quick access.
 var readProtocolsOnce = sync.OnceFunc(func() {
-	file, err := open("/etc/protocols")
+	file, err := open("@IANA_ETC@/etc/protocols")
 	if err != nil {
 		return
 	}
            '');
        ianaServicesPatch = final.writeText "go-use-iana-services.patch"
          (builtins.replaceStrings
            [ "@IANA_ETC@" ]
            [ ianaPath ]
            ''
--- a/src/net/port_unix.go
+++ b/src/net/port_unix.go
@@ -16,7 +16,7 @@
 var onceReadServices sync.Once

 func readServices() {
-	file, err := open("/etc/services")
+	file, err := open("@IANA_ETC@/etc/services")
 	if err != nil {
 		return
 	}
            '');
      in
      filtered ++ [
        ianaProtocolsPatch
        ianaServicesPatch
        # https://github.com/NixOS/nixpkgs/pull/372367
        ./go_no_vendor_checks-1.23.patch
      ];

    # Use go_1_23 as bootstrap since it should have a compatible version
    # If this still fails with bootstrap issues, you may need to update nixpkgs
    goBootstrap = if (builtins.compareVersions super.go_1_23.version "1.22.6") >= 0
                  then super.go_1_23
                  else builtins.throw "Go 1.25.0 requires Go 1.22.6 or later as bootstrap, but got ${super.go_1_23.version}";
  });

  buildGo125Module = super.buildGoModule.override {
    go = final.go_1_25;
  };
}
