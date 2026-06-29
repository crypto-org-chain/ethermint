Schema-level test for Ethermint JSON-RPC against ethereum/execution-apis fixtures.

The pytest fixture syncs `.io` fixtures from ethereum/execution-apis before the
test run. The copied fixture directories and upstream geth chain files are
generated under this `execution-api/` directory and ignored by git.
By default the sync downloads the latest `main` archive from
`ethereum/execution-apis`.

Running the test

```sh
nix-shell ./tests/integration_tests/shell.nix --run \
    "pytest -vv --basetemp=/tmp/eth -s -k test_rpc_spec_schema \
    --session-timeout=6000 --timeout=6000"
```

Reading the report after running the test

```sh
/tmp/eth/ethermintcurrent/rpc_schema_report.md
```
