{
  poetry2nix,
  lib,
  python311,
}:
poetry2nix.mkPoetryEnv {
  projectDir = ../tests/integration_tests;
  python = python311;
  # Overrides are applied in list order, on top of poetry2nix's baseOverlay.
  # The rpds-py override MUST run *before* defaultPoetryOverrides: the default
  # override mangles rpds-py with overridePythonAttrs, which drops the
  # callPackage-level `.override` we need to flip `preferWheel`. Running first,
  # `super.rpds-py` is still the raw mkPoetryDep result.
  overrides = [
    # Force rpds-py to install from its prebuilt wheel instead of compiling the
    # Rust extension from the sdist. The source build runs a cargo vendor step
    # that fetches crates from crates.io's `/api/v1/crates/.../download`
    # endpoint, which returns HTTP 403 from CI runner IPs and breaks the build.
    # rpds-py ships cp311 wheels for manylinux x86_64 (CI) and macOS arm64,
    # so preferring the wheel sidesteps crates.io entirely. With a wheel src,
    # defaultPoetryOverrides' own rpds-py cargo handling becomes a no-op.
    (self: super: {
      rpds-py = super.rpds-py.override { preferWheel = true; };
    })
    poetry2nix.defaultPoetryOverrides
    (
      self: super:
      let
        buildSystems = {
          pystarport = [ "poetry-core" ];
          cprotobuf = [ "setuptools" ];
          durations = [ "setuptools" ];
          multitail2 = [ "setuptools" ];
          pytest-github-actions-annotate-failures = [ "setuptools" ];
          flake8-black = [ "setuptools" ];
          flake8-isort = [ "hatchling" ];
          pyunormalize = [ "setuptools" ];
          eth-bloom = [ "setuptools" ];
        };
      in
      (lib.mapAttrs (
        attr: systems:
        super.${attr}.overridePythonAttrs (old: {
          nativeBuildInputs = (old.nativeBuildInputs or [ ]) ++ map (a: self.${a}) systems;
        })
      ) buildSystems)
      // {
        # Fix malformed license field in types-requests package
        types-requests = super.types-requests.overridePythonAttrs (old: {
          postPatch = (old.postPatch or "") + ''
            # Fix malformed license field in pyproject.toml
            if [ -f pyproject.toml ]; then
              # Fix license field format
              sed -i 's/license = "Apache-2.0"/license = {text = "Apache-2.0"}/' pyproject.toml
              # Remove invalid license-files property from [project] section
              sed -i '/^license-files = /d' pyproject.toml
            fi
          '';
        });
      }
    )
  ];
}
