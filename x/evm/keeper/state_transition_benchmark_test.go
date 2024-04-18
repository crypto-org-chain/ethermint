package keeper_test

import (
	"errors"
	"math"
	"math/big"
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/evmos/ethermint/app"
	"github.com/evmos/ethermint/testutil"
	evmtypes "github.com/evmos/ethermint/x/evm/types"
	feemarkettypes "github.com/evmos/ethermint/x/feemarket/types"
	"github.com/stretchr/testify/require"
)

type StateTransitionBenchmarkTestSuite struct {
	testutil.BaseTestSuiteWithAccount
	enableFeemarket bool
	enableLondonHF  bool
}

func (suite *StateTransitionBenchmarkTestSuite) SetupTest(b *testing.B) {
	suite.BaseTestSuiteWithAccount.SetupTestWithCb(b, func(app *app.EthermintApp, genesis app.GenesisState) app.GenesisState {
		feemarketGenesis := feemarkettypes.DefaultGenesisState()
		if suite.enableFeemarket {
			feemarketGenesis.Params.EnableHeight = 1
			feemarketGenesis.Params.NoBaseFee = false
		} else {
			feemarketGenesis.Params.NoBaseFee = true
		}
		genesis[feemarkettypes.ModuleName] = app.AppCodec().MustMarshalJSON(feemarketGenesis)
		if !suite.enableLondonHF {
			evmGenesis := evmtypes.DefaultGenesisState()
			maxInt := sdkmath.NewInt(math.MaxInt64)
			evmGenesis.Params.ChainConfig.LondonBlock = &maxInt
			evmGenesis.Params.ChainConfig.ArrowGlacierBlock = &maxInt
			evmGenesis.Params.ChainConfig.GrayGlacierBlock = &maxInt
			evmGenesis.Params.ChainConfig.MergeNetsplitBlock = &maxInt
			evmGenesis.Params.ChainConfig.ShanghaiTime = &maxInt
			genesis[evmtypes.ModuleName] = app.AppCodec().MustMarshalJSON(evmGenesis)
		}
		return genesis
	})
}

var templateAccessListTx = &ethtypes.AccessListTx{
	GasPrice: big.NewInt(1),
	Gas:      21000,
	To:       &common.Address{},
	Value:    big.NewInt(0),
	Data:     []byte{},
}

var templateLegacyTx = &ethtypes.LegacyTx{
	GasPrice: big.NewInt(1),
	Gas:      21000,
	To:       &common.Address{},
	Value:    big.NewInt(0),
	Data:     []byte{},
}

var templateDynamicFeeTx = &ethtypes.DynamicFeeTx{
	GasFeeCap: big.NewInt(10),
	GasTipCap: big.NewInt(2),
	Gas:       21000,
	To:        &common.Address{},
	Value:     big.NewInt(0),
	Data:      []byte{},
}

func newSignedEthTx(
	txData ethtypes.TxData,
	nonce uint64,
	addr sdk.Address,
	krSigner keyring.Signer,
	ethSigner ethtypes.Signer,
) (*evmtypes.MsgEthereumTx, error) {
	var ethTx *ethtypes.Transaction
	switch txData := txData.(type) {
	case *ethtypes.AccessListTx:
		txData.Nonce = nonce
		ethTx = ethtypes.NewTx(txData)
	case *ethtypes.LegacyTx:
		txData.Nonce = nonce
		ethTx = ethtypes.NewTx(txData)
	case *ethtypes.DynamicFeeTx:
		txData.Nonce = nonce
		ethTx = ethtypes.NewTx(txData)
	default:
		return nil, errors.New("unknown transaction type!")
	}

	sig, _, err := krSigner.SignByAddress(addr, ethTx.Hash().Bytes(), signing.SignMode_SIGN_MODE_TEXTUAL)
	if err != nil {
		return nil, err
	}

	ethTx, err = ethTx.WithSignature(ethSigner, sig)
	if err != nil {
		return nil, err
	}

	var msg evmtypes.MsgEthereumTx
	if err := msg.FromSignedEthereumTx(ethTx, ethtypes.LatestSignerForChainID(ethSigner.ChainID())); err != nil {
		return nil, err
	}
	return &msg, nil
}

