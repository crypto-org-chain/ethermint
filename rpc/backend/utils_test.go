package backend

import (
	"fmt"
	"slices"

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
	key := string(header.ProposerAddress)
	h, w := header.Height, int64(validatorAccountCacheBlocks)
	window := blockRange{h - w, h + w}

	// mocks allow one call each, so a repeat lookup must hit the cache
	testCases := []struct {
		name      string
		malleate  func(queryClient *mocks.EVMQueryClient)
		expPass   bool
		expRanges []blockRange
	}{
		{
			"pass - resolved at the latest height",
			func(queryClient *mocks.EVMQueryClient) {
				queryClient.On("ValidatorAccount", suite.backend.ctx, req).Return(found, nil).Once()
			},
			true,
			[]blockRange{window},
		},
		{
			"pass - validator removed, resolved at the block height",
			func(queryClient *mocks.EVMQueryClient) {
				queryClient.On("ValidatorAccount", suite.backend.ctx, req).Return(nil, notFound).Once()
				queryClient.On("ValidatorAccount", rpc.ContextWithHeight(header.Height), req).Return(found, nil).Once()
			},
			true,
			[]blockRange{window},
		},
		{
			"pass - entry within the block window is reused",
			func(_ *mocks.EVMQueryClient) {
				suite.backend.validatorAccounts.Add(key, validatorAccountEntry{acc: validator}.withBlock(h+w))
			},
			true,
			[]blockRange{{h, h + 2*w}},
		},
		{
			"pass - same account outside the block window extends its ranges",
			func(queryClient *mocks.EVMQueryClient) {
				suite.backend.validatorAccounts.Add(key, validatorAccountEntry{acc: validator}.withBlock(h+3*w))
				queryClient.On("ValidatorAccount", suite.backend.ctx, req).Return(found, nil).Once()
			},
			true,
			[]blockRange{window, {h + 2*w, h + 4*w}},
		},
		{
			"pass - another account outside the block window replaces its ranges",
			func(queryClient *mocks.EVMQueryClient) {
				stale := sdk.AccAddress(tests.GenerateAddress().Bytes())
				suite.backend.validatorAccounts.Add(key, validatorAccountEntry{acc: stale}.withBlock(h+w+1))
				queryClient.On("ValidatorAccount", suite.backend.ctx, req).Return(found, nil).Once()
			},
			true,
			[]blockRange{window},
		},
		{
			"fail - not found at either height",
			func(queryClient *mocks.EVMQueryClient) {
				queryClient.On("ValidatorAccount", suite.backend.ctx, req).Return(nil, notFound).Once()
				queryClient.On("ValidatorAccount", rpc.ContextWithHeight(header.Height), req).Return(nil, notFound).Once()
			},
			false,
			nil,
		},
		{
			"fail - malformed account address",
			func(queryClient *mocks.EVMQueryClient) {
				queryClient.On("ValidatorAccount", suite.backend.ctx, req).
					Return(&evmtypes.QueryValidatorAccountResponse{AccountAddress: "invalid"}, nil).Once()
			},
			false,
			nil,
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

			entry, _ := suite.backend.validatorAccounts.Get(key)
			suite.Require().Equal(tc.expRanges, entry.ranges)
		})
	}
}

func (suite *BackendTestSuite) TestValidatorAccountEntryWithBlock() {
	w := int64(validatorAccountCacheBlocks)
	// a full entry of single-block ranges spaced far apart, starting at 4w
	full := make([]blockRange, validatorAccountCacheRanges)
	for i := range full {
		at := int64(i+1) * 4 * w
		full[i] = blockRange{at, at}
	}

	testCases := []struct {
		name      string
		ranges    []blockRange
		height    int64
		expRanges []blockRange
	}{
		{
			"empty entry gets the window",
			nil,
			100,
			[]blockRange{{100 - w, 100 + w}},
		},
		{
			"window one block apart stays separate",
			[]blockRange{{0, 10}},
			12 + w,
			[]blockRange{{0, 10}, {12, 12 + 2*w}},
		},
		{
			"touching window merges",
			[]blockRange{{0, 10}},
			11 + w,
			[]blockRange{{0, 11 + 2*w}},
		},
		{
			"window spanning several ranges merges them",
			[]blockRange{{0, 10}, {20, 30}, {4 * w, 5 * w}},
			15,
			[]blockRange{{15 - w, 15 + w}, {4 * w, 5 * w}},
		},
		{
			"full entry drops the farthest range, after the window",
			full,
			0,
			append([]blockRange{{-w, w}}, full[:len(full)-1]...),
		},
		{
			"full entry drops the farthest range, before the window",
			full,
			1000 * w,
			append(slices.Clone(full[1:]), blockRange{999 * w, 1001 * w}),
		},
		{
			"full entry drops the farthest range, window in the middle",
			full,
			42 * w,
			slices.Concat(full[:10], []blockRange{{41 * w, 43 * w}}, full[10:len(full)-1]),
		},
		{
			"full entry merging into a neighbour drops nothing",
			full,
			5 * w,
			append([]blockRange{{4 * w, 6 * w}}, full[1:]...),
		},
	}
	for _, tc := range testCases {
		suite.Run(fmt.Sprintf("Case %s", tc.name), func() {
			before := slices.Clone(tc.ranges)
			got := validatorAccountEntry{ranges: tc.ranges}.withBlock(tc.height)

			suite.Require().Equal(tc.expRanges, got.ranges)
			suite.Require().True(got.covers(tc.height))
			suite.Require().Equal(before, tc.ranges, "cached ranges must not be mutated")
		})
	}
}
