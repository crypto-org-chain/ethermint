package keeper_test

import (
	"math/big"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"

	abci "github.com/cometbft/cometbft/abci/types"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/evmos/ethermint/app"
	"github.com/evmos/ethermint/crypto/ethsecp256k1"
	"github.com/evmos/ethermint/encoding"
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

var _ = Describe("Evm", func() {
	Describe("Performing EVM transactions", func() {
		type txParams struct {
			gasLimit  uint64
			gasPrice  *big.Int
			gasFeeCap *big.Int
			gasTipCap *big.Int
			accesses  *ethtypes.AccessList
		}
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
				s.SetupTest(sdk.NewDec(minGasPrices), big.NewInt(baseFee))
			})

			Context("during CheckTx", func() {
				DescribeTable("should accept transactions with gas Limit > 0",
					func(malleate getprices) {
						p := malleate()
						to := tests.GenerateAddress()
						msgEthereumTx := s.buildEthTx(&to, p.gasLimit, p.gasPrice, p.gasFeeCap, p.gasTipCap, p.accesses)
						res := s.checkEthTx(msgEthereumTx)
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
						to := tests.GenerateAddress()
						msgEthereumTx := s.buildEthTx(&to, p.gasLimit, p.gasPrice, p.gasFeeCap, p.gasTipCap, p.accesses)
						res := s.checkEthTx(msgEthereumTx)
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
						to := tests.GenerateAddress()
						msgEthereumTx := s.buildEthTx(&to, p.gasLimit, p.gasPrice, p.gasFeeCap, p.gasTipCap, p.accesses)
						res := s.deliverEthTx(msgEthereumTx)
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
						to := tests.GenerateAddress()
						msgEthereumTx := s.buildEthTx(&to, p.gasLimit, p.gasPrice, p.gasFeeCap, p.gasTipCap, p.accesses)
						res := s.checkEthTx(msgEthereumTx)
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
	testutil.EVMTestSuiteWithAccountAndQueryClient
	ethSigner ethtypes.Signer
	privKey   *ethsecp256k1.PrivKey
}

func (suite *IntegrationTestSuite) SetupTest(minGasPrice sdk.Dec, baseFee *big.Int) {
	t := s.T()
	suite.EVMTestSuiteWithAccountAndQueryClient.SetupTestWithCb(t, func(app *app.EthermintApp, genesis app.GenesisState) app.GenesisState {
		feemarketGenesis := feemarkettypes.DefaultGenesisState()
		feemarketGenesis.Params.NoBaseFee = true
		genesis[feemarkettypes.ModuleName] = app.AppCodec().MustMarshalJSON(feemarketGenesis)
		return genesis
	})
	amount, ok := sdk.NewIntFromString("10000000000000000000")
	suite.Require().True(ok)
	initBalance := sdk.Coins{sdk.Coin{
		Denom:  evmtypes.DefaultEVMDenom,
		Amount: amount,
	}}
	privKey, address := suite.generateKey()
	testutil.FundAccount(s.App.BankKeeper, s.Ctx, address, initBalance)
	s.Commit()
	params := feemarkettypes.DefaultParams()
	params.MinGasPrice = minGasPrice
	suite.App.FeeMarketKeeper.SetParams(suite.Ctx, params)
	suite.App.FeeMarketKeeper.SetBaseFee(suite.Ctx, baseFee)
	s.Commit()
	suite.ethSigner = ethtypes.LatestSignerForChainID(suite.App.EvmKeeper.ChainID())
	suite.privKey = privKey
}

func (suite *IntegrationTestSuite) generateKey() (*ethsecp256k1.PrivKey, sdk.AccAddress) {
	address, priv := tests.NewAddrKey()
	return priv.(*ethsecp256k1.PrivKey), sdk.AccAddress(address.Bytes())
}

func (suite *IntegrationTestSuite) getNonce(addressBytes []byte) uint64 {
	return suite.App.EvmKeeper.GetNonce(
		suite.Ctx,
		common.BytesToAddress(addressBytes),
	)
}

func (suite *IntegrationTestSuite) buildEthTx(
	to *common.Address,
	gasLimit uint64,
	gasPrice *big.Int,
	gasFeeCap *big.Int,
	gasTipCap *big.Int,
	accesses *ethtypes.AccessList,
) *evmtypes.MsgEthereumTx {
	chainID := suite.App.EvmKeeper.ChainID()
	adr := suite.privKey.PubKey().Address()
	from := common.BytesToAddress(adr.Bytes())
	nonce := suite.getNonce(from.Bytes())
	data := make([]byte, 0)
	msgEthereumTx := evmtypes.NewTx(
		chainID,
		nonce,
		to,
		nil,
		gasLimit,
		gasPrice,
		gasFeeCap,
		gasTipCap,
		data,
		accesses,
	)
	msgEthereumTx.From = from.Bytes()
	return msgEthereumTx
}

func (suite *IntegrationTestSuite) prepareEthTx(msgEthereumTx *evmtypes.MsgEthereumTx) []byte {
	encodingConfig := encoding.MakeConfig(app.ModuleBasics)
	option, err := codectypes.NewAnyWithValue(&evmtypes.ExtensionOptionsEthereumTx{})
	suite.Require().NoError(err)

	txBuilder := encodingConfig.TxConfig.NewTxBuilder()
	builder, ok := txBuilder.(authtx.ExtensionOptionsTxBuilder)
	suite.Require().True(ok)
	builder.SetExtensionOptions(option)

	err = msgEthereumTx.Sign(suite.ethSigner, tests.NewSigner(suite.privKey))
	suite.Require().NoError(err)

	err = txBuilder.SetMsgs(msgEthereumTx)
	suite.Require().NoError(err)

	txData, err := evmtypes.UnpackTxData(msgEthereumTx.Data)
	suite.Require().NoError(err)

	evmDenom := suite.App.EvmKeeper.GetParams(suite.Ctx).EvmDenom
	fees := sdk.Coins{{Denom: evmDenom, Amount: sdk.NewIntFromBigInt(txData.Fee())}}
	builder.SetFeeAmount(fees)
	builder.SetGasLimit(msgEthereumTx.GetGas())

	// bz are bytes to be broadcasted over the network
	bz, err := encodingConfig.TxConfig.TxEncoder()(txBuilder.GetTx())
	suite.Require().NoError(err)

	return bz
}

func (suite *IntegrationTestSuite) checkEthTx(msgEthereumTx *evmtypes.MsgEthereumTx) abci.ResponseCheckTx {
	bz := suite.prepareEthTx(msgEthereumTx)
	req := abci.RequestCheckTx{Tx: bz}
	res := suite.App.BaseApp.CheckTx(req)
	return res
}

func (suite *IntegrationTestSuite) deliverEthTx(msgEthereumTx *evmtypes.MsgEthereumTx) abci.ResponseDeliverTx {
	bz := suite.prepareEthTx(msgEthereumTx)
	req := abci.RequestDeliverTx{Tx: bz}
	res := s.App.BaseApp.DeliverTx(req)
	return res
}
