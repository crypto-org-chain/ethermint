package keeper_test

import (
	"math/big"
	"testing"

	sdkmath "cosmossdk.io/math"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"

	"github.com/cosmos/cosmos-sdk/server"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/evmos/ethermint/app"
	"github.com/evmos/ethermint/tests"
	"github.com/evmos/ethermint/testutil"

	evmtypes "github.com/evmos/ethermint/x/evm/types"
	feemarkettypes "github.com/evmos/ethermint/x/feemarket/types"
)

var s *IntegrationTestSuite

func TestEvm(t *testing.T) {
	// Run Ginkgo integration tests
	s = new(IntegrationTestSuite)
	suite.Run(t, s)

	RegisterFailHandler(Fail)
	RunSpecs(t, "IntegrationTestSuite")
}

type txParams struct {
	gasLimit  uint64
	gasPrice  *big.Int
	gasFeeCap *big.Int
	gasTipCap *big.Int
	accesses  *ethtypes.AccessList
}

var _ = Describe("Evm", func() {
	Describe("Performing EVM transactions", func() {
		type getprices func() txParams

		Context("with MinGasPrices (feemarket param) < BaseFee (feemarket)", func() {
			var (
				baseFee      int64
				minGasPrices int64
			)

			BeforeEach(func() {
				baseFee = 10_000_000_000
				minGasPrices = baseFee - 5_000_000_000

				// Note that the tests run the same transactions with `gasLimit =
				// 100_000`. With the fee calculation `Fee = (baseFee + tip) * gasLimit`,
				// a `minGasPrices = 5_000_000_000` results in `minGlobalFee =
				// 500_000_000_000_000`
				setupTest(sdkmath.LegacyNewDec(minGasPrices), big.NewInt(baseFee))
			})

			Context("during CheckTx", func() {
				DescribeTable("should accept transactions with gas Limit > 0",
					func(malleate getprices) {
						p := malleate()
						res := s.CheckTx(prepareEthTx(p))
						Expect(res.IsOK()).To(Equal(true), "transaction should have succeeded", res.GetLog())
					},
					Entry("legacy tx", func() txParams {
						return txParams{100000, big.NewInt(baseFee), nil, nil, nil}
					}),
					Entry("dynamic tx", func() txParams {
						return txParams{100000, nil, big.NewInt(baseFee), big.NewInt(0), &ethtypes.AccessList{}}
					}),
				)
				DescribeTable("should not accept transactions with gas Limit > 0",
					func(malleate getprices) {
						p := malleate()
						res := s.CheckTx(prepareEthTx(p))
						Expect(res.IsOK()).To(Equal(false), "transaction should have failed", res.GetLog())
					},
					Entry("legacy tx", func() txParams {
						return txParams{0, big.NewInt(baseFee), nil, nil, nil}
					}),
					Entry("dynamic tx", func() txParams {
						return txParams{0, nil, big.NewInt(baseFee), big.NewInt(0), &ethtypes.AccessList{}}
					}),
				)
			})

			Context("during DeliverTx", func() {
				DescribeTable("should accept transactions with gas Limit > 0",
					func(malleate getprices) {
						p := malleate()
						res := s.DeliverTx(prepareEthTx(p))
						Expect(res.IsOK()).To(Equal(true), "transaction should have succeeded", res.GetLog())
					},
					Entry("legacy tx", func() txParams {
						return txParams{100000, big.NewInt(baseFee), nil, nil, nil}
					}),
					Entry("dynamic tx", func() txParams {
						return txParams{100000, nil, big.NewInt(baseFee), big.NewInt(0), &ethtypes.AccessList{}}
					}),
				)
				DescribeTable("should not accept transactions with gas Limit > 0",
					func(malleate getprices) {
						p := malleate()
						res := s.DeliverTx(prepareEthTx(p))
						Expect(res.IsOK()).To(Equal(false), "transaction should have failed", res.GetLog())
					},
					Entry("legacy tx", func() txParams {
						return txParams{0, big.NewInt(baseFee), nil, nil, nil}
					}),
					Entry("dynamic tx", func() txParams {
						return txParams{0, nil, big.NewInt(baseFee), big.NewInt(0), &ethtypes.AccessList{}}
					}),
				)
			})
		})
	})
})

type IntegrationTestSuite struct {
	testutil.BaseTestSuiteWithAccount
}

func setupTest(minGasPrice sdkmath.LegacyDec, baseFee *big.Int) {
	t := s.T()
	s.SetupTestWithCbAndOpts(
		t,
		func(app *app.EthermintApp, genesis app.GenesisState) app.GenesisState {
			feemarketGenesis := feemarkettypes.DefaultGenesisState()
			feemarketGenesis.Params.NoBaseFee = true
			genesis[feemarkettypes.ModuleName] = app.AppCodec().MustMarshalJSON(feemarketGenesis)
			return genesis
		},
		simtestutil.AppOptionsMap{server.FlagMinGasPrices: "1" + evmtypes.DefaultEVMDenom},
	)
	amount, ok := sdkmath.NewIntFromString("10000000000000000000")
	s.Require().True(ok)
	initBalance := sdk.Coins{sdk.Coin{
		Denom:  evmtypes.DefaultEVMDenom,
		Amount: amount,
	}}
	testutil.FundAccount(s.App.BankKeeper, s.Ctx, sdk.AccAddress(s.Address.Bytes()), initBalance)
	s.Commit(t)
	params := feemarkettypes.DefaultParams()
	params.MinGasPrice = minGasPrice
	s.App.FeeMarketKeeper.SetParams(s.Ctx, params)
	s.App.FeeMarketKeeper.SetBaseFee(s.Ctx, baseFee)
	s.Commit(t)
}

func prepareEthTx(p txParams) []byte {
	to := tests.GenerateAddress()
	msg := s.BuildEthTx(&to, p.gasLimit, p.gasPrice, p.gasFeeCap, p.gasTipCap, p.accesses, s.PrivKey)
	return s.PrepareEthTx(msg, s.PrivKey)
}
