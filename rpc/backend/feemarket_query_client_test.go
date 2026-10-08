package backend

import (
	sdkmath "cosmossdk.io/math"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/evmos/ethermint/rpc/backend/mocks"
	rpc "github.com/evmos/ethermint/rpc/types"
	feemarkettypes "github.com/evmos/ethermint/x/feemarket/types"
)

var _ feemarkettypes.QueryClient = &mocks.FeeMarketQueryClient{}

// Params
func RegisterFeeMarketParams(feeMarketClient *mocks.FeeMarketQueryClient, height int64) {
	RegisterFeeMarketParamsWithBaseFee(feeMarketClient, height, feemarkettypes.DefaultParams().BaseFee)
}

func RegisterFeeMarketParamsWithBaseFee(feeMarketClient *mocks.FeeMarketQueryClient, height int64, baseFee sdkmath.Int) {
	RegisterFeeMarketParamsWith(feeMarketClient, height, feeMarketParamsWithBaseFee(baseFee))
}

// feeMarketParamsWithBaseFee returns default params with the given stored base fee.
func feeMarketParamsWithBaseFee(baseFee sdkmath.Int) feemarkettypes.Params {
	params := feemarkettypes.DefaultParams()
	params.BaseFee = baseFee
	return params
}

func RegisterFeeMarketParamsWith(feeMarketClient *mocks.FeeMarketQueryClient, height int64, params feemarkettypes.Params) {
	feeMarketClient.On("Params", rpc.ContextWithHeight(height), &feemarkettypes.QueryParamsRequest{}).
		Return(&feemarkettypes.QueryParamsResponse{Params: params}, nil)
}

func RegisterFeeMarketParamsError(feeMarketClient *mocks.FeeMarketQueryClient, height int64) {
	feeMarketClient.On("Params", rpc.ContextWithHeight(height), &feemarkettypes.QueryParamsRequest{}).
		Return(nil, sdkerrors.ErrInvalidRequest)
}
