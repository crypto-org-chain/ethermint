from __future__ import annotations

import base64
import json
import re
from typing import Any, Dict, List, Optional, Tuple

from eth_keys import keys
from eth_utils import keccak

# =============================================================================
# Proto to Amino Type Mapping
# =============================================================================

# Maps protobuf type URLs to Amino type names
PROTO_TO_AMINO_TYPE_MAP: Dict[str, str] = {
    # Staking
    "/cosmos.staking.v1beta1.MsgDelegate": "cosmos-sdk/MsgDelegate",
    "/cosmos.staking.v1beta1.MsgUndelegate": "cosmos-sdk/MsgUndelegate",
    "/cosmos.staking.v1beta1.MsgBeginRedelegate": "cosmos-sdk/MsgBeginRedelegate",
    "/cosmos.staking.v1beta1.MsgCreateValidator": "cosmos-sdk/MsgCreateValidator",
    "/cosmos.staking.v1beta1.MsgEditValidator": "cosmos-sdk/MsgEditValidator",
    # Bank
    "/cosmos.bank.v1beta1.MsgSend": "cosmos-sdk/MsgSend",
    "/cosmos.bank.v1beta1.MsgMultiSend": "cosmos-sdk/MsgMultiSend",
    # Distribution
    "/cosmos.distribution.v1beta1.MsgWithdrawDelegatorReward": (
        "cosmos-sdk/MsgWithdrawDelegationReward"
    ),
    "/cosmos.distribution.v1beta1.MsgSetWithdrawAddress": (
        "cosmos-sdk/MsgModifyWithdrawAddress"
    ),
    "/cosmos.distribution.v1beta1.MsgWithdrawValidatorCommission": (
        "cosmos-sdk/MsgWithdrawValidatorCommission"
    ),
    "/cosmos.distribution.v1beta1.MsgFundCommunityPool": (
        "cosmos-sdk/MsgFundCommunityPool"
    ),
    # Governance
    "/cosmos.gov.v1beta1.MsgVote": "cosmos-sdk/MsgVote",
    "/cosmos.gov.v1beta1.MsgDeposit": "cosmos-sdk/MsgDeposit",
    "/cosmos.gov.v1beta1.MsgSubmitProposal": "cosmos-sdk/MsgSubmitProposal",
    # Slashing
    "/cosmos.slashing.v1beta1.MsgUnjail": "cosmos-sdk/MsgUnjail",
    # IBC
    "/ibc.applications.transfer.v1.MsgTransfer": "cosmos-sdk/MsgTransfer",
    # Authz
    "/cosmos.authz.v1beta1.MsgGrant": "cosmos-sdk/MsgGrant",
    "/cosmos.authz.v1beta1.MsgRevoke": "cosmos-sdk/MsgRevoke",
    "/cosmos.authz.v1beta1.MsgExec": "cosmos-sdk/MsgExec",
    # Feegrant
    "/cosmos.feegrant.v1beta1.MsgGrantAllowance": "cosmos-sdk/MsgGrantAllowance",
    "/cosmos.feegrant.v1beta1.MsgRevokeAllowance": "cosmos-sdk/MsgRevokeAllowance",
}


def proto_to_amino_type(proto_type: str) -> str:
    return PROTO_TO_AMINO_TYPE_MAP.get(proto_type, proto_type)


def register_amino_type(proto_type: str, amino_type: str) -> None:
    PROTO_TO_AMINO_TYPE_MAP[proto_type] = amino_type


def _python_type_to_eip712(value: Any, parent_key: str = "") -> str:
    if isinstance(value, bool):
        return "bool"
    elif isinstance(value, int):
        return "uint256"
    elif isinstance(value, str):
        return "string"
    elif isinstance(value, bytes):
        return "bytes"
    elif isinstance(value, list):
        if len(value) == 0:
            # Default to string array for empty lists
            return "string[]"
        # Infer from first element
        item_type = _python_type_to_eip712(value[0], parent_key)
        return f"{item_type}[]"
    elif isinstance(value, dict):
        # Complex type - will need separate type definition
        # Use capitalized parent key as type name
        return _make_type_name(parent_key)
    else:
        return "string"


def _make_type_name(key: str) -> str:
    # Handle special cases
    if key.lower() == "amount":
        return "TypeAmount"
    if key.lower() == "value":
        return "MsgValue"

    # Convert snake_case to PascalCase
    parts = key.split("_")
    pascal = "".join(word.capitalize() for word in parts)

    # Add Type prefix if not already there
    if not pascal.startswith("Type") and not pascal.startswith("Msg"):
        pascal = "Type" + pascal

    return pascal