func newEthMsgTx(
	nonce uint64,
	address common.Address,
	krSigner keyring.Signer,
	ethSigner ethtypes.Signer,
	txType byte,
	data []byte,
	accessList ethtypes.AccessList,
) (*evmtypes.MsgEthereumTx, *big.Int, error) {
	var (
		ethTx   *ethtypes.Transaction
		baseFee *big.Int
	)
	switch txType {
	case ethtypes.LegacyTxType:
		templateLegacyTx.Nonce = nonce
		if data != nil {
			templateLegacyTx.Data = data
		}
		ethTx = ethtypes.NewTx(templateLegacyTx)
	case ethtypes.AccessListTxType:
		templateAccessListTx.Nonce = nonce
		if data != nil {
			templateAccessListTx.Data = data
		} else {
			templateAccessListTx.Data = []byte{}
		}

		templateAccessListTx.AccessList = accessList
		ethTx = ethtypes.NewTx(templateAccessListTx)
	case ethtypes.DynamicFeeTxType:
		templateDynamicFeeTx.Nonce = nonce

		if data != nil {
			templateAccessListTx.Data = data
		} else {
			templateAccessListTx.Data = []byte{}
		}
		templateAccessListTx.AccessList = accessList
		ethTx = ethtypes.NewTx(templateDynamicFeeTx)
		baseFee = big.NewInt(3)
	default:
		return nil, baseFee, errors.New("unsupport tx type")
	}

	msg := &evmtypes.MsgEthereumTx{}
	msg.FromEthereumTx(ethTx)
	msg.From = address.Bytes()

	return msg, baseFee, msg.Sign(ethSigner, krSigner)
}

func newNativeMessage(
	nonce uint64,
	blockHeight int64,
	address common.Address,
	cfg *params.ChainConfig,
	krSigner keyring.Signer,
	ethSigner ethtypes.Signer,
	txType byte,
	data []byte,
	accessList ethtypes.AccessList,
) (core.Message, error) {
	msg, baseFee, err := newEthMsgTx(nonce, address, krSigner, ethSigner, txType, data, accessList)
	if err != nil {
		return core.Message{}, err
	}

	m, err := msg.AsMessage(baseFee)
	if err != nil {
		return core.Message{}, err
	}

	return m, nil
}

func BenchmarkApplyTransaction(b *testing.B) {
	suite := StateTransitionBenchmarkTestSuite{enableLondonHF: true}
	suite.SetupTest(b)

	ethSigner := ethtypes.LatestSignerForChainID(suite.App.EvmKeeper.ChainID())

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		tx, err := newSignedEthTx(templateAccessListTx,
			suite.App.EvmKeeper.GetNonce(suite.Ctx, suite.Address),
			sdk.AccAddress(suite.Address.Bytes()),
			suite.Signer,
			ethSigner,
		)
		require.NoError(b, err)

		b.StartTimer()
		resp, err := suite.App.EvmKeeper.ApplyTransaction(suite.Ctx, tx)
		b.StopTimer()

		require.NoError(b, err)
		require.False(b, resp.Failed())
	}
}

func BenchmarkApplyTransactionWithLegacyTx(b *testing.B) {
	suite := StateTransitionBenchmarkTestSuite{enableLondonHF: true}
	suite.SetupTest(b)

	ethSigner := ethtypes.LatestSignerForChainID(suite.App.EvmKeeper.ChainID())

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		tx, err := newSignedEthTx(templateLegacyTx,
			suite.App.EvmKeeper.GetNonce(suite.Ctx, suite.Address),
			sdk.AccAddress(suite.Address.Bytes()),
			suite.Signer,
			ethSigner,
		)
		require.NoError(b, err)

		b.StartTimer()
		resp, err := suite.App.EvmKeeper.ApplyTransaction(suite.Ctx, tx)
		b.StopTimer()

		require.NoError(b, err)
		require.False(b, resp.Failed())
	}
}

