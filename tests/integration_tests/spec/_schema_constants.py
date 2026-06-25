# Named constant sets that drive the schema test's allow-lists and exceptions.
# Controls which spec cases are excluded, which RPC methods are treated as
# not-yet-implemented, and which known Ethermint/Geth divergences are tolerated
# without failing the schema assertion.

from _rpc_spec_common import CATEGORY_TITLES

REPORT_FILENAME = "rpc_schema_report.md"
SCHEMA_CATEGORY_TITLES = {
    **CATEGORY_TITLES,
    "schema_correct": "correct implemented by schema",
}

# Methods with a partial implementation: the endpoint exists but the response
# schema doesn't yet match the execution-apis spec. Classified as not_implemented
# so they are excluded from schema-mismatch assertions until the shape is complete.
INCOMPLETE_UNIMPLEMENTED_RPC_METHODS = {
    # Returns an incomplete txpool shape compared with the execution-apis fixture.
    "txpool_content",
}

# Full set of methods whose not_implemented verdict is expected and allowed.
# Includes all partially-implemented methods (above) plus those not implemented at all.
UNIMPLEMENTED_RPC_METHODS = {
    "debug_getRawBlock",
    "debug_getRawHeader",
    "debug_getRawReceipts",
    "debug_getRawTransaction",
    "eth_blobBaseFee",
    "eth_capabilities",
    "eth_config",
    "eth_getStorageValues",
    "testing_buildBlockV1",
    "txpool_contentFrom",
} | INCOMPLETE_UNIMPLEMENTED_RPC_METHODS

SCHEMA_MISMATCH_WHITELIST = {
    "request_schema_wrong": set(),
    "response_schema_wrong": set(),
    "mixed_wrong": set(),
}

ETHERMINT_MODERN_BLOCK_FIELD_SCHEMA_EXCEPTIONS = {
    # Genesis is a pre-fork block in the copied Geth fixture. Ethermint currently
    # returns Prague-era block fields for it, so ignore only those extra fields
    # until the block formatter becomes fork-aware for historical heights.
    "eth_getBlockByNumber/get-genesis",
    # The copied geth block-hash fixture is also from an old fork era, but this
    # test rewrites the hash to a local Ethermint block before comparing schema.
    "eth_getBlockByHash/get-block-by-hash",
}
ETHERMINT_LOCAL_TX_SCHEMA_EXCEPTIONS = {
    "eth_getBlockByHash/get-block-by-hash",
}
RELAXED_BLOCK_TRANSACTION_SCHEMA_EXCEPTIONS = {
    # These block tags resolve against the local Ethermint chain, not the copied
    # Geth fixture chain. Keep checking the block response envelope, but do not
    # require per-transaction object schemas to line up when the actual
    # transactions are from a different chain and can be different tx types.
    "eth_getBlockByNumber/get-finalized",
    "eth_getBlockByNumber/get-latest",
    "eth_getBlockByNumber/get-safe",
}

MODERN_BLOCK_FIELDS = {
    "baseFeePerGas",
    "blobGasUsed",
    "excessBlobGas",
    "parentBeaconBlockRoot",
    "requestsHash",
    "withdrawals",
    "withdrawalsRoot",
}
ETH_SIMULATE_BLOCK_HARDFORK_FIELDS = MODERN_BLOCK_FIELDS
ETH_SIMULATE_TRANSACTION_HARDFORK_FIELDS = {
    "accessList",
    "authorizationList",
    "blobVersionedHashes",
    "chainId",
    "maxFeePerBlobGas",
    "maxFeePerGas",
    "maxPriorityFeePerGas",
    "yParity",
}
ETH_SIMULATE_TIMESTAMP_HEADROOM = 120