def _extract_types_from_value(
    value: Any,
    type_name: str,
    types_accumulator: Dict[str, List[Dict[str, str]]],
    visited: Optional[set] = None,
) -> None:
    if visited is None:
        visited = set()

    if type_name in visited:
        return
    visited.add(type_name)

    if not isinstance(value, dict):
        return

    fields = []
    for key, val in value.items():
        field_type = _python_type_to_eip712(val, key)
        fields.append({"name": key, "type": field_type})

        # Recursively process nested dicts
        if isinstance(val, dict):
            nested_type_name = _make_type_name(key)
            _extract_types_from_value(val, nested_type_name, types_accumulator, visited)
        elif isinstance(val, list) and len(val) > 0 and isinstance(val[0], dict):
            # Array of objects
            nested_type_name = _make_type_name(key)
            _extract_types_from_value(
                val[0], nested_type_name, types_accumulator, visited
            )

    types_accumulator[type_name] = fields


def infer_msg_types(msg_value: Dict[str, Any]) -> Dict[str, List[Dict[str, str]]]:
    types: Dict[str, List[Dict[str, str]]] = {}
    _extract_types_from_value(msg_value, "MsgValue", types)
    return types


# =============================================================================
# Message Conversion
# =============================================================================


def proto_msg_to_amino(msg: Dict[str, Any]) -> Dict[str, Any]:
    msg_type = msg.get("@type", "")
    amino_type = proto_to_amino_type(msg_type)

    # Extract value (everything except @type)
    value = {k: v for k, v in msg.items() if k != "@type"}

    return {
        "type": amino_type,
        "value": value,
    }


# =============================================================================
# EIP-712 Typed Data Builder
# =============================================================================


def build_eip712_domain(chain_id_num: int) -> Dict[str, Any]:
    return {
        "name": "Cosmos Web3",
        "version": "1.0.0",
        "chainId": chain_id_num,
        "verifyingContract": "cosmos",
        "salt": "0",
    }


def build_base_types() -> Dict[str, List[Dict[str, str]]]:
    return {
        "EIP712Domain": [
            {"name": "name", "type": "string"},
            {"name": "version", "type": "string"},
            {"name": "chainId", "type": "uint256"},
            {"name": "verifyingContract", "type": "string"},
            {"name": "salt", "type": "string"},
        ],
        "Tx": [
            {"name": "account_number", "type": "string"},
            {"name": "chain_id", "type": "string"},
            {"name": "fee", "type": "Fee"},
            {"name": "memo", "type": "string"},
            {"name": "msgs", "type": "Msg[]"},
            {"name": "sequence", "type": "string"},
        ],
        "Fee": [
            {"name": "feePayer", "type": "string"},
            {"name": "amount", "type": "Coin[]"},
            {"name": "gas", "type": "string"},
        ],
        "Coin": [
            {"name": "denom", "type": "string"},
            {"name": "amount", "type": "string"},
        ],
        "Msg": [
            {"name": "type", "type": "string"},
            {"name": "value", "type": "MsgValue"},
        ],
    }


def build_legacy_eip712_typed_data(
    chain_id: str,
    chain_id_num: int,
    account_number: int,
    sequence: int,
    fee_payer: str,
    fee_amount: str,
    fee_denom: str,
    gas: int,
    msgs: List[Dict[str, Any]],
    memo: str = "",
    custom_msg_types: Optional[Dict[str, List[Dict[str, str]]]] = None,
) -> Dict[str, Any]:
    # Build domain
    domain = build_eip712_domain(chain_id_num)

    # Build base types
    types = build_base_types()

    # Convert messages to Amino format
    amino_msgs = []
    for msg in msgs:
        if "@type" in msg:
            amino_msgs.append(proto_msg_to_amino(msg))
        elif "type" in msg and "value" in msg:
            # Already in amino format
            amino_msgs.append(msg)
        else:
            raise ValueError(f"Unknown message format: {msg}")

    # Determine message value types
    if custom_msg_types:
        types.update(custom_msg_types)
    elif amino_msgs:
        # Infer types from first message (legacy behavior)
        first_msg_value = amino_msgs[0]["value"]
        msg_types = infer_msg_types(first_msg_value)
        types.update(msg_types)

    # Build the message payload
    message = {
        "account_number": str(account_number),
        "chain_id": chain_id,
        "fee": {
            "feePayer": fee_payer,
            "amount": [{"denom": fee_denom, "amount": fee_amount}],
            "gas": str(gas),
        },
        "memo": memo,
        "msgs": amino_msgs,
        "sequence": str(sequence),
    }

    return {
        "types": types,
        "primaryType": "Tx",
        "domain": domain,
        "message": message,
    }


