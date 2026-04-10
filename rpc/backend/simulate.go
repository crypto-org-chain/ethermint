package backend

import (
	"context"
	"encoding/json"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	rpctypes "github.com/evmos/ethermint/rpc/types"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

// simulateError is a JSON-RPC error with a specific error code.
type simulateError struct {
	code    int
	message string
}

func (e *simulateError) Error() string  { return e.message }
func (e *simulateError) ErrorCode() int { return e.code }

// SimulateV1 implements eth_simulateV1 by forwarding to the keeper via gRPC.
func (b *Backend) SimulateV1(opts rpctypes.SimOpts, blockNr rpctypes.BlockNumber) (json.RawMessage, error) {
	if len(opts.BlockStateCalls) == 0 {
		return nil, &rpctypes.InvalidParamsError{Message: "empty input"}
	} else if len(opts.BlockStateCalls) > rpctypes.MaxSimulateBlocks {
		return nil, &rpctypes.ClientLimitExceededError{Message: "too many blocks"}
	}

	header, err := b.HeaderByNumber(blockNr)
	if err != nil {
		return nil, err
	}

	// EthHeaderFromTendermint leaves GasLimit at 0; fill it from
	// consensus params so the simulated blocks inherit the real limit.
	if header.GasLimit == 0 {
		height := blockNr.Int64()
		if height <= 0 {
			if tendermintHeader, err := b.TendermintHeaderByNumber(blockNr); err == nil {
				height = tendermintHeader.Header.Height
			}
		}
		if maxGas, err := rpctypes.BlockMaxGasFromConsensusParams(b.ctx, b.clientCtx, height); err == nil && maxGas > 0 {
			header.GasLimit = uint64(maxGas)
		}
	}

	payload := rpctypes.SimulateV1Args{
		Opts:       opts,
		BaseHeader: header,
	}
	bz, err := json.Marshal(&payload)
	if err != nil {
		return nil, err
	}

	tendermintHeader, err := b.TendermintHeaderByNumber(blockNr)
	if err != nil {
		return nil, err
	}

	req := evmtypes.SimulateV1Request{
		Args:            bz,
		GasCap:          b.RPCGasCap(),
		ProposerAddress: sdk.ConsAddress(tendermintHeader.Header.ProposerAddress),
		ChainId:         b.chainID.Int64(),
	}

	ctx := rpctypes.ContextWithHeight(blockNr.Int64())
	timeout := b.RPCEVMTimeout()

	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	res, err := b.queryClient.SimulateV1(ctx, &req)
	if err != nil {
		return nil, err
	}

	// Simulation errors are returned via dedicated fields, not packed into Result.
	if res.ErrorCode != 0 {
		return nil, &simulateError{code: int(res.ErrorCode), message: res.ErrorMessage}
	}

	length := len(res.Result)
	if length > int(b.cfg.JSONRPC.ReturnDataLimit) && b.cfg.JSONRPC.ReturnDataLimit != 0 {
		return nil, fmt.Errorf("simulate returned result of length %d exceeding limit %d", length, b.cfg.JSONRPC.ReturnDataLimit)
	}

	return json.RawMessage(res.Result), nil
}
