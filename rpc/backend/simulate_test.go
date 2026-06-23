package backend

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/evmos/ethermint/rpc/backend/mocks"
	rpctypes "github.com/evmos/ethermint/rpc/types"
	"github.com/evmos/ethermint/tests"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
	mock "github.com/stretchr/testify/mock"
)

func (suite *BackendTestSuite) TestSimulateV1() {
	validator := sdk.AccAddress(tests.GenerateAddress().Bytes())
	baseFee := sdkmath.NewInt(1)
	height := int64(1)

	testCases := []struct {
		name         string
		registerMock func()
		opts         rpctypes.SimOpts
		blockNr      rpctypes.BlockNumber
		expPass      bool
		expErrMsg    string
	}{
		{
			name:         "fail - empty block state calls",
			registerMock: func() {},
			opts: rpctypes.SimOpts{
				BlockStateCalls: []rpctypes.SimBlock{},
			},
			blockNr:   rpctypes.BlockNumber(1),
			expPass:   false,
			expErrMsg: "empty input",
		},
		{
			name:         "fail - too many blocks",
			registerMock: func() {},
			opts: rpctypes.SimOpts{
				BlockStateCalls: make([]rpctypes.SimBlock, rpctypes.MaxSimulateBlocks+1),
			},
			blockNr:   rpctypes.BlockNumber(1),
			expPass:   false,
			expErrMsg: "too many blocks",
		},
		{
			name: "fail - HeaderByNumber error",
			registerMock: func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				RegisterBlockError(client, height)
			},
			opts: rpctypes.SimOpts{
				BlockStateCalls: []rpctypes.SimBlock{{}},
			},
			blockNr: rpctypes.BlockNumber(1),
			expPass: false,
		},
		{
			name: "fail - SimulateV1 gRPC error",
			registerMock: func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				// simulate.go calls both TendermintHeaderByNumber and HeaderByNumber; both mocks are needed.
				RegisterHeader(client, &height, nil)
				RegisterConsensusParams(client, height)
				RegisterEmptyBlockResults(client, height)
				RegisterBlock(client, height, nil)
				RegisterBaseFee(queryClient, baseFee)
				RegisterValidatorAccount(queryClient, validator)
				RegisterSimulateV1Error(queryClient)
			},
			opts: rpctypes.SimOpts{
				BlockStateCalls: []rpctypes.SimBlock{{}},
			},
			blockNr: rpctypes.BlockNumber(1),
			expPass: false,
		},
		{
			name: "fail - result exceeds return data limit",
			registerMock: func() {
				client := suite.backend.clientCtx.Client.(*mocks.Client)
				queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
				RegisterHeader(client, &height, nil)
				RegisterConsensusParams(client, height)
				RegisterEmptyBlockResults(client, height)
				RegisterBlock(client, height, nil)
				RegisterBaseFee(queryClient, baseFee)
				RegisterValidatorAccount(queryClient, validator)
				// Large result that exceeds the limit.
				largeResult := make([]byte, 1024)
				RegisterSimulateV1Success(queryClient, largeResult)
				suite.backend.cfg.JSONRPC.ReturnDataLimit = 10
			},
			opts: rpctypes.SimOpts{
				BlockStateCalls: []rpctypes.SimBlock{{}},
			},
			blockNr:   rpctypes.BlockNumber(1),
			expPass:   false,
			expErrMsg: "exceeding limit",
		},
	}

	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("case %s", tc.name), func() {
			suite.SetupTest()
			// Reset return data limit (may have been modified in a previous test case).
			suite.backend.cfg.JSONRPC.ReturnDataLimit = 0
			tc.registerMock()

			res, err := suite.backend.SimulateV1(tc.opts, tc.blockNr)
			if tc.expPass {
				suite.Require().NoError(err)
				suite.Require().NotNil(res)
			} else {
				suite.Require().Error(err)
				if tc.expErrMsg != "" {
					suite.Require().Contains(err.Error(), tc.expErrMsg)
				}
			}
		})
	}
}

// TestSimulateV1ErrorCode verifies that when gRPC returns OK and the response
// carries ErrorCode/ErrorMessage (as grpc_query.SimulateV1 now does after a
// failed sim.Execute), the backend surfaces a *simulateError with the same code
// and message. This replaces the old behaviour where the error was JSON-encoded
// inside the Result bytes.
func (suite *BackendTestSuite) TestSimulateV1ErrorCode() {
	validator := sdk.AccAddress(tests.GenerateAddress().Bytes())
	baseFee := sdkmath.NewInt(1)
	height := int64(1)

	suite.SetupTest()
	client := suite.backend.clientCtx.Client.(*mocks.Client)
	queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
	RegisterHeader(client, &height, nil)
	RegisterConsensusParams(client, height)
	RegisterEmptyBlockResults(client, height)
	RegisterBlock(client, height, nil)
	RegisterBaseFee(queryClient, baseFee)
	RegisterValidatorAccount(queryClient, validator)

	msg := "too many blocks"
	queryClient.On("SimulateV1", mock.Anything, mock.MatchedBy(func(req *evmtypes.SimulateV1Request) bool {
		return req != nil
	})).Return(&evmtypes.SimulateV1Response{
		ErrorCode:    int32(rpctypes.ErrCodeClientLimitExceeded),
		ErrorMessage: msg,
	}, nil)

	opts := rpctypes.SimOpts{BlockStateCalls: []rpctypes.SimBlock{{}}}
	_, err := suite.backend.SimulateV1(opts, rpctypes.BlockNumber(1))
	suite.Require().Error(err)

	var simErr *simulateError
	suite.Require().True(errors.As(err, &simErr))
	suite.Require().Equal(rpctypes.ErrCodeClientLimitExceeded, simErr.ErrorCode())
	suite.Require().Equal(msg, simErr.Error())
}