# =============================================================================
# EIP-712 Encoding (Core Implementation)
# =============================================================================


def _encode_type(type_name: str, types: Dict[str, List[Dict[str, str]]]) -> str:
    result = type_name + "("
    fields = types[type_name]
    field_strs = []
    for field in fields:
        field_strs.append(f"{field['type']} {field['name']}")
    result += ",".join(field_strs) + ")"
    return result


def _find_type_dependencies(
    type_name: str,
    types: Dict[str, List[Dict[str, str]]],
    deps: set,
) -> None:
    if type_name in deps or type_name not in types:
        return
    deps.add(type_name)
    for field in types[type_name]:
        field_type = field["type"]
        # Handle array types
        if field_type.endswith("[]"):
            field_type = field_type[:-2]
        if field_type in types:
            _find_type_dependencies(field_type, types, deps)


def _encode_type_with_deps(
    primary_type: str,
    types: Dict[str, List[Dict[str, str]]],
) -> str:
    deps: set = set()
    _find_type_dependencies(primary_type, types, deps)
    deps.discard(primary_type)

    result = _encode_type(primary_type, types)
    for dep in sorted(deps):
        result += _encode_type(dep, types)
    return result


def _type_hash(type_name: str, types: Dict[str, List[Dict[str, str]]]) -> bytes:
    encoded = _encode_type_with_deps(type_name, types)
    return keccak(text=encoded)


def _encode_data(
    type_name: str,
    data: Any,
    types: Dict[str, List[Dict[str, str]]],
) -> bytes:
    # Handle primitive types
    if type_name == "string":
        return keccak(text=data if data else "")
    elif type_name == "bytes":
        if not data:
            return keccak(b"")
        return keccak(data if isinstance(data, bytes) else bytes.fromhex(data))
    elif type_name == "bool":
        return (1 if data else 0).to_bytes(32, "big")
    elif type_name == "address":
        # For Ethermint legacy, address can be a string like "cosmos"
        if isinstance(data, str) and not data.startswith("0x"):
            # Non-hex address - hash as string
            return keccak(text=data)
        # Standard hex address
        addr = data[2:] if data.startswith("0x") else data
        return bytes.fromhex(addr.zfill(64))
    elif type_name.startswith("uint") or type_name.startswith("int"):
        if isinstance(data, str):
            if data.startswith("0x"):
                val = int(data, 16)
            else:
                val = int(data)
        else:
            val = int(data) if data else 0
        return val.to_bytes(32, "big", signed=type_name.startswith("int"))
    elif type_name.startswith("bytes") and type_name != "bytes":
        # Fixed-size bytes (bytes1, bytes32, etc.)
        if isinstance(data, str):
            data = bytes.fromhex(data[2:] if data.startswith("0x") else data)
        return data.ljust(32, b"\x00") if data else b"\x00" * 32
    elif type_name.endswith("[]"):
        # Array type
        item_type = type_name[:-2]
        encoded_items = b""
        for item in data or []:
            if item_type in types:
                encoded_items += _hash_struct(item_type, item, types)
            else:
                encoded_items += _encode_data(item_type, item, types)
        return keccak(encoded_items)
    elif type_name in types:
        # Struct type - return hash
        return _hash_struct(type_name, data, types)
    else:
        # Unknown type - treat as string
        return keccak(text=str(data) if data else "")


def _hash_struct(
    type_name: str,
    data: Dict[str, Any],
    types: Dict[str, List[Dict[str, str]]],
) -> bytes:
    type_hash = _type_hash(type_name, types)
    encoded_values = b""

    for field in types[type_name]:
        field_name = field["name"]
        field_type = field["type"]
        value = data.get(field_name) if data else None
        encoded_values += _encode_data(field_type, value, types)

    return keccak(type_hash + encoded_values)


def _hash_domain(
    domain: Dict[str, Any], types: Dict[str, List[Dict[str, str]]]
) -> bytes:
    return _hash_struct("EIP712Domain", domain, types)


# =============================================================================
# Signing
# =============================================================================


def sign_legacy_eip712(
    typed_data: Dict[str, Any],
    private_key_hex: str,
) -> Tuple[bytes, bytes]:
    types = typed_data["types"]
    domain = typed_data["domain"]
    primary_type = typed_data["primaryType"]
    message = typed_data["message"]

    # Compute domain separator
    domain_separator = _hash_domain(domain, types)

    # Compute message hash
    message_hash = _hash_struct(primary_type, message, types)

    sig_hash = keccak(b"\x19\x01" + domain_separator + message_hash)

    # Sign the hash
    private_key_hex = private_key_hex.replace("0x", "")
    pk = keys.PrivateKey(bytes.fromhex(private_key_hex))
    signature = pk.sign_msg_hash(sig_hash)

    # Convert to bytes: r (32) + s (32) + v (1)
    # v needs to be 27 or 28 for legacy format
    v = signature.v + 27 if signature.v < 27 else signature.v
    sig_bytes = (
        signature.r.to_bytes(32, "big") + signature.s.to_bytes(32, "big") + bytes([v])
    )

    return sig_bytes, sig_hash


