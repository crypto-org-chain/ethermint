package keeper_test

import (
	"encoding/json"
	"math/big"
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"

	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/evmos/ethermint/server/config"
	"github.com/evmos/ethermint/testutil"
	ethermint "github.com/evmos/ethermint/types"
	evmkeeper "github.com/evmos/ethermint/x/evm/keeper"
	"github.com/evmos/ethermint/x/evm/types"
)

type KeeperBenchmarkTestSuite struct {
	testutil.EVMTestSuiteWithAccountAndQueryClient
}

// deployTestMessageCall deploy a test erc20 contract and returns the contract address
func (suite *KeeperBenchmarkTestSuite) deployTestMessageCall(b *testing.B) common.Address {
	chainID := suite.App.EvmKeeper.ChainID()
	data := types.TestMessageCall.Bin
	args, err := json.Marshal(&types.TransactionArgs{
		From: &suite.Address,
		Data: (*hexutil.Bytes)(&data),
	})
	require.NoError(b, err)

	res, err := suite.EvmQueryClient.EstimateGas(suite.Ctx, &types.EthCallRequest{
		Args:            args,
		GasCap:          uint64(config.DefaultGasCap),
		ProposerAddress: suite.Ctx.BlockHeader().ProposerAddress,
	})
	require.NoError(b, err)

	nonce := suite.App.EvmKeeper.GetNonce(suite.Ctx, suite.Address)
	erc20DeployTx := types.NewTxContract(
		chainID,
		nonce,
		nil,     // amount
		res.Gas, // gasLimit
		nil,     // gasPrice
		nil, nil,
		data, // input
		nil,  // accesses
	)
	erc20DeployTx.From = suite.Address.Bytes()
	err = erc20DeployTx.Sign(ethtypes.LatestSignerForChainID(chainID), suite.Signer)
	require.NoError(b, err)
	rsp, err := suite.App.EvmKeeper.EthereumTx(suite.Ctx, erc20DeployTx)
	require.NoError(b, err)
	require.Empty(b, rsp.VmError)
	return crypto.CreateAddress(suite.Address, nonce)
}

func setupContract(b *testing.B) (*KeeperBenchmarkTestSuite, common.Address) {
	suite := KeeperBenchmarkTestSuite{}
	suite.SetupTest(b)

	amt := sdk.Coins{ethermint.NewDefaultCoinInt64(1000000000000000000)}
	err := suite.App.BankKeeper.MintCoins(suite.Ctx, types.ModuleName, amt)
	require.NoError(b, err)
	err = suite.App.BankKeeper.SendCoinsFromModuleToAccount(suite.Ctx, types.ModuleName, suite.Address.Bytes(), amt)
	require.NoError(b, err)

	contractAddr := suite.DeployTestContract(b, suite.Address, sdkmath.NewIntWithDecimal(1000, 18).BigInt(), false)
	suite.Commit(b)

	return &suite, contractAddr
}

func setupTestMessageCall(b *testing.B) (*KeeperBenchmarkTestSuite, common.Address) {
	suite := KeeperBenchmarkTestSuite{}
	suite.SetupTest(b)

	amt := sdk.Coins{ethermint.NewDefaultCoinInt64(1000000000000000000)}
	err := suite.App.BankKeeper.MintCoins(suite.Ctx, types.ModuleName, amt)
	require.NoError(b, err)
	err = suite.App.BankKeeper.SendCoinsFromModuleToAccount(suite.Ctx, types.ModuleName, suite.Address.Bytes(), amt)
	require.NoError(b, err)

	contractAddr := suite.deployTestMessageCall(b)
	suite.Commit(b)

	return &suite, contractAddr
}

type TxBuilder func(suite *KeeperBenchmarkTestSuite, contract common.Address) *types.MsgEthereumTx

func doBenchmark(b *testing.B, txBuilder TxBuilder) {
	suite, contractAddr := setupContract(b)

	msg := txBuilder(suite, contractAddr)
	msg.From = suite.Address.Bytes()
	err := msg.Sign(ethtypes.LatestSignerForChainID(suite.App.EvmKeeper.ChainID()), suite.Signer)
	require.NoError(b, err)

	b.ResetTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		ctx, _ := suite.Ctx.CacheContext()

		fees := sdk.Coins{sdk.NewCoin(suite.EvmDenom(), sdkmath.NewIntFromBigInt(msg.GetFee()))}
		err = evmkeeper.DeductFees(suite.App.BankKeeper, suite.Ctx, suite.App.AccountKeeper.GetAccount(ctx, msg.GetFrom()), fees)
		require.NoError(b, err)

		rsp, err := suite.App.EvmKeeper.EthereumTx(ctx, msg)
		require.NoError(b, err)
		require.False(b, rsp.Failed())
	}
}

