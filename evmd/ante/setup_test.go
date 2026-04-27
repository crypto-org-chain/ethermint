package ante_test

import (
	"math/big"

	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/evmos/ethermint/ante/interfaces"
	"github.com/evmos/ethermint/tests"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
)

func (suite *AnteTestSuite) TestEthSetupContextDecorator() {
	tx := evmtypes.NewTxContract(suite.app.EvmKeeper.ChainID(), 1, big.NewInt(10), 1000, big.NewInt(1), nil, nil, nil, nil)

	testCases := []struct {
		name    string
		tx      sdk.Tx
		expPass bool
	}{
		{
			"success - transaction implement GasTx",
			tx,
			true,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			ctx, err := interfaces.SetupEthContext(suite.ctx)

			if tc.expPass {
				suite.Require().NoError(err)
				suite.Equal(storetypes.GasConfig{}, ctx.KVGasConfig())
				suite.Equal(storetypes.GasConfig{}, ctx.TransientKVGasConfig())
			} else {
				suite.Require().Error(err)
			}
		})
	}
}

func (suite *AnteTestSuite) TestValidateBasicDecorator() {
	addr, privKey := tests.NewAddrKey()

	signedTx := evmtypes.NewTxContract(suite.app.EvmKeeper.ChainID(), 1, big.NewInt(10), 1000, big.NewInt(1), nil, nil, nil, nil)
	signedTx.From = addr.Bytes()
	err := signedTx.Sign(suite.ethSigner, tests.NewSigner(privKey))
	suite.Require().NoError(err)

	unprotectedTx := evmtypes.NewTxContract(nil, 1, big.NewInt(10), 1000, big.NewInt(1), nil, nil, nil, nil)
	unprotectedTx.From = addr.Bytes()
	err = unprotectedTx.Sign(ethtypes.HomesteadSigner{}, tests.NewSigner(privKey))
	suite.Require().NoError(err)
	tmTx, err := unprotectedTx.BuildTx(suite.clientCtx.TxConfig.NewTxBuilder(), evmtypes.DefaultEVMDenom)
	suite.Require().NoError(err)

	testCases := []struct {
		name                string
		tx                  sdk.Tx
		allowUnprotectedTxs bool
		reCheckTx           bool
		expPass             bool
	}{
		{"invalid transaction type", &invalidTx{}, false, false, false},
		{
			"invalid sender",
			evmtypes.NewTx(suite.app.EvmKeeper.ChainID(), 1, &addr, big.NewInt(10), 1000, big.NewInt(1), nil, nil, nil, nil),
			true,
			false,
			false,
		},
		{"invalid, reject unprotected txs", tmTx, false, false, false},
		{"successful, allow unprotected txs", tmTx, true, false, true},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			suite.evmParamsOption = func(params *evmtypes.Params) {
				params.AllowUnprotectedTxs = tc.allowUnprotectedTxs
			}
			suite.SetupTest()

			evmParams := suite.app.EvmKeeper.GetParams(suite.ctx)
			chainID := suite.app.EvmKeeper.ChainID()
			chainCfg := evmParams.GetChainConfig()
			ethCfg := chainCfg.EthereumConfig(chainID)
			baseFee := suite.app.EvmKeeper.GetBaseFee(suite.ctx, ethCfg)

			err := interfaces.ValidateEthBasic(suite.ctx.WithIsReCheckTx(tc.reCheckTx), tc.tx, &evmParams, baseFee)

			if tc.expPass {
				suite.Require().NoError(err)
			} else {
				suite.Require().Error(err)
			}
		})
	}
	suite.evmParamsOption = nil
}

