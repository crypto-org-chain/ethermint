package ante_test

import (
	"math/big"

	sdkmath "cosmossdk.io/math"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/evmos/ethermint/appmempool"
	"github.com/evmos/ethermint/tests"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

func (suite *AnteTestSuite) TestEVMSigPreVerifier() {
	suite.SetupTest()

	decoder := suite.clientCtx.TxConfig.TxDecoder()
	encoder := suite.clientCtx.TxConfig.TxEncoder()

	// Unparseable chain ID yields no hook: caller keeps admission fully locked.
	suite.Require().Nil(appmempool.NewEVMSigPreVerifier("garbage", decoder))

	hook := appmempool.NewEVMSigPreVerifier(suite.ctx.ChainID(), decoder)
	suite.Require().NotNil(hook)

	addr, priv := tests.NewAddrKey()
	to := tests.GenerateAddress()

	// Valid signature on a pure-EVM tx passes pre-verification.
	msg := suite.BuildTestEthTx(addr, to, big.NewInt(10), nil, big.NewInt(1), nil, nil, nil)
	signedTx := suite.CreateTestTx(msg, priv, 1, false)
	validBz, err := encoder(signedTx)
	suite.Require().NoError(err)
	suite.Require().NoError(hook(validBz))

	// Undecodable bytes defer to the locked path (nil, not a reject).
	suite.Require().NoError(hook([]byte("not a tx")))

	// Non-EVM (cosmos) tx defers to the locked path.
	send := banktypes.NewMsgSend(addr.Bytes(), to.Bytes(), sdk.NewCoins(sdk.NewCoin(evmtypes.DefaultEVMDenom, sdkmath.NewInt(1))))
	cosmosTx := suite.CreateTestCosmosTxBuilder(sdkmath.NewInt(1), evmtypes.DefaultEVMDenom, send).GetTx()
	cosmosBz, err := encoder(cosmosTx)
	suite.Require().NoError(err)
	suite.Require().NoError(hook(cosmosBz))

	// Tampered sender on a pure-EVM tx is rejected early: sign correctly, then
	// overwrite From so the recovered signer no longer matches.
	badMsg := suite.BuildTestEthTx(addr, to, big.NewInt(10), nil, big.NewInt(1), nil, nil, nil)
	suite.Require().NoError(badMsg.Sign(suite.ethSigner, tests.NewSigner(priv)))
	badMsg.From = tests.GenerateAddress().Bytes()

	builder := suite.clientCtx.TxConfig.NewTxBuilder()
	opt, err := codectypes.NewAnyWithValue(&evmtypes.ExtensionOptionsEthereumTx{})
	suite.Require().NoError(err)
	builder.(authtx.ExtensionOptionsTxBuilder).SetExtensionOptions(opt)
	suite.Require().NoError(builder.SetMsgs(badMsg))
	builder.SetGasLimit(badMsg.GetGas())
	builder.SetFeeAmount(sdk.NewCoins(sdk.NewCoin(evmtypes.DefaultEVMDenom, sdkmath.NewIntFromBigInt(badMsg.GetFee()))))
	badBz, err := encoder(builder.GetTx())
	suite.Require().NoError(err)
	suite.Require().Error(hook(badBz))
}