LOCAL_FIXTURE_TX_FIELDS = {
    "chainId",
}
LOCAL_TRANSACTION_SCHEMA_EXCEPTIONS = {
    "eth_getTransactionByBlockHashAndIndex/get-block-n",
    "eth_getTransactionByBlockNumberAndIndex/get-block-n",
    "eth_getTransactionByHash/get-legacy-create",
    "eth_getTransactionByHash/get-legacy-input",
    "eth_getTransactionByHash/get-legacy-tx",
}
LOCAL_RECEIPT_SCHEMA_EXCEPTIONS = {
    "eth_getBlockReceipts/get-block-receipts-by-hash",
    "eth_getBlockReceipts/get-block-receipts-latest",
}
LEGACY_RECEIPT_ROOT_STATUS_SCHEMA_EXCEPTIONS = {
    "eth_getTransactionReceipt/get-legacy-contract",
    "eth_getTransactionReceipt/get-legacy-input",
    "eth_getTransactionReceipt/get-legacy-receipt",
}
LOCAL_RECEIPT_FUTURE_NULL_RESULT_ERROR_EXCEPTIONS = {
    "eth_getBlockReceipts/get-block-receipts-future",
}
LOCAL_LOG_SCHEMA_EXCEPTIONS = {
    "eth_getLogs/filter-with-blockHash",
    "eth_getLogs/filter-with-blockHash-and-topics",
}
LOCAL_LOG_FUTURE_BLOCK_RANGE_EXCEPTIONS = {
    "eth_getLogs/filter-error-future-block-range",
}
LOCAL_PROOF_SCHEMA_EXCEPTIONS = {
    "eth_getProof/get-account-proof-blockhash",
}
LOCAL_FIXTURE_RECEIPT_ADDRESS_FIELDS = {
    "contractAddress",
    "to",
}

EXCLUDED_SCHEMA_SPEC_CASES = {
    # Ethermint currently emits Prague-era block fields for historical blocks.
    # Exclude these explicitly fork-scoped Geth fixtures instead of treating
    # their expected older response shape as a current Ethermint schema failure.
    "eth_getBlockByNumber/get-block-london-fork",
    "eth_getBlockByNumber/get-block-merge-fork",
    "eth_getBlockByNumber/get-block-shanghai-fork",
    "eth_getBlockByNumber/get-block-cancun-fork",
}

TRANSACTION_BY_HASH_LOCAL_TX_KEYS = {
    "eth_getTransactionByHash/get-access-list": "access_list",
    "eth_getTransactionByHash/get-blob-tx": "blob",
    "eth_getTransactionByHash/get-dynamic-fee": "dynamic_fee",
    "eth_getTransactionByHash/get-legacy-create": "legacy_create",
    "eth_getTransactionByHash/get-legacy-input": "legacy_create",
    "eth_getTransactionByHash/get-legacy-tx": "legacy",
    "eth_getTransactionByHash/get-setcode-tx": "setcode",
}
TRANSACTION_RECEIPT_LOCAL_TX_KEYS = {
    "eth_getTransactionReceipt/get-access-list": "access_list",
    "eth_getTransactionReceipt/get-blob-tx": "blob",
    "eth_getTransactionReceipt/get-dynamic-fee": "dynamic_fee",
    "eth_getTransactionReceipt/get-legacy-contract": "legacy_create",
    "eth_getTransactionReceipt/get-legacy-input": "legacy_create",
    "eth_getTransactionReceipt/get-legacy-receipt": "legacy",
    "eth_getTransactionReceipt/get-setcode-tx": "setcode",
}
SEND_RAW_TRANSACTION_LOCAL_TX_KEYS = {
    "eth_sendRawTransaction/send-access-list-transaction": "access_list",
    "eth_sendRawTransaction/send-blob-tx": "blob",
    "eth_sendRawTransaction/send-dynamic-fee-access-list-transaction": (
        "dynamic_fee_access_list"
    ),
    "eth_sendRawTransaction/send-dynamic-fee-transaction": "dynamic_fee_create",
    "eth_sendRawTransaction/send-legacy-transaction": "legacy",
}

LEGACY_CREATE_BYTECODE = (
    "0x600d380380600d6000396000f360004381526020014681526020014181526020014"
    "881526020014481526020013281526020013481526020016000f3"
)
BLOB_VERSIONED_HASH = (
    "0x0100000000000000000000000000000000000000000000000000000000000000"
)
