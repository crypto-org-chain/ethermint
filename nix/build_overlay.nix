# some basic overlays necessary for the build
final: super:
{
  # nixpkgs 25.11 has go_1_25
  go = super.go_1_25;
}
