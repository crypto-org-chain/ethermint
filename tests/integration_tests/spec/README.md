# execution-apis

The specs are copied from the [ethereum/execution-apis](https://github.com/ethereum/execution-apis/tree/main/tests) repository.

Before the RPC schema tests are collected, `spec/conftest.py` syncs the local
`.io` fixtures and execution-apis chain fixture files from upstream. Set
`EXECUTION_APIS_REF` to test against a specific branch, tag, or commit. Set
`EXECUTION_APIS_SYNC=0` to skip the network sync and use fixtures already
generated in the local working tree.