def get_public_key(private_key_hex: str) -> bytes:
    private_key_hex = private_key_hex.replace("0x", "")
    pk = keys.PrivateKey(bytes.fromhex(private_key_hex))
    return pk.public_key.to_compressed_bytes()


def build_cosmos_tx_json(
    msgs: List[Dict[str, Any]],
    fee_amount: str,
    fee_denom: str,
    gas: int,
    fee_payer: str,
    public_key_bytes: bytes,
    signature: bytes,
    chain_id_num: int,
    sequence: int,
    memo: str = "",
) -> Dict[str, Any]:
    return {
        "body": {
            "messages": msgs,
            "memo": memo,
            "timeout_height": "0",
            "extension_options": [
                {
                    "@type": "/ethermint.types.v1.ExtensionOptionsWeb3Tx",
                    "typedDataChainId": str(chain_id_num),
                    "feePayer": fee_payer,
                    "feePayerSig": base64.b64encode(signature).decode("ascii"),
                }
            ],
            "non_critical_extension_options": [],
        },
        "auth_info": {
            "signer_infos": [
                {
                    "public_key": {
                        "@type": "/ethermint.crypto.v1.ethsecp256k1.PubKey",
                        "key": base64.b64encode(public_key_bytes).decode("ascii"),
                    },
                    "mode_info": {
                        "single": {
                            "mode": "SIGN_MODE_LEGACY_AMINO_JSON",
                        }
                    },
                    "sequence": str(sequence),
                }
            ],
            "fee": {
                "amount": [{"denom": fee_denom, "amount": fee_amount}],
                "gas_limit": str(gas),
                "payer": "",
                "granter": "",
            },
        },
        "signatures": [""],
    }


def parse_chain_id(chain_id: str) -> int:
    match = re.search(r"_(\d+)-", chain_id)
    if match:
        return int(match.group(1))
    # Fallback: try to parse as pure number
    try:
        return int(chain_id)
    except ValueError:
        raise ValueError(f"Cannot parse chain ID: {chain_id}")


class LegacyEIP712Signer:

    def __init__(
        self,
        private_key: str,
        chain_id: str,
        fee_denom: str = "aphoton",
    ):
        self.private_key = private_key.replace("0x", "")
        self.chain_id = chain_id
        self.chain_id_num = parse_chain_id(chain_id)
        self.fee_denom = fee_denom
        self.public_key = get_public_key(self.private_key)

    def sign_tx(
        self,
        msgs: List[Dict[str, Any]],
        fee_payer: str,
        account_number: int,
        sequence: int,
        gas: int = 200000,
        fee_amount: str = "20000",
        fee_denom: Optional[str] = None,
        memo: str = "",
        custom_msg_types: Optional[Dict[str, List[Dict[str, str]]]] = None,
    ) -> Dict[str, Any]:
        try:
            fee_denom = fee_denom or self.fee_denom

            # Build EIP-712 typed data
            typed_data = build_legacy_eip712_typed_data(
                chain_id=self.chain_id,
                chain_id_num=self.chain_id_num,
                account_number=account_number,
                sequence=sequence,
                fee_payer=fee_payer,
                fee_amount=fee_amount,
                fee_denom=fee_denom,
                gas=gas,
                msgs=msgs,
                memo=memo,
                custom_msg_types=custom_msg_types,
            )

            # Sign
            signature, msg_hash = sign_legacy_eip712(typed_data, self.private_key)

            # Build transaction JSON
            tx_json = build_cosmos_tx_json(
                msgs=msgs,
                fee_amount=fee_amount,
                fee_denom=fee_denom,
                gas=gas,
                fee_payer=fee_payer,
                public_key_bytes=self.public_key,
                signature=signature,
                chain_id_num=self.chain_id_num,
                sequence=sequence,
                memo=memo,
            )

            return {
                "success": True,
                "tx_json": json.dumps(tx_json, indent=2),
                "typed_data": json.dumps(typed_data, indent=2),
                "signature_hex": signature.hex(),
                "tx": tx_json,
                "typed_data_dict": typed_data,
            }

        except Exception as e:
            return {
                "success": False,
                "error": str(e),
            }
