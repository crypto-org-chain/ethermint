package types

type StateTransitionApplyResult struct {
	RealGasUsed uint64
	Response    *MsgEthereumTxResponse
}
