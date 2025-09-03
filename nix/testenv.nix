{
  poetry2nix,
  lib,
  python311,
}:
poetry2nix.mkPoetryEnv {
  projectDir = ../tests/integration_tests;
  python = python311;
  overrides = poetry2nix.overrides.withDefaults (
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
    (
      lib.mapAttrs (
      attr: systems: 
      super.${attr}.overridePythonAttrs (old: {
          nativeBuildInputs = (old.nativeBuildInputs or [ ]) ++ map (a: self.${a}) systems;
        })
      ) buildSystems) // {
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
  );
}