// TestSimulateV1ReturnDataLimitZeroUnlimited verifies that ReturnDataLimit=0 means
// no limit and large results are returned successfully.
func (suite *BackendTestSuite) TestSimulateV1ReturnDataLimitZeroUnlimited() {
	validator := sdk.AccAddress(tests.GenerateAddress().Bytes())
	baseFee := sdkmath.NewInt(1)
	height := int64(1)

	suite.SetupTest()
	suite.backend.cfg.JSONRPC.ReturnDataLimit = 0 // unlimited
	client := suite.backend.clientCtx.Client.(*mocks.Client)
	queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
	RegisterHeader(client, &height, nil)
	RegisterConsensusParams(client, height)
	RegisterEmptyBlockResults(client, height)
	RegisterBlock(client, height, nil)
	RegisterBaseFee(queryClient, baseFee)
	RegisterValidatorAccount(queryClient, validator)
	largeResult := []byte(`[{"number":"0x1","calls":[]}]`)
	RegisterSimulateV1Success(queryClient, largeResult)

	opts := rpctypes.SimOpts{BlockStateCalls: []rpctypes.SimBlock{{}}}
	res, err := suite.backend.SimulateV1(opts, rpctypes.BlockNumber(1))
	suite.Require().NoError(err)
	suite.Require().NotNil(res)
}

// TestSimulateErrorType verifies the simulateError type fields.
func (suite *BackendTestSuite) TestSimulateErrorType() {
	msg := "block numbers must be in order: 5 <= 3"
	e := &simulateError{code: rpctypes.ErrCodeBlockNumberInvalid, message: msg}
	suite.Require().Equal(rpctypes.ErrCodeBlockNumberInvalid, e.ErrorCode())
	suite.Require().Equal(msg, e.Error())
}

// TestSimulateV1JSONPayload verifies that the SimulateV1Args payload is
// marshalled correctly and round-trips cleanly.
func (suite *BackendTestSuite) TestSimulateV1JSONPayload() {
	opts := rpctypes.SimOpts{
		BlockStateCalls:        []rpctypes.SimBlock{{}},
		TraceTransfers:         true,
		Validation:             false,
		ReturnFullTransactions: true,
	}
	payload := rpctypes.SimulateV1Args{
		Opts:       opts,
		BaseHeader: nil,
	}
	bz, err := json.Marshal(&payload)
	suite.Require().NoError(err)

	var decoded rpctypes.SimulateV1Args
	suite.Require().NoError(json.Unmarshal(bz, &decoded))
	suite.Require().Equal(opts.TraceTransfers, decoded.Opts.TraceTransfers)
	suite.Require().Equal(opts.Validation, decoded.Opts.Validation)
	suite.Require().Equal(opts.ReturnFullTransactions, decoded.Opts.ReturnFullTransactions)
	suite.Require().Len(decoded.Opts.BlockStateCalls, 1)
}

// TestSimulateV1WithTimeout verifies the context.WithTimeout branch taken
// when EVMTimeout > 0.
func (suite *BackendTestSuite) TestSimulateV1WithTimeout() {
	validator := sdk.AccAddress(tests.GenerateAddress().Bytes())
	baseFee := sdkmath.NewInt(1)
	height := int64(1)

	suite.SetupTest()
	suite.backend.cfg.JSONRPC.EVMTimeout = 5 * time.Second // triggers context.WithTimeout path
	client := suite.backend.clientCtx.Client.(*mocks.Client)
	queryClient := suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient)
	RegisterHeader(client, &height, nil)
	RegisterConsensusParams(client, height)
	RegisterEmptyBlockResults(client, height)
	RegisterBlock(client, height, nil)
	RegisterBaseFee(queryClient, baseFee)
	RegisterValidatorAccount(queryClient, validator)
	result := []byte(`[{"number":"0x1","calls":[]}]`)
	RegisterSimulateV1Success(queryClient, result)

	opts := rpctypes.SimOpts{BlockStateCalls: []rpctypes.SimBlock{{}}}
	res, err := suite.backend.SimulateV1(opts, rpctypes.BlockNumber(1))
	suite.Require().NoError(err)
	suite.Require().NotNil(res)
}

// RegisterSimulateV1Success registers a SimulateV1 mock that returns the given result bytes.
func RegisterSimulateV1Success(queryClient *mocks.EVMQueryClient, result []byte) {
	queryClient.On("SimulateV1", mock.Anything, mock.MatchedBy(func(req *evmtypes.SimulateV1Request) bool {
		return req != nil
	})).Return(&evmtypes.SimulateV1Response{Result: result}, nil)
}

// RegisterSimulateV1Error registers a SimulateV1 mock that returns a gRPC error.
func RegisterSimulateV1Error(queryClient *mocks.EVMQueryClient) {
	queryClient.On("SimulateV1", mock.Anything, mock.MatchedBy(func(req *evmtypes.SimulateV1Request) bool {
		return req != nil
	})).Return(nil, fmt.Errorf("internal error"))
}