func BenchmarkTokenTransfer(b *testing.B) {
	doBenchmark(b, func(suite *KeeperBenchmarkTestSuite, contract common.Address) *types.MsgEthereumTx {
		input, err := types.ERC20Contract.ABI.Pack("transfer", common.HexToAddress("0x378c50D9264C63F3F92B806d4ee56E9D86FfB3Ec"), big.NewInt(1000))
		require.NoError(b, err)
		nonce := suite.App.EvmKeeper.GetNonce(suite.Ctx, suite.Address)
		return types.NewTx(suite.App.EvmKeeper.ChainID(), nonce, &contract, big.NewInt(0), 410000, big.NewInt(1), nil, nil, input, nil)
	})
}

func BenchmarkEmitLogs(b *testing.B) {
	doBenchmark(b, func(suite *KeeperBenchmarkTestSuite, contract common.Address) *types.MsgEthereumTx {
		input, err := types.ERC20Contract.ABI.Pack("benchmarkLogs", big.NewInt(1000))
		require.NoError(b, err)
		nonce := suite.App.EvmKeeper.GetNonce(suite.Ctx, suite.Address)
		return types.NewTx(suite.App.EvmKeeper.ChainID(), nonce, &contract, big.NewInt(0), 4100000, big.NewInt(1), nil, nil, input, nil)
	})
}

func BenchmarkTokenTransferFrom(b *testing.B) {
	doBenchmark(b, func(suite *KeeperBenchmarkTestSuite, contract common.Address) *types.MsgEthereumTx {
		input, err := types.ERC20Contract.ABI.Pack("transferFrom", suite.Address, common.HexToAddress("0x378c50D9264C63F3F92B806d4ee56E9D86FfB3Ec"), big.NewInt(0))
		require.NoError(b, err)
		nonce := suite.App.EvmKeeper.GetNonce(suite.Ctx, suite.Address)
		return types.NewTx(suite.App.EvmKeeper.ChainID(), nonce, &contract, big.NewInt(0), 410000, big.NewInt(1), nil, nil, input, nil)
	})
}

func BenchmarkTokenMint(b *testing.B) {
	doBenchmark(b, func(suite *KeeperBenchmarkTestSuite, contract common.Address) *types.MsgEthereumTx {
		input, err := types.ERC20Contract.ABI.Pack("mint", common.HexToAddress("0x378c50D9264C63F3F92B806d4ee56E9D86FfB3Ec"), big.NewInt(1000))
		require.NoError(b, err)
		nonce := suite.App.EvmKeeper.GetNonce(suite.Ctx, suite.Address)
		return types.NewTx(suite.App.EvmKeeper.ChainID(), nonce, &contract, big.NewInt(0), 410000, big.NewInt(1), nil, nil, input, nil)
	})
}

func BenchmarkMessageCall(b *testing.B) {
	suite, contract := setupTestMessageCall(b)

	input, err := types.TestMessageCall.ABI.Pack("benchmarkMessageCall", big.NewInt(10000))
	require.NoError(b, err)
	nonce := suite.App.EvmKeeper.GetNonce(suite.Ctx, suite.Address)
	msg := types.NewTx(suite.App.EvmKeeper.ChainID(), nonce, &contract, big.NewInt(0), 25000000, big.NewInt(1), nil, nil, input, nil)

	msg.From = suite.Address.Bytes()
	err = msg.Sign(ethtypes.LatestSignerForChainID(suite.App.EvmKeeper.ChainID()), suite.Signer)
	require.NoError(b, err)

	b.ResetTimer()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		ctx, _ := suite.Ctx.CacheContext()

		fees := sdk.Coins{sdk.NewCoin(suite.EvmDenom(), sdkmath.NewIntFromBigInt(msg.GetFee()))}
		err = evmkeeper.DeductFees(suite.App.BankKeeper, suite.Ctx, suite.App.AccountKeeper.GetAccount(ctx, msg.GetFrom()), fees)
		require.NoError(b, err)

		rsp, err := suite.App.EvmKeeper.EthereumTx(ctx, msg)
		require.NoError(b, err)
		require.False(b, rsp.Failed())
	}
}