func (suite *AnteTestSuite) TestValidateEthBasicRejectsDuplicateLane() {
	suite.SetupTest()

	addr, privKey := tests.NewAddrKey()
	chainID := suite.app.EvmKeeper.ChainID()

	msg1 := evmtypes.NewTxContract(chainID, 7, big.NewInt(10), 1000, big.NewInt(1), nil, nil, nil, nil)
	msg1.From = addr.Bytes()
	err := msg1.Sign(suite.ethSigner, tests.NewSigner(privKey))
	suite.Require().NoError(err)

	msg2 := evmtypes.NewTxContract(chainID, 7, big.NewInt(11), 1000, big.NewInt(1), nil, nil, nil, nil)
	msg2.From = addr.Bytes()
	err = msg2.Sign(suite.ethSigner, tests.NewSigner(privKey))
	suite.Require().NoError(err)

	tx := suite.buildMultiEthEnvelopeTx(privKey, msg1, msg2)
	evmParams := suite.app.EvmKeeper.GetParams(suite.ctx)
	ethCfg := evmParams.GetChainConfig().EthereumConfig(chainID)
	baseFee := suite.app.EvmKeeper.GetBaseFee(suite.ctx, ethCfg)

	err = interfaces.ValidateEthBasic(suite.ctx, tx, &evmParams, baseFee)
	suite.Require().Error(err)
	suite.Require().Contains(err.Error(), "duplicate inner ethereum lane")
}

func (suite *AnteTestSuite) TestValidateEthBasicRejectsOver64Msgs() {
	suite.SetupTest()

	addr, privKey := tests.NewAddrKey()
	chainID := suite.app.EvmKeeper.ChainID()

	msgs := make([]*evmtypes.MsgEthereumTx, 0, 65)
	for i := 0; i < 65; i++ {
		msg := evmtypes.NewTxContract(chainID, uint64(i), big.NewInt(10), 1000, big.NewInt(1), nil, nil, nil, nil)
		msg.From = addr.Bytes()
		err := msg.Sign(suite.ethSigner, tests.NewSigner(privKey))
		suite.Require().NoError(err)
		msgs = append(msgs, msg)
	}

	tx := suite.buildMultiEthEnvelopeTx(privKey, msgs...)
	evmParams := suite.app.EvmKeeper.GetParams(suite.ctx)
	ethCfg := evmParams.GetChainConfig().EthereumConfig(chainID)
	baseFee := suite.app.EvmKeeper.GetBaseFee(suite.ctx, ethCfg)

	err := interfaces.ValidateEthBasic(suite.ctx, tx, &evmParams, baseFee)
	suite.Require().Error(err)
	suite.Require().Contains(err.Error(), "number of messages should be <=")
}

func (suite *AnteTestSuite) TestValidateEthBasicAccepts64Msgs() {
	suite.SetupTest()

	addr, privKey := tests.NewAddrKey()
	chainID := suite.app.EvmKeeper.ChainID()

	msgs := make([]*evmtypes.MsgEthereumTx, 0, 64)
	for i := 0; i < 64; i++ {
		msg := evmtypes.NewTxContract(chainID, uint64(i), big.NewInt(10), 1000, big.NewInt(1), nil, nil, nil, nil)
		msg.From = addr.Bytes()
		err := msg.Sign(suite.ethSigner, tests.NewSigner(privKey))
		suite.Require().NoError(err)
		msgs = append(msgs, msg)
	}

	tx := suite.buildMultiEthEnvelopeTx(privKey, msgs...)
	evmParams := suite.app.EvmKeeper.GetParams(suite.ctx)
	ethCfg := evmParams.GetChainConfig().EthereumConfig(chainID)
	baseFee := suite.app.EvmKeeper.GetBaseFee(suite.ctx, ethCfg)

	err := interfaces.ValidateEthBasic(suite.ctx, tx, &evmParams, baseFee)
	suite.Require().NoError(err)
}

func (suite *AnteTestSuite) buildMultiEthEnvelopeTx(firstSigner cryptotypes.PrivKey, msgs ...*evmtypes.MsgEthereumTx) sdk.Tx {
	suite.Require().NotEmpty(msgs)

	txBuilder := suite.CreateTestTxBuilder(msgs[0], firstSigner, 1, false)
	sdkMsgs := make([]sdk.Msg, 0, len(msgs))
	txFee := sdk.Coins{}
	txGasLimit := uint64(0)

	for _, msg := range msgs {
		sdkMsgs = append(sdkMsgs, msg)
		txGasLimit += msg.GetGas()
		txFee = txFee.Add(sdk.Coin{
			Denom:  evmtypes.DefaultEVMDenom,
			Amount: sdkmath.NewIntFromBigInt(msg.GetFee()),
		})
	}

	err := txBuilder.SetMsgs(sdkMsgs...)
	suite.Require().NoError(err)
	txBuilder.SetFeeAmount(txFee)
	txBuilder.SetGasLimit(txGasLimit)

	return txBuilder.GetTx()
}
