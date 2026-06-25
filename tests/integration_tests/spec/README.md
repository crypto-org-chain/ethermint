Schema-level test for Ethermint JSON-RPC against execution-apis fixtures.

Each `.io` fixture file in the sub-directories of this `spec/` folder encodes one
request/response pair from the ethereum/execution-apis test suite.  This test
replays every fixture against a live Ethermint node and checks that the schema
of the response (key set + JSON value types) matches — without requiring exact
values, because many fields (block hash, gas price, …) are chain-specific.

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