func BenchmarkApplyTransactionWithDynamicFeeTx(b *testing.B) {
	suite := StateTransitionBenchmarkTestSuite{enableFeemarket: true, enableLondonHF: true}
	suite.SetupTest(b)

	ethSigner := ethtypes.LatestSignerForChainID(suite.App.EvmKeeper.ChainID())

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		tx, err := newSignedEthTx(templateDynamicFeeTx,
			suite.App.EvmKeeper.GetNonce(suite.Ctx, suite.Address),
			sdk.AccAddress(suite.Address.Bytes()),
			suite.Signer,
			ethSigner,
		)
		require.NoError(b, err)

		b.StartTimer()
		resp, err := suite.App.EvmKeeper.ApplyTransaction(suite.Ctx, tx)
		b.StopTimer()

		require.NoError(b, err)
		require.False(b, resp.Failed())
	}
}

func BenchmarkApplyMessage(b *testing.B) {
	suite := StateTransitionBenchmarkTestSuite{enableLondonHF: true}
	suite.SetupTest(b)

	params := suite.App.EvmKeeper.GetParams(suite.Ctx)
	ethCfg := params.ChainConfig.EthereumConfig(suite.App.EvmKeeper.ChainID())
	signer := ethtypes.LatestSignerForChainID(suite.App.EvmKeeper.ChainID())

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()

		m, err := newNativeMessage(
			suite.App.EvmKeeper.GetNonce(suite.Ctx, suite.Address),
			suite.Ctx.BlockHeight(),
			suite.Address,
			ethCfg,
			suite.Signer,
			signer,
			ethtypes.AccessListTxType,
			nil,
			nil,
		)
		require.NoError(b, err)

		b.StartTimer()
		resp, err := suite.App.EvmKeeper.ApplyMessage(suite.Ctx, m, nil, true)
		b.StopTimer()

		require.NoError(b, err)
		require.False(b, resp.Failed())
	}
}

func BenchmarkApplyMessageWithLegacyTx(b *testing.B) {
	suite := StateTransitionBenchmarkTestSuite{enableLondonHF: true}
	suite.SetupTest(b)

	params := suite.App.EvmKeeper.GetParams(suite.Ctx)
	ethCfg := params.ChainConfig.EthereumConfig(suite.App.EvmKeeper.ChainID())
	signer := ethtypes.LatestSignerForChainID(suite.App.EvmKeeper.ChainID())

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()

		m, err := newNativeMessage(
			suite.App.EvmKeeper.GetNonce(suite.Ctx, suite.Address),
			suite.Ctx.BlockHeight(),
			suite.Address,
			ethCfg,
			suite.Signer,
			signer,
			ethtypes.LegacyTxType,
			nil,
			nil,
		)
		require.NoError(b, err)

		b.StartTimer()
		resp, err := suite.App.EvmKeeper.ApplyMessage(suite.Ctx, m, nil, true)
		b.StopTimer()

		require.NoError(b, err)
		require.False(b, resp.Failed())
	}
}

func BenchmarkApplyMessageWithDynamicFeeTx(b *testing.B) {
	suite := StateTransitionBenchmarkTestSuite{enableFeemarket: true, enableLondonHF: true}
	suite.SetupTest(b)

	params := suite.App.EvmKeeper.GetParams(suite.Ctx)
	ethCfg := params.ChainConfig.EthereumConfig(suite.App.EvmKeeper.ChainID())
	signer := ethtypes.LatestSignerForChainID(suite.App.EvmKeeper.ChainID())

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()

		m, err := newNativeMessage(
			suite.App.EvmKeeper.GetNonce(suite.Ctx, suite.Address),
			suite.Ctx.BlockHeight(),
			suite.Address,
			ethCfg,
			suite.Signer,
			signer,
			ethtypes.DynamicFeeTxType,
			nil,
			nil,
		)
		require.NoError(b, err)

		b.StartTimer()
		resp, err := suite.App.EvmKeeper.ApplyMessage(suite.Ctx, m, nil, true)
		b.StopTimer()

		require.NoError(b, err)
		require.False(b, resp.Failed())
	}
}
