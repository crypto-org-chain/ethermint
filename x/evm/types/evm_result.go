package types

// EVMResult is the internal result of an EVM execution.
// It contains all the data from MsgEthereumTxResponse plus additional
// query-only fields that should NOT be included in consensus.
//
// IMPORTANT: This struct is used internally for queries (eth_call, eth_estimateGas).
// For transaction execution that affects consensus, use ToMsgResponse() to get
// the consensus-safe MsgEthereumTxResponse.
type EVMResult struct {
	// Consensus fields - these are included in MsgEthereumTxResponse
	// and affect the app hash through ExecTxResult.Data

	// Hash is the ethereum transaction hash in hex format
	Hash string
	// Logs contains the transaction hash and the proto-compatible ethereum logs
	Logs []*Log
	// Ret is the returned data from evm function (result or data supplied with revert opcode)
	Ret []byte
	// VmError is the error returned by vm execution
	VmError string
	// GasUsed specifies how much gas was consumed by the transaction (after minGasMultiplier adjustment)
	GasUsed uint64
	// BlockHash is the block hash for json-rpc to use
	BlockHash []byte

	// Query-only fields - these are NOT included in MsgEthereumTxResponse
	// and do NOT affect consensus. They are only used for queries.

	// ExecutionGasUsed is the actual gas consumed during EVM execution,
	// before the minGasMultiplier adjustment. This is used for gas estimation
	// in eth_estimateGas queries only.
	ExecutionGasUsed uint64
}

// ToMsgResponse converts EVMResult to MsgEthereumTxResponse for consensus.
// This method strips out query-only fields (like ExecutionGasUsed) that should
// not be included in the transaction result that gets hashed into LastResultsHash.
//
// IMPORTANT: Always use this method when returning results from transaction execution
// (ApplyTransaction). Never include query-only fields in the response that goes
// through the message service router.
func (r *EVMResult) ToMsgResponse() *MsgEthereumTxResponse {
	return &MsgEthereumTxResponse{
		Hash:      r.Hash,
		Logs:      r.Logs,
		Ret:       r.Ret,
		VmError:   r.VmError,
		GasUsed:   r.GasUsed,
		BlockHash: r.BlockHash,
	}
}

// ToEthCallResponse converts EVMResult to EthCallResponse for query responses.
// This is used for eth_call queries and is separate from MsgEthereumTxResponse
// which is used for transaction execution and affects consensus.
func (r *EVMResult) ToEthCallResponse() *EthCallResponse {
	return &EthCallResponse{
		Hash:      r.Hash,
		Logs:      r.Logs,
		Ret:       r.Ret,
		VmError:   r.VmError,
		GasUsed:   r.GasUsed,
		BlockHash: r.BlockHash,
	}
}

// Failed returns true if the EVM execution failed
func (r *EVMResult) Failed() bool {
	return len(r.VmError) > 0
}
