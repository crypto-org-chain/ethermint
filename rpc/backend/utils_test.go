package backend

import (
	"fmt"

	"github.com/cometbft/cometbft/proto/tendermint/crypto"
	tmtypes "github.com/cometbft/cometbft/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/evmos/ethermint/rpc/backend/mocks"
	rpc "github.com/evmos/ethermint/rpc/types"
	"github.com/evmos/ethermint/tests"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

func mookProofs(num int, withData bool) *crypto.ProofOps {
	var proofOps *crypto.ProofOps
	if num > 0 {
		proofOps = new(crypto.ProofOps)
		for i := 0; i < num; i++ {
			proof := crypto.ProofOp{}
			if withData {
				proof.Data = []byte("\n\031\n\003KEY\022\005VALUE\032\013\010\001\030\001 \001*\003\000\002\002")
			}
			proofOps.Ops = append(proofOps.Ops, proof)
		}
	}
	return proofOps
}

func (suite *BackendTestSuite) TestGetHexProofs() {
	defaultRes := []string{""}
	testCases := []struct {
		name  string
		proof *crypto.ProofOps
		exp   []string
	}{
		{
			"no proof provided",
			mookProofs(0, false),
			defaultRes,
		},
		{
			"no proof data provided",
			mookProofs(1, false),
			defaultRes,
		},
		{
			"valid proof provided",
			mookProofs(1, true),
			[]string{"0x0a190a034b4559120556414c55451a0b0801180120012a03000202"},
		},
	}
	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.name), func() {
			suite.Require().Equal(tc.exp, GetHexProofs(tc.proof))
		})
	}
}

func (suite *BackendTestSuite) TestGetValidatorAccount() {
	validator := sdk.AccAddress(tests.GenerateAddress().Bytes())
	header := &tmtypes.Header{Height: 5, ProposerAddress: tests.GenerateAddress().Bytes()}
	req := &evmtypes.QueryValidatorAccountRequest{ConsAddress: sdk.ConsAddress(header.ProposerAddress).String()}
	found := &evmtypes.QueryValidatorAccountResponse{AccountAddress: validator.String()}
	notFound := status.Error(codes.NotFound, "validator not found")

	// mocks allow one query per height, so a second lookup must come from the cache;
	// an unregistered height fails the test if queried.
	testCases := []struct {
		name     string
		malleate func(queryClient *mocks.EVMQueryClient)
		expPass  bool
	}{
		{
			"pass - resolved at the latest height, block height not queried",
			func(queryClient *mocks.EVMQueryClient) {
				queryClient.On("ValidatorAccount", suite.backend.ctx, req).Return(found, nil).Once()
			},
			true,
		},
		{
			"pass - validator gone at the latest height, resolved at the block height",
			func(queryClient *mocks.EVMQueryClient) {
				queryClient.On("ValidatorAccount", suite.backend.ctx, req).Return(nil, notFound).Once()
				queryClient.On("ValidatorAccount", rpc.ContextWithHeight(header.Height), req).Return(found, nil).Once()
			},
			true,
		},
		{
			"fail - not found at either height",
			func(queryClient *mocks.EVMQueryClient) {
				queryClient.On("ValidatorAccount", suite.backend.ctx, req).Return(nil, notFound).Once()
				queryClient.On("ValidatorAccount", rpc.ContextWithHeight(header.Height), req).Return(nil, notFound).Once()
			},
			false,
		},
	}
	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.name), func() {
			suite.SetupTest()
			tc.malleate(suite.backend.queryClient.QueryClient.(*mocks.EVMQueryClient))

			acc, err := suite.backend.getValidatorAccount(header)
			if !tc.expPass {
				suite.Require().Error(err)
				suite.Require().Zero(suite.backend.validatorAccounts.Len(), "failed lookups must not be cached")
				return
			}
			suite.Require().NoError(err)
			suite.Require().Equal(validator, acc)

			acc, err = suite.backend.getValidatorAccount(header)
			suite.Require().NoError(err)
			suite.Require().Equal(validator, acc)
		})
	}
}
